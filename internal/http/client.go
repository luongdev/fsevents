package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"text/template"
	"time"

	"go.uber.org/zap"

	"fsevents/internal/config"
	"fsevents/internal/processor"
	"fsevents/pkg/types"
)

// Client handles HTTP forwarding of events to configured destinations
type Client struct {
	destinations     []config.HTTPDestination
	httpClient       *http.Client
	fieldMapper      *processor.FieldMapper
	processorManager *processor.ProcessorManager
	logger           *zap.Logger
}

// NewClient creates a new HTTP client with the given destinations and processors
func NewClient(destinations []config.HTTPDestination, fieldMapper *processor.FieldMapper, processorManager *processor.ProcessorManager, logger *zap.Logger) *Client {
	return &Client{
		destinations:     destinations,
		fieldMapper:      fieldMapper,
		processorManager: processorManager,
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // Default timeout, can be overridden per destination
		},
		logger: logger.Named("http"),
	}
}

// ForwardEvent forwards an event to configured HTTP destinations based on their event filters
func (c *Client) ForwardEvent(ctx context.Context, event *types.Event) error {
	if len(c.destinations) == 0 {
		c.logger.Debug("No HTTP destinations configured, skipping forward")
		return nil
	}

	// Filter destinations that should receive this event
	eligibleDestinations := c.getEligibleDestinations(event)
	if len(eligibleDestinations) == 0 {
		c.logger.Debug("No destinations configured for this event type",
			zap.String("event_name", event.Name),
			zap.String("event_subclass", event.Subclass),
		)
		return nil
	}

	// Forward to eligible destinations in parallel
	errChan := make(chan error, len(eligibleDestinations))

	for _, dest := range eligibleDestinations {
		go func(destination config.HTTPDestination) {
			// Create payload specific to this destination
			payload, err := c.createPayload(event, destination)
			if err != nil {
				errChan <- fmt.Errorf("failed to create payload for destination %s: %w", destination.Name, err)
				return
			}

			// Log payload for debugging
			c.logger.Debug("Created HTTP payload",
				zap.String("destination", destination.Name),
				zap.String("event_name", event.Name),
				zap.String("payload", string(payload)),
			)

			err = c.forwardToDestination(ctx, destination, payload)
			errChan <- err
		}(dest)
	}

	// Collect results
	var errors []error
	for i := 0; i < len(eligibleDestinations); i++ {
		if err := <-errChan; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to forward to %d destinations: %v", len(errors), errors)
	}

	return nil
}

// getEligibleDestinations returns destinations that should receive this event
func (c *Client) getEligibleDestinations(event *types.Event) []config.HTTPDestination {
	var eligible []config.HTTPDestination

	for _, dest := range c.destinations {
		// If no filters configured (neither EventFilters nor Filters), forward all events
		if len(dest.EventFilters) == 0 && len(dest.Filters) == 0 {
			eligible = append(eligible, dest)
			continue
		}

		// Check if this destination should receive this event
		if c.shouldForwardToDestination(dest, event) {
			eligible = append(eligible, dest)
		}
	}

	return eligible
}

// shouldForwardToDestination checks if a destination should receive an event
func (c *Client) shouldForwardToDestination(dest config.HTTPDestination, event *types.Event) bool {
	// First check advanced filters if they exist
	if len(dest.Filters) > 0 {
		return c.evaluateDestinationFilters(dest, event)
	}

	// Build event name with subclass for CUSTOM events (for legacy EventFilters)
	eventNameWithSubclass := event.Name
	if event.Name == "CUSTOM" && event.Subclass != "" {
		eventNameWithSubclass = event.Name + " " + event.Subclass
	}

	// Fall back to legacy EventFilters for backward compatibility
	for _, filter := range dest.EventFilters {
		// Exact match
		if filter == event.Name || filter == eventNameWithSubclass {
			return true
		}

		// Wildcard match
		if filter == "*" {
			return true
		}

		// Prefix wildcard match
		if strings.HasSuffix(filter, "*") {
			prefix := strings.TrimSuffix(filter, "*")
			if strings.HasPrefix(event.Name, prefix) || strings.HasPrefix(eventNameWithSubclass, prefix) {
				return true
			}
		}
	}

	return false
}

// evaluateDestinationFilters evaluates advanced filter rules for a destination
func (c *Client) evaluateDestinationFilters(dest config.HTTPDestination, event *types.Event) bool {
	// Build event name with subclass for logging
	eventNameWithSubclass := event.Name
	if event.Name == "CUSTOM" && event.Subclass != "" {
		eventNameWithSubclass = event.Name + " " + event.Subclass
	}

	// Default to AND logic if not specified
	filterLogic := dest.FilterLogic
	if filterLogic == "" {
		filterLogic = "AND"
	}

	c.logger.Debug("Evaluating destination filters",
		zap.String("destination", dest.Name),
		zap.String("event_name", event.Name),
		zap.String("event_name_with_subclass", eventNameWithSubclass),
		zap.Int("filter_count", len(dest.Filters)),
		zap.String("filter_logic", filterLogic),
	)

	// Apply filter rules based on logic type
	if filterLogic == "OR" {
		// OR logic: event passes if ANY filter matches
		for _, filter := range dest.Filters {
			if c.evaluateDestinationFilter(event, filter, dest.Name) {
				c.logger.Debug("Event passed OR destination filter",
					zap.String("destination", dest.Name),
					zap.String("event_name", event.Name),
					zap.String("filter_field", filter.Field),
					zap.String("filter_operator", filter.Operator),
					zap.String("filter_value", filter.Value),
				)
				return true
			}
		}

		c.logger.Debug("Event filtered out by destination OR logic - no filters matched",
			zap.String("destination", dest.Name),
			zap.String("event_name", event.Name),
			zap.Int("filter_count", len(dest.Filters)),
		)
		return false
	} else {
		// AND logic: event passes only if ALL filters match
		for _, filter := range dest.Filters {
			if !c.evaluateDestinationFilter(event, filter, dest.Name) {
				c.logger.Debug("Event filtered out by destination AND logic",
					zap.String("destination", dest.Name),
					zap.String("event_name", event.Name),
					zap.String("filter_field", filter.Field),
					zap.String("filter_operator", filter.Operator),
					zap.String("filter_value", filter.Value),
				)
				return false
			}
		}

		c.logger.Debug("Event passed all destination AND filters",
			zap.String("destination", dest.Name),
			zap.String("event_name", event.Name),
			zap.Int("filter_count", len(dest.Filters)),
		)
		return true
	}
}

// evaluateDestinationFilter evaluates a single filter rule against an event for destination filtering
func (c *Client) evaluateDestinationFilter(event *types.Event, filter config.FilterRule, destinationName string) bool {
	// Get the actual value from the event
	var actualValue string

	switch filter.Field {
	case "Event-Name":
		actualValue = event.Name
	case "Event-Subclass":
		actualValue = event.Subclass
	default:
		// For destination filtering, we only have basic event info
		// Try to get from headers if event has them, otherwise empty
		if event.Headers != nil {
			actualValue = event.GetHeader(filter.Field)
		}
	}

	// Apply the operator
	switch strings.ToLower(filter.Operator) {
	case "equals", "eq", "=", "==":
		return actualValue == filter.Value
	case "not_equals", "ne", "!=":
		return actualValue != filter.Value
	case "contains":
		return strings.Contains(actualValue, filter.Value)
	case "not_contains":
		return !strings.Contains(actualValue, filter.Value)
	case "starts_with":
		return strings.HasPrefix(actualValue, filter.Value)
	case "ends_with":
		return strings.HasSuffix(actualValue, filter.Value)
	case "regex":
		// Compile and match regex pattern
		regex, err := regexp.Compile(filter.Value)
		if err != nil {
			c.logger.Error("Invalid regex pattern in destination filter",
				zap.String("destination", destinationName),
				zap.String("field", filter.Field),
				zap.String("pattern", filter.Value),
				zap.Error(err),
			)
			return false
		}

		matched := regex.MatchString(actualValue)
		c.logger.Debug("Regex destination filter evaluation",
			zap.String("destination", destinationName),
			zap.String("field", filter.Field),
			zap.String("pattern", filter.Value),
			zap.String("actual_value", actualValue),
			zap.Bool("matched", matched),
		)
		return matched
	default:
		c.logger.Warn("Unknown filter operator in destination filter",
			zap.String("destination", destinationName),
			zap.String("operator", filter.Operator),
			zap.String("field", filter.Field),
		)
		return false
	}
}

// createPayload converts an event to JSON payload using configured mappers and processors
func (c *Client) createPayload(event *types.Event, dest config.HTTPDestination) ([]byte, error) {
	// Start with field mapping
	payload := c.fieldMapper.MapEvent(event)

	// Run through custom processors
	var err error
	payload, err = c.processorManager.ProcessEvent(event, payload)
	if err != nil {
		c.logger.Warn("Processor failed, using original payload", zap.Error(err))
	}

	// Apply payload template if configured
	if dest.PayloadTemplate != nil {
		return c.applyTemplate(payload, dest.PayloadTemplate)
	}

	// Default to JSON
	return json.Marshal(payload)
}

// applyTemplate applies the configured payload template
func (c *Client) applyTemplate(data map[string]interface{}, template *config.PayloadTemplate) ([]byte, error) {
	// Add template headers to payload metadata if they exist
	if len(template.Headers) > 0 {
		metadata := make(map[string]interface{})
		if existing, ok := data["metadata"]; ok {
			if existingMap, ok := existing.(map[string]interface{}); ok {
				metadata = existingMap
			}
		}

		// Add template headers to metadata
		templateHeaders := make(map[string]string)
		for key, value := range template.Headers {
			templateHeaders[key] = value
		}
		metadata["template_headers"] = templateHeaders
		data["metadata"] = metadata
	}

	switch template.Format {
	case "json", "":
		if template.Template != "" {
			// Process Go template
			return c.processGoTemplate(data, template.Template)
		}
		return json.Marshal(data)
	case "xml":
		if template.Template != "" {
			// Process Go template for XML
			return c.processGoTemplate(data, template.Template)
		}
		// TODO: Implement XML formatting
		c.logger.Warn("XML format not yet implemented, using JSON")
		return json.Marshal(data)
	case "form":
		if template.Template != "" {
			// Process Go template for Form
			return c.processGoTemplate(data, template.Template)
		}
		// TODO: Implement form data formatting
		c.logger.Warn("Form format not yet implemented, using JSON")
		return json.Marshal(data)
	default:
		c.logger.Warn("Unknown payload format, using JSON", zap.String("format", template.Format))
		return json.Marshal(data)
	}
}

// processGoTemplate processes a Go template with the given data
func (c *Client) processGoTemplate(data map[string]interface{}, templateStr string) ([]byte, error) {
	// Create template with custom functions
	tmpl, err := template.New("payload").Funcs(c.getTemplateFunctions()).Parse(templateStr)
	if err != nil {
		c.logger.Error("Failed to parse template", zap.Error(err))
		return nil, fmt.Errorf("template parsing error: %w", err)
	}

	// Execute template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		c.logger.Error("Failed to execute template", zap.Error(err))
		return nil, fmt.Errorf("template execution error: %w", err)
	}

	c.logger.Debug("Template processed successfully",
		zap.String("template", templateStr),
		zap.String("result", buf.String()))

	return buf.Bytes(), nil
}

// getTemplateFunctions returns custom functions for Go templates
func (c *Client) getTemplateFunctions() template.FuncMap {
	return template.FuncMap{
		// Date/time functions
		"now": func() time.Time {
			return time.Now()
		},
		"formatTime": func(layout string, t time.Time) string {
			return t.Format(layout)
		},
		"formatTimeRFC3339": func(t time.Time) string {
			return t.Format(time.RFC3339)
		},
		"formatTimeUnix": func(t time.Time) int64 {
			return t.Unix()
		},
		"formatTimeUnixMilli": func(t time.Time) int64 {
			return t.UnixMilli()
		},

		// JSON functions
		"toJSON": func(v interface{}) (string, error) {
			bytes, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return string(bytes), nil
		},
		"toJSONPretty": func(v interface{}) (string, error) {
			bytes, err := json.MarshalIndent(v, "", "  ")
			if err != nil {
				return "", err
			}
			return string(bytes), nil
		},

		// String functions
		"upper": func(s string) string {
			return strings.ToUpper(s)
		},
		"lower": func(s string) string {
			return strings.ToLower(s)
		},
		"trim": func(s string) string {
			return strings.TrimSpace(s)
		},
		"replace": func(old, new, s string) string {
			return strings.ReplaceAll(s, old, new)
		},
		"contains": func(substr, s string) bool {
			return strings.Contains(s, substr)
		},
		"hasPrefix": func(prefix, s string) bool {
			return strings.HasPrefix(s, prefix)
		},
		"hasSuffix": func(suffix, s string) bool {
			return strings.HasSuffix(s, suffix)
		},

		// Default value function
		"default": func(defaultVal, val interface{}) interface{} {
			if val == nil || val == "" {
				return defaultVal
			}
			return val
		},

		// Conditional functions
		"if": func(condition bool, trueVal, falseVal interface{}) interface{} {
			if condition {
				return trueVal
			}
			return falseVal
		},

		// Math functions
		"add": func(a, b int) int {
			return a + b
		},
		"subtract": func(a, b int) int {
			return a - b
		},
		"multiply": func(a, b int) int {
			return a * b
		},
		"divide": func(a, b int) int {
			if b == 0 {
				return 0
			}
			return a / b
		},
	}
}

// forwardToDestination forwards payload to a specific destination with retry logic
func (c *Client) forwardToDestination(ctx context.Context, dest config.HTTPDestination, payload []byte) error {
	c.logger.Debug("Forwarding event to destination",
		zap.String("destination", dest.Name),
		zap.String("url", dest.URL),
		zap.String("method", dest.Method),
	)

	var lastErr error
	maxAttempts := dest.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1 // At least one attempt
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Create request context with timeout
		reqCtx := ctx
		if dest.Timeout > 0 {
			var cancel context.CancelFunc
			reqCtx, cancel = context.WithTimeout(ctx, dest.Timeout)
			defer cancel()
		}

		// Make the HTTP request
		err := c.makeRequest(reqCtx, dest, payload)
		if err == nil {
			c.logger.Info("Successfully forwarded event",
				zap.String("destination", dest.Name),
				zap.String("url", dest.URL),
				zap.Int("attempt", attempt),
			)
			return nil
		}

		lastErr = err
		c.logger.Warn("Failed to forward event",
			zap.String("destination", dest.Name),
			zap.String("url", dest.URL),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", maxAttempts),
			zap.Error(err),
		)

		// If this was the last attempt, don't wait
		if attempt == maxAttempts {
			break
		}

		// Calculate backoff delay
		delay := c.calculateBackoffDelay(attempt, dest.Retry)
		c.logger.Debug("Retrying after delay",
			zap.String("destination", dest.Name),
			zap.Duration("delay", delay),
			zap.Int("next_attempt", attempt+1),
		)

		// Wait before retry
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("failed after %d attempts, last error: %w", maxAttempts, lastErr)
}

// makeRequest makes a single HTTP request to a destination
func (c *Client) makeRequest(ctx context.Context, dest config.HTTPDestination, payload []byte) error {
	// Log the complete HTTP request body for debugging
	c.logger.Debug("HTTP request body",
		zap.String("destination", dest.Name),
		zap.String("url", dest.URL),
		zap.String("method", dest.Method),
		zap.String("request_body", string(payload)),
		zap.Int("content_length", len(payload)),
	)

	// Create request
	req, err := http.NewRequestWithContext(ctx, dest.Method, dest.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set destination headers first
	for key, value := range dest.Headers {
		req.Header.Set(key, value)
	}

	// Add payload template headers if they exist
	if dest.PayloadTemplate != nil && len(dest.PayloadTemplate.Headers) > 0 {
		for key, value := range dest.PayloadTemplate.Headers {
			req.Header.Set(key, value)
		}
		c.logger.Debug("Applied template headers",
			zap.String("destination", dest.Name),
			zap.Any("template_headers", dest.PayloadTemplate.Headers),
		)
	}

	// Set content length
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(payload)))

	// Log request headers for debugging
	c.logger.Debug("HTTP request headers",
		zap.String("destination", dest.Name),
		zap.Any("headers", req.Header),
	)

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for logging
	body, _ := io.ReadAll(resp.Body)

	// Check status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	c.logger.Debug("HTTP request successful",
		zap.String("destination", dest.Name),
		zap.Int("status_code", resp.StatusCode),
		zap.String("response_body", string(body)),
	)

	return nil
}

// calculateBackoffDelay calculates the delay for the next retry attempt
func (c *Client) calculateBackoffDelay(attempt int, retry config.RetryConfig) time.Duration {
	switch retry.Backoff {
	case "exponential":
		// Exponential backoff: initial_delay * 2^(attempt-1)
		delay := retry.InitialDelay
		for i := 1; i < attempt; i++ {
			delay *= 2
		}
		if delay > retry.MaxDelay {
			delay = retry.MaxDelay
		}
		return delay
	case "linear":
		// Linear backoff: initial_delay * attempt
		delay := retry.InitialDelay * time.Duration(attempt)
		if delay > retry.MaxDelay {
			delay = retry.MaxDelay
		}
		return delay
	case "fixed":
		// Fixed delay
		return retry.InitialDelay
	default:
		// Default to fixed delay
		return retry.InitialDelay
	}
}

// GetDestinationCount returns the number of configured destinations
func (c *Client) GetDestinationCount() int {
	return len(c.destinations)
}

package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	payloadTemplate  *config.PayloadTemplate
	logger           *zap.Logger
}

// NewClient creates a new HTTP client with the given destinations and processors
func NewClient(destinations []config.HTTPDestination, fieldMapper *processor.FieldMapper, processorManager *processor.ProcessorManager, payloadTemplate *config.PayloadTemplate, logger *zap.Logger) *Client {
	return &Client{
		destinations:     destinations,
		fieldMapper:      fieldMapper,
		processorManager: processorManager,
		payloadTemplate:  payloadTemplate,
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

	// Convert event to JSON payload
	payload, err := c.createPayload(event)
	if err != nil {
		return fmt.Errorf("failed to create payload: %w", err)
	}

	// Log payload for debugging
	c.logger.Debug("Created HTTP payload",
		zap.String("event_name", event.Name),
		zap.String("payload", string(payload)),
		zap.Int("eligible_destinations", len(eligibleDestinations)),
	)

	// Forward to eligible destinations in parallel
	errChan := make(chan error, len(eligibleDestinations))

	for _, dest := range eligibleDestinations {
		go func(destination config.HTTPDestination) {
			err := c.forwardToDestination(ctx, destination, payload)
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

	// Build event name with subclass for CUSTOM events
	eventNameWithSubclass := event.Name
	if event.Name == "CUSTOM" && event.Subclass != "" {
		eventNameWithSubclass = event.Name + " " + event.Subclass
	}

	for _, dest := range c.destinations {
		// If no event filters configured, forward all events
		if len(dest.EventFilters) == 0 {
			eligible = append(eligible, dest)
			continue
		}

		// Check if this destination should receive this event
		if c.shouldForwardToDestination(dest, event.Name, eventNameWithSubclass) {
			eligible = append(eligible, dest)
		}
	}

	return eligible
}

// shouldForwardToDestination checks if a destination should receive an event
func (c *Client) shouldForwardToDestination(dest config.HTTPDestination, eventName, eventNameWithSubclass string) bool {
	for _, filter := range dest.EventFilters {
		// Exact match
		if filter == eventName || filter == eventNameWithSubclass {
			return true
		}

		// Wildcard match
		if filter == "*" {
			return true
		}

		// Prefix wildcard match
		if strings.HasSuffix(filter, "*") {
			prefix := strings.TrimSuffix(filter, "*")
			if strings.HasPrefix(eventName, prefix) || strings.HasPrefix(eventNameWithSubclass, prefix) {
				return true
			}
		}
	}

	return false
}

// createPayload converts an event to JSON payload using configured mappers and processors
func (c *Client) createPayload(event *types.Event) ([]byte, error) {
	// Start with field mapping
	payload := c.fieldMapper.MapEvent(event)

	// Run through custom processors
	var err error
	payload, err = c.processorManager.ProcessEvent(event, payload)
	if err != nil {
		c.logger.Warn("Processor failed, using original payload", zap.Error(err))
	}

	// Apply payload template if configured
	if c.payloadTemplate != nil {
		return c.applyTemplate(payload)
	}

	// Default to JSON
	return json.Marshal(payload)
}

// applyTemplate applies the configured payload template
func (c *Client) applyTemplate(data map[string]interface{}) ([]byte, error) {
	switch c.payloadTemplate.Format {
	case "json", "":
		if c.payloadTemplate.Template != "" {
			// TODO: Implement Go template processing
			c.logger.Warn("Custom JSON templates not yet implemented, using default")
		}
		return json.Marshal(data)
	case "xml":
		// TODO: Implement XML formatting
		c.logger.Warn("XML format not yet implemented, using JSON")
		return json.Marshal(data)
	case "form":
		// TODO: Implement form data formatting
		c.logger.Warn("Form format not yet implemented, using JSON")
		return json.Marshal(data)
	default:
		c.logger.Warn("Unknown payload format, using JSON", zap.String("format", c.payloadTemplate.Format))
		return json.Marshal(data)
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
	// Create request
	req, err := http.NewRequestWithContext(ctx, dest.Method, dest.URL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	for key, value := range dest.Headers {
		req.Header.Set(key, value)
	}

	// Set content length
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(payload)))

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

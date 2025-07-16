package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// Validate validates the entire configuration
func (c *Config) Validate() error {
	if err := c.ESL.Validate(); err != nil {
		return fmt.Errorf("ESL configuration error: %w", err)
	}

	if err := c.Events.Validate(); err != nil {
		return fmt.Errorf("Events configuration error: %w", err)
	}

	if err := c.HTTP.Validate(); err != nil {
		return fmt.Errorf("HTTP configuration error: %w", err)
	}

	if err := c.Logging.Validate(); err != nil {
		return fmt.Errorf("Logging configuration error: %w", err)
	}

	if err := c.Metrics.Validate(); err != nil {
		return fmt.Errorf("Metrics configuration error: %w", err)
	}

	return nil
}

// Validate validates ESL configuration
func (e *ESLConfig) Validate() error {
	if e.Host == "" {
		return fmt.Errorf("host cannot be empty")
	}

	if e.Port <= 0 || e.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", e.Port)
	}

	if e.Password == "" {
		return fmt.Errorf("password cannot be empty")
	}

	if e.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got %v", e.Timeout)
	}

	if e.ReconnectInterval <= 0 {
		return fmt.Errorf("reconnect_interval must be positive, got %v", e.ReconnectInterval)
	}

	if e.MaxReconnectAttempts < 0 {
		return fmt.Errorf("max_reconnect_attempts cannot be negative, got %d", e.MaxReconnectAttempts)
	}

	return nil
}

// Validate validates Events configuration
func (e *EventsConfig) Validate() error {
	if len(e.SubscribeEvents) == 0 {
		return fmt.Errorf("subscribe_events cannot be empty")
	}

	// Validate event names
	validEvents := map[string]bool{
		"CHANNEL_CREATE":          true,
		"CHANNEL_DESTROY":         true,
		"CHANNEL_ANSWER":          true,
		"CHANNEL_HANGUP":          true,
		"CHANNEL_BRIDGE":          true,
		"CHANNEL_UNBRIDGE":        true,
		"CHANNEL_PROGRESS":        true,
		"CHANNEL_OUTGOING":        true,
		"CHANNEL_PARK":            true,
		"CHANNEL_UNPARK":          true,
		"API":                     true,
		"LOG":                     true,
		"INBOUND_CHAN":            true,
		"OUTBOUND_CHAN":           true,
		"STARTUP":                 true,
		"SHUTDOWN":                true,
		"PUBLISH":                 true,
		"UNPUBLISH":               true,
		"TALK":                    true,
		"NOTALK":                  true,
		"SESSION_CRASH":           true,
		"MODULE_LOAD":             true,
		"MODULE_UNLOAD":           true,
		"DTMF":                    true,
		"MESSAGE":                 true,
		"PRESENCE_IN":             true,
		"NOTIFY_IN":               true,
		"PRESENCE_OUT":            true,
		"PRESENCE_PROBE":          true,
		"MESSAGE_WAITING":         true,
		"MESSAGE_QUERY":           true,
		"ROSTER":                  true,
		"CODEC":                   true,
		"BACKGROUND_JOB":          true,
		"DETECTED_SPEECH":         true,
		"DETECTED_TONE":           true,
		"PRIVATE_COMMAND":         true,
		"HEARTBEAT":               true,
		"TRAP":                    true,
		"ADD_SCHEDULE":            true,
		"DEL_SCHEDULE":            true,
		"EXE_SCHEDULE":            true,
		"RE_SCHEDULE":             true,
		"RELOADXML":               true,
		"NOTIFY":                  true,
		"PHONE_FEATURE":           true,
		"PHONE_FEATURE_SUBSCRIBE": true,
		"SEND_MESSAGE":            true,
		"RECV_MESSAGE":            true,
		"REQUEST_PARAMS":          true,
		"CHANNEL_DATA":            true,
		"GENERAL":                 true,
		"COMMAND":                 true,
		"SESSION_HEARTBEAT":       true,
		"CLIENT_DISCONNECTED":     true,
		"SERVER_DISCONNECTED":     true,
		"SEND_INFO":               true,
		"RECV_INFO":               true,
		"RECV_RTCP_MESSAGE":       true,
		"CALL_SECURE":             true,
		"NAT":                     true,
		"RECORD_START":            true,
		"RECORD_STOP":             true,
		"PLAYBACK_START":          true,
		"PLAYBACK_STOP":           true,
		"CALL_UPDATE":             true,
		"FAILURE":                 true,
		"SOCKET_DATA":             true,
		"MEDIA_BUG_START":         true,
		"MEDIA_BUG_STOP":          true,
		"CONFERENCE_DATA_QUERY":   true,
		"CONFERENCE_DATA":         true,
		"CALL_SETUP_REQ":          true,
		"CALL_SETUP_RESULT":       true,
		"CALL_DETAIL":             true,
		"DEVICE_STATE":            true,
		"CUSTOM":                  true, // Base CUSTOM event
		"ALL":                     true,
	}

	for _, event := range e.SubscribeEvents {
		// Check if it's a CUSTOM event with subclass (e.g., "CUSTOM callcenter::info", "CUSTOM my-event")
		if strings.HasPrefix(event, "CUSTOM ") {
			// Extract subclass and validate format
			subclass := strings.TrimPrefix(event, "CUSTOM ")
			if subclass == "" {
				return fmt.Errorf("CUSTOM event must have a subclass: %s", event)
			}
			// Validate characters in subclass (alphanumeric, underscore, colon, hyphen, dot, space)
			validSubclassPattern := regexp.MustCompile(`^[a-zA-Z0-9_:\-\.\s]+$`)
			if !validSubclassPattern.MatchString(subclass) {
				return fmt.Errorf("invalid characters in CUSTOM event subclass: %s", event)
			}
		} else if !validEvents[event] {
			return fmt.Errorf("invalid event name: %s", event)
		}
	}

	// Validate filter logic
	if e.FilterLogic != "" && e.FilterLogic != "AND" && e.FilterLogic != "OR" {
		return fmt.Errorf("filter_logic must be 'AND' or 'OR', got: %s", e.FilterLogic)
	}

	// Validate filters
	for i, filter := range e.Filters {
		if err := filter.Validate(); err != nil {
			return fmt.Errorf("filter %d error: %w", i, err)
		}
	}

	// Validate field mappings
	for i, mapping := range e.FieldMappings {
		if err := mapping.Validate(); err != nil {
			return fmt.Errorf("field mapping %d error: %w", i, err)
		}
	}

	// Validate event field mappings
	for i, eventMapping := range e.EventFieldMappings {
		if err := eventMapping.Validate(); err != nil {
			return fmt.Errorf("event field mapping %d error: %w", i, err)
		}
	}

	// Validate global field filters
	if e.FieldFilters != nil {
		if err := e.FieldFilters.Validate(); err != nil {
			return fmt.Errorf("global field filters error: %w", err)
		}
	}

	return nil
}

// Validate validates FilterRule configuration
func (f *FilterRule) Validate() error {
	if f.Field == "" {
		return fmt.Errorf("field cannot be empty")
	}

	validOperators := map[string]bool{
		"equals":       true,
		"not_equals":   true,
		"contains":     true,
		"not_contains": true,
		"regex":        true,
		"not_regex":    true,
		"starts_with":  true,
		"ends_with":    true,
		"exists":       true,
		"not_exists":   true,
	}

	if !validOperators[f.Operator] {
		return fmt.Errorf("invalid operator: %s", f.Operator)
	}

	// For regex operators, validate the regex pattern
	if f.Operator == "regex" || f.Operator == "not_regex" {
		if f.Value == "" {
			return fmt.Errorf("regex value cannot be empty")
		}
		if _, err := regexp.Compile(f.Value); err != nil {
			return fmt.Errorf("invalid regex pattern: %w", err)
		}
	}

	// For exists/not_exists operators, value should be empty
	if (f.Operator == "exists" || f.Operator == "not_exists") && f.Value != "" {
		return fmt.Errorf("exists/not_exists operators should not have a value")
	}

	return nil
}

// Validate validates HTTP configuration
func (h *HTTPConfig) Validate() error {
	if len(h.Destinations) == 0 {
		return fmt.Errorf("at least one HTTP destination must be configured")
	}

	for i, dest := range h.Destinations {
		if err := dest.Validate(); err != nil {
			return fmt.Errorf("destination %d (%s) error: %w", i, dest.Name, err)
		}
	}

	return nil
}

// Validate validates HTTPDestination configuration
func (d *HTTPDestination) Validate() error {
	if d.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}

	if d.URL == "" {
		return fmt.Errorf("URL cannot be empty")
	}

	// Validate URL format
	if _, err := url.Parse(d.URL); err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	// Validate HTTP method
	validMethods := map[string]bool{
		"GET":     true,
		"POST":    true,
		"PUT":     true,
		"PATCH":   true,
		"DELETE":  true,
		"HEAD":    true,
		"OPTIONS": true,
	}

	method := strings.ToUpper(d.Method)
	if !validMethods[method] {
		return fmt.Errorf("invalid HTTP method: %s", d.Method)
	}

	if d.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got %v", d.Timeout)
	}

	// Validate event filters
	for _, filter := range d.EventFilters {
		if filter == "" {
			return fmt.Errorf("event filter cannot be empty for destination %s", d.Name)
		}
		// Basic validation for event names
		if !isValidEventFilter(filter) {
			return fmt.Errorf("invalid event filter '%s' for destination %s", filter, d.Name)
		}
	}

	if err := d.Retry.Validate(); err != nil {
		return fmt.Errorf("retry configuration error: %w", err)
	}

	// Validate payload template if provided
	if d.PayloadTemplate != nil {
		if err := d.PayloadTemplate.Validate(); err != nil {
			return fmt.Errorf("payload template error: %w", err)
		}
	}

	return nil
}

// isValidEventFilter validates event filter format
func isValidEventFilter(filter string) bool {
	// Allow wildcard
	if filter == "*" {
		return true
	}

	// Allow prefix wildcard (e.g., "CHANNEL_*")
	if strings.HasSuffix(filter, "*") {
		prefix := strings.TrimSuffix(filter, "*")
		return len(prefix) > 0 && isValidEventName(prefix)
	}

	// Regular event name validation
	return isValidEventName(filter)
}

// isValidEventName validates event name format
func isValidEventName(name string) bool {
	// Allow CUSTOM events with subclass
	if strings.HasPrefix(name, "CUSTOM ") {
		return len(name) > 7 // "CUSTOM " + subclass
	}

	// Regular event names should be uppercase and contain only letters, numbers, and underscores
	if len(name) == 0 {
		return false
	}

	for _, char := range name {
		if !((char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '_' || char == ':') {
			return false
		}
	}

	return true
}

// Validate validates RetryConfig configuration
func (r *RetryConfig) Validate() error {
	if r.MaxAttempts < 0 {
		return fmt.Errorf("max_attempts cannot be negative, got %d", r.MaxAttempts)
	}

	if r.MaxAttempts > 0 {
		validBackoffs := map[string]bool{
			"fixed":       true,
			"linear":      true,
			"exponential": true,
		}

		if !validBackoffs[r.Backoff] {
			return fmt.Errorf("invalid backoff strategy: %s", r.Backoff)
		}

		if r.InitialDelay <= 0 {
			return fmt.Errorf("initial_delay must be positive when retries are enabled, got %v", r.InitialDelay)
		}

		if r.MaxDelay <= 0 {
			return fmt.Errorf("max_delay must be positive when retries are enabled, got %v", r.MaxDelay)
		}

		if r.InitialDelay > r.MaxDelay {
			return fmt.Errorf("initial_delay (%v) cannot be greater than max_delay (%v)", r.InitialDelay, r.MaxDelay)
		}
	}

	return nil
}

// Validate validates LoggingConfig configuration
func (l *LoggingConfig) Validate() error {
	validLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
		"fatal": true,
		"panic": true,
	}

	if !validLevels[strings.ToLower(l.Level)] {
		return fmt.Errorf("invalid log level: %s", l.Level)
	}

	validFormats := map[string]bool{
		"json":    true,
		"console": true,
	}

	if !validFormats[strings.ToLower(l.Format)] {
		return fmt.Errorf("invalid log format: %s", l.Format)
	}

	if l.Output == "" {
		return fmt.Errorf("output cannot be empty")
	}

	return nil
}

// Validate validates MetricsConfig configuration
func (m *MetricsConfig) Validate() error {
	if m.Enabled {
		if m.Port <= 0 || m.Port > 65535 {
			return fmt.Errorf("port must be between 1 and 65535, got %d", m.Port)
		}

		if m.Path == "" {
			return fmt.Errorf("path cannot be empty when metrics are enabled")
		}

		if !strings.HasPrefix(m.Path, "/") {
			return fmt.Errorf("path must start with '/', got %s", m.Path)
		}
	}

	return nil
}

// Validate validates FieldMapping configuration
func (f *FieldMapping) Validate() error {
	if f.From == "" {
		return fmt.Errorf("from field cannot be empty")
	}

	if f.To == "" {
		return fmt.Errorf("to field cannot be empty")
	}

	// Validate transform types
	validTransforms := map[string]bool{
		"lowercase":     true,
		"uppercase":     true,
		"trim":          true,
		"reverse":       true,
		"first_word":    true,
		"last_word":     true,
		"url_encode":    true,
		"url_decode":    true,
		"base64_encode": true,
		"base64_decode": true,
		":int":          true,
		":float":        true,
		":bool":         true,
		":millis":       true,
		":round":        true,
		"":              true, // Empty transform is valid
	}

	for i, transform := range f.Transforms {
		// Check regex replacement pattern
		if strings.HasPrefix(transform, "s/") && strings.Count(transform, "/") >= 2 {
			parts := strings.Split(transform[2:], "/")
			if len(parts) >= 2 {
				pattern := parts[0]
				if _, err := regexp.Compile(pattern); err != nil {
					return fmt.Errorf("invalid regex pattern in transform %d: %w", i, err)
				}
			}
		} else if strings.HasPrefix(transform, ":add:") || strings.HasPrefix(transform, ":subtract:") ||
			strings.HasPrefix(transform, ":multiply:") || strings.HasPrefix(transform, ":divide:") {
			// Validate math operations format
			if err := validateMathTransform(transform); err != nil {
				return fmt.Errorf("invalid math transform in transform %d: %w", i, err)
			}
		} else if !validTransforms[transform] {
			return fmt.Errorf("invalid transform type in transform %d: %s", i, transform)
		}
	}

	// Validate event types if specified
	for i, eventType := range f.EventTypes {
		if !isValidEventFilter(eventType) {
			return fmt.Errorf("invalid event type %d: %s", i, eventType)
		}
	}

	return nil
}

// Validate validates EventFieldMappings configuration
func (e *EventFieldMappings) Validate() error {
	if len(e.EventTypes) == 0 {
		return fmt.Errorf("event_types cannot be empty")
	}

	// Validate event types
	for i, eventType := range e.EventTypes {
		if !isValidEventFilter(eventType) {
			return fmt.Errorf("invalid event type %d: %s", i, eventType)
		}
	}

	// Validate mappings
	for i, mapping := range e.Mappings {
		if err := mapping.Validate(); err != nil {
			return fmt.Errorf("mapping %d error: %w", i, err)
		}
	}

	// Validate field filters
	if e.FieldFilters != nil {
		if err := e.FieldFilters.Validate(); err != nil {
			return fmt.Errorf("field filters error: %w", err)
		}
	}

	return nil
}

// Validate validates FieldFilter configuration
func (f *FieldFilter) Validate() error {
	// Validate include/exclude header patterns
	for i, pattern := range f.IncludeHeaders {
		if pattern == "" {
			return fmt.Errorf("include header pattern %d cannot be empty", i)
		}
		if !isValidPattern(pattern) {
			return fmt.Errorf("invalid include header pattern %d: %s", i, pattern)
		}
	}

	for i, pattern := range f.ExcludeHeaders {
		if pattern == "" {
			return fmt.Errorf("exclude header pattern %d cannot be empty", i)
		}
		if !isValidPattern(pattern) {
			return fmt.Errorf("invalid exclude header pattern %d: %s", i, pattern)
		}
	}

	// Validate include/exclude field patterns
	for i, pattern := range f.IncludeFields {
		if pattern == "" {
			return fmt.Errorf("include field pattern %d cannot be empty", i)
		}
		if !isValidPattern(pattern) {
			return fmt.Errorf("invalid include field pattern %d: %s", i, pattern)
		}
	}

	for i, pattern := range f.ExcludeFields {
		if pattern == "" {
			return fmt.Errorf("exclude field pattern %d cannot be empty", i)
		}
		if !isValidPattern(pattern) {
			return fmt.Errorf("invalid exclude field pattern %d: %s", i, pattern)
		}
	}

	return nil
}

// isValidPattern validates field/header pattern format (supports wildcards)
func isValidPattern(pattern string) bool {
	if pattern == "" {
		return false
	}

	// Allow wildcard
	if pattern == "*" {
		return true
	}

	// Basic pattern validation - allow alphanumeric, underscore, hyphen, dot, colon, and wildcards
	validPatternRegex := regexp.MustCompile(`^[a-zA-Z0-9_\-\.\:\*]+$`)
	return validPatternRegex.MatchString(pattern)
}

// validateMathTransform validates math operation transforms like :add:10, :divide:1000
func validateMathTransform(transform string) error {
	// Parse the transform format: :operation:operand
	parts := strings.Split(transform, ":")
	if len(parts) != 3 {
		return fmt.Errorf("invalid math transform format: %s (expected format: :operation:operand)", transform)
	}

	operation := parts[1]
	operandStr := parts[2]

	// Validate operation
	validOperations := map[string]bool{
		"add":      true,
		"subtract": true,
		"multiply": true,
		"divide":   true,
	}

	if !validOperations[operation] {
		return fmt.Errorf("invalid math operation: %s (valid: add, subtract, multiply, divide)", operation)
	}

	// Validate operand is a valid number
	if _, err := strconv.ParseFloat(operandStr, 64); err != nil {
		return fmt.Errorf("invalid operand '%s': must be a valid number", operandStr)
	}

	// Special validation for division by zero
	if operation == "divide" {
		if operand, _ := strconv.ParseFloat(operandStr, 64); operand == 0 {
			return fmt.Errorf("division by zero is not allowed")
		}
	}

	return nil
}

// Validate validates PayloadTemplate configuration
func (p *PayloadTemplate) Validate() error {
	if p == nil {
		return nil
	}

	// Validate format
	validFormats := map[string]bool{
		"json": true,
		"xml":  true,
		"form": true,
		"":     true, // Default to json
	}

	if !validFormats[p.Format] {
		return fmt.Errorf("invalid payload format: %s (supported: json, xml, form)", p.Format)
	}

	// Validate template syntax if provided
	if p.Template != "" {
		// Parse the template string with custom functions
		tmpl := template.New("payload_template").Funcs(getTemplateFunctions())
		_, err := tmpl.Parse(p.Template)
		if err != nil {
			return fmt.Errorf("invalid template syntax: %w", err)
		}
	}

	// Validate headers
	for key, value := range p.Headers {
		if key == "" {
			return fmt.Errorf("header key cannot be empty")
		}
		if value == "" {
			return fmt.Errorf("header value cannot be empty for key: %s", key)
		}
	}

	return nil
}

// getTemplateFunctions returns custom functions for Go template validation
func getTemplateFunctions() template.FuncMap {
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

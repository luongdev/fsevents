package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
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

	// Validate filters
	for i, filter := range e.Filters {
		if err := filter.Validate(); err != nil {
			return fmt.Errorf("filter %d error: %w", i, err)
		}
	}

	return nil
}

// Validate validates EventFilter configuration
func (f *EventFilter) Validate() error {
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

	if err := d.Retry.Validate(); err != nil {
		return fmt.Errorf("retry configuration error: %w", err)
	}

	return nil
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

package types

import (
	"time"
)

// Event represents a FreeSWITCH event
type Event struct {
	// Core event fields
	Name      string            `json:"event_name"`
	Subclass  string            `json:"event_subclass,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
	Headers   map[string]string `json:"headers"`
	Body      string            `json:"body,omitempty"`

	// Processing metadata
	ReceivedAt   time.Time `json:"received_at"`
	ProcessedAt  time.Time `json:"processed_at,omitempty"`
	Filtered     bool      `json:"filtered"`
	FilterReason string    `json:"filter_reason,omitempty"`
}

// Channel represents channel information from events
type Channel struct {
	UUID              string `json:"uuid"`
	Direction         string `json:"direction"`
	State             string `json:"state"`
	CallerIDName      string `json:"caller_id_name"`
	CallerIDNumber    string `json:"caller_id_number"`
	DestinationNumber string `json:"destination_number"`
	Context           string `json:"context"`
	DialPlan          string `json:"dialplan"`
	CreatedTime       string `json:"created_time"`
	AnsweredTime      string `json:"answered_time,omitempty"`
	HangupTime        string `json:"hangup_time,omitempty"`
	HangupCause       string `json:"hangup_cause,omitempty"`
}

// CustomEvent represents a CUSTOM event with structured data
type CustomEvent struct {
	Event
	Subclass string                 `json:"subclass"`
	Data     map[string]interface{} `json:"data"`
}

// EventStats represents statistics about event processing
type EventStats struct {
	TotalReceived   int64     `json:"total_received"`
	TotalFiltered   int64     `json:"total_filtered"`
	TotalForwarded  int64     `json:"total_forwarded"`
	TotalErrors     int64     `json:"total_errors"`
	LastEventTime   time.Time `json:"last_event_time"`
	EventsPerSecond float64   `json:"events_per_second"`
}

// NewEvent creates a new Event from FreeSWITCH event data
func NewEvent(name string, headers map[string]string, body string) *Event {
	now := time.Now()

	event := &Event{
		Name:       name,
		Headers:    headers,
		Body:       body,
		ReceivedAt: now,
		Timestamp:  now,
	}

	// Extract common fields
	if subclass, ok := headers["Event-Subclass"]; ok {
		event.Subclass = subclass
	}

	// Parse timestamp if available
	if tsStr, ok := headers["Event-Date-Timestamp"]; ok {
		if ts, err := time.Parse("1504569600000000", tsStr); err == nil {
			event.Timestamp = ts
		}
	}

	return event
}

// GetHeader returns a header value, with optional default
func (e *Event) GetHeader(key string, defaultValue ...string) string {
	if value, ok := e.Headers[key]; ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	return ""
}

// HasHeader checks if a header exists
func (e *Event) HasHeader(key string) bool {
	_, ok := e.Headers[key]
	return ok
}

// IsChannelEvent returns true if this is a channel-related event
func (e *Event) IsChannelEvent() bool {
	channelEvents := map[string]bool{
		"CHANNEL_CREATE":   true,
		"CHANNEL_DESTROY":  true,
		"CHANNEL_ANSWER":   true,
		"CHANNEL_HANGUP":   true,
		"CHANNEL_BRIDGE":   true,
		"CHANNEL_UNBRIDGE": true,
		"CHANNEL_PROGRESS": true,
		"CHANNEL_OUTGOING": true,
		"CHANNEL_PARK":     true,
		"CHANNEL_UNPARK":   true,
	}
	return channelEvents[e.Name]
}

// IsCustomEvent returns true if this is a CUSTOM event
func (e *Event) IsCustomEvent() bool {
	return e.Name == "CUSTOM"
}

// GetChannelInfo extracts channel information from the event
func (e *Event) GetChannelInfo() *Channel {
	if !e.IsChannelEvent() {
		return nil
	}

	return &Channel{
		UUID:              e.GetHeader("Unique-ID"),
		Direction:         e.GetHeader("Call-Direction"),
		State:             e.GetHeader("Channel-State"),
		CallerIDName:      e.GetHeader("Caller-Caller-ID-Name"),
		CallerIDNumber:    e.GetHeader("Caller-Caller-ID-Number"),
		DestinationNumber: e.GetHeader("Caller-Destination-Number"),
		Context:           e.GetHeader("Caller-Context"),
		DialPlan:          e.GetHeader("Caller-Dialplan"),
		CreatedTime:       e.GetHeader("Caller-Channel-Created-Time"),
		AnsweredTime:      e.GetHeader("Caller-Channel-Answered-Time"),
		HangupTime:        e.GetHeader("Caller-Channel-Hangup-Time"),
		HangupCause:       e.GetHeader("Hangup-Cause"),
	}
}

// ToMap converts the event to a map for JSON serialization
func (e *Event) ToMap() map[string]interface{} {
	result := map[string]interface{}{
		"event_name":  e.Name,
		"timestamp":   e.Timestamp.Format(time.RFC3339),
		"received_at": e.ReceivedAt.Format(time.RFC3339),
		"headers":     e.Headers,
		"filtered":    e.Filtered,
	}

	if e.Subclass != "" {
		result["event_subclass"] = e.Subclass
	}

	if e.Body != "" {
		result["body"] = e.Body
	}

	if !e.ProcessedAt.IsZero() {
		result["processed_at"] = e.ProcessedAt.Format(time.RFC3339)
	}

	if e.FilterReason != "" {
		result["filter_reason"] = e.FilterReason
	}

	// Add channel info if available
	if channel := e.GetChannelInfo(); channel != nil {
		result["channel"] = channel
	}

	return result
}

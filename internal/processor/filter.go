package processor

import (
	"regexp"
	"strings"

	"go.uber.org/zap"

	"fsevents/internal/config"
	"fsevents/pkg/types"
)

// EventFilter handles filtering of events based on configuration rules
type EventFilter struct {
	filters []config.FilterRule
	logger  *zap.Logger
}

// NewEventFilter creates a new event filter with the given rules
func NewEventFilter(filters []config.FilterRule, logger *zap.Logger) *EventFilter {
	return &EventFilter{
		filters: filters,
		logger:  logger.Named("filter"),
	}
}

// ShouldProcess determines if an event should be processed based on filter rules
func (f *EventFilter) ShouldProcess(event *types.Event) bool {
	// If no filters configured, process all events
	if len(f.filters) == 0 {
		return true
	}

	// Apply all filter rules - event must match ALL rules (AND logic)
	for _, filter := range f.filters {
		if !f.evaluateFilter(event, filter) {
			f.logger.Debug("Event filtered out",
				zap.String("event_name", event.Name),
				zap.String("filter_field", filter.Field),
				zap.String("filter_operator", filter.Operator),
				zap.String("filter_value", filter.Value),
			)
			return false
		}
	}

	f.logger.Debug("Event passed all filters",
		zap.String("event_name", event.Name),
		zap.Int("filter_count", len(f.filters)),
	)
	return true
}

// evaluateFilter evaluates a single filter rule against an event
func (f *EventFilter) evaluateFilter(event *types.Event, filter config.FilterRule) bool {
	// Get the actual value from the event
	var actualValue string

	switch filter.Field {
	case "Event-Name":
		actualValue = event.Name
	case "Event-Subclass":
		actualValue = event.Subclass
	default:
		// Try to get from headers
		actualValue = event.GetHeader(filter.Field)
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
			f.logger.Error("Invalid regex pattern",
				zap.String("field", filter.Field),
				zap.String("pattern", filter.Value),
				zap.Error(err),
			)
			return false
		}

		matched := regex.MatchString(actualValue)
		f.logger.Debug("Regex filter evaluation",
			zap.String("field", filter.Field),
			zap.String("pattern", filter.Value),
			zap.String("actual_value", actualValue),
			zap.Bool("matched", matched),
		)
		return matched
	default:
		f.logger.Warn("Unknown filter operator",
			zap.String("operator", filter.Operator),
			zap.String("field", filter.Field),
		)
		return false
	}
}

// GetFilterCount returns the number of configured filters
func (f *EventFilter) GetFilterCount() int {
	return len(f.filters)
}

package processor

import (
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"fsevents/internal/config"
	"fsevents/pkg/types"
)

// FieldMapper handles mapping of event headers to output fields
type FieldMapper struct {
	mappings []config.FieldMapping
	logger   *zap.Logger
}

// NewFieldMapper creates a new field mapper with the given mappings
func NewFieldMapper(mappings []config.FieldMapping, logger *zap.Logger) *FieldMapper {
	return &FieldMapper{
		mappings: mappings,
		logger:   logger.Named("mapper"),
	}
}

// MapEvent maps event headers to output fields based on configuration
func (m *FieldMapper) MapEvent(event *types.Event) map[string]interface{} {
	result := make(map[string]interface{})

	// Always include basic event info
	result["timestamp"] = event.Timestamp.UTC().Format(time.RFC3339)
	result["event_name"] = event.Name

	if event.Subclass != "" {
		result["event_subclass"] = event.Subclass
	}

	// Apply configured field mappings
	for _, mapping := range m.mappings {
		value := m.extractValue(event, mapping)
		if value != nil {
			result[mapping.To] = value
		}
	}

	// If no mappings configured, include all headers as fallback
	if len(m.mappings) == 0 {
		result["headers"] = event.Headers
		if event.Body != "" {
			result["body"] = event.Body
		}
	}

	return result
}

// extractValue extracts and transforms a value from event based on field mapping
func (m *FieldMapper) extractValue(event *types.Event, mapping config.FieldMapping) interface{} {
	// Get raw value from event
	var rawValue string

	// Check if it's a special field
	switch mapping.From {
	case "Event-Name":
		rawValue = event.Name
	case "Event-Subclass":
		rawValue = event.Subclass
	case "Event-Body":
		rawValue = event.Body
	case "Event-Timestamp":
		return event.Timestamp.UTC().Format(time.RFC3339)
	default:
		// Regular header lookup
		rawValue = event.GetHeader(mapping.From)
	}

	// Use default if value is empty and default is provided
	if rawValue == "" && mapping.DefaultValue != "" {
		rawValue = mapping.DefaultValue
	}

	// Skip if still empty
	if rawValue == "" {
		return nil
	}

	// Apply transformation
	transformedValue := m.applyTransform(rawValue, mapping.Transform)

	// Try to convert to appropriate type
	return m.convertType(transformedValue, mapping.Transform)
}

// applyTransform applies string transformations
func (m *FieldMapper) applyTransform(value, transform string) string {
	switch transform {
	case "lowercase":
		return strings.ToLower(value)
	case "uppercase":
		return strings.ToUpper(value)
	case "trim":
		return strings.TrimSpace(value)
	case "":
		return value // No transformation
	default:
		m.logger.Warn("Unknown transform type", zap.String("transform", transform))
		return value
	}
}

// convertType attempts to convert string to appropriate type based on hints
func (m *FieldMapper) convertType(value, transform string) interface{} {
	// Check for type conversion hints in transform
	switch transform {
	case ":int":
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
		m.logger.Warn("Failed to convert to int", zap.String("value", value))
	case ":float":
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
		m.logger.Warn("Failed to convert to float", zap.String("value", value))
	case ":bool":
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
		m.logger.Warn("Failed to convert to bool", zap.String("value", value))
	}

	// Return as string by default
	return value
}

// GetMappingCount returns the number of configured mappings
func (m *FieldMapper) GetMappingCount() int {
	return len(m.mappings)
}

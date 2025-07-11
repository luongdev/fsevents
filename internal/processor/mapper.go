package processor

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"fsevents/internal/config"
	"fsevents/pkg/types"
)

// FieldMapper handles mapping of event headers to output fields
type FieldMapper struct {
	globalMappings []config.FieldMapping
	eventMappings  []config.EventFieldMappings
	globalFilters  *config.FieldFilter
	logger         *zap.Logger
}

// NewFieldMapper creates a new field mapper with the given mappings and filters
func NewFieldMapper(globalMappings []config.FieldMapping, eventMappings []config.EventFieldMappings, globalFilters *config.FieldFilter, logger *zap.Logger) *FieldMapper {
	return &FieldMapper{
		globalMappings: globalMappings,
		eventMappings:  eventMappings,
		globalFilters:  globalFilters,
		logger:         logger.Named("mapper"),
	}
}

// MapEvent maps event headers to output fields based on configuration and applies field filtering
func (m *FieldMapper) MapEvent(event *types.Event) map[string]interface{} {
	result := make(map[string]interface{})

	// Always include basic event info - default to RFC3339 format
	result["timestamp"] = event.Timestamp.UTC().Format(time.RFC3339)
	result["event_name"] = event.Name

	if event.Subclass != "" {
		result["event_subclass"] = event.Subclass
	}

	// Get applicable mappings and filters for this event
	applicableMappings := m.getApplicableMappings(event)
	applicableFilters := m.getApplicableFilters(event)

	// Apply field mappings
	for _, mapping := range applicableMappings {
		value := m.extractValue(event, mapping)
		if value != nil {
			result[mapping.To] = value
		}
	}

	// If no mappings configured, include filtered headers as fallback
	if len(applicableMappings) == 0 {
		filteredHeaders := m.filterHeaders(event.Headers, applicableFilters)
		if len(filteredHeaders) > 0 {
			result["headers"] = filteredHeaders
		}

		// Include body if allowed by filters
		if event.Body != "" && m.shouldIncludeBody(applicableFilters) {
			result["body"] = event.Body
		}
	}

	// Apply field filtering to the final result
	result = m.filterFields(result, applicableFilters)

	return result
}

// getApplicableMappings returns all mappings that apply to the given event
func (m *FieldMapper) getApplicableMappings(event *types.Event) []config.FieldMapping {
	var result []config.FieldMapping

	// Add global mappings (those without event_types specified)
	for _, mapping := range m.globalMappings {
		if len(mapping.EventTypes) == 0 || m.matchesEventTypes(event, mapping.EventTypes) {
			result = append(result, mapping)
		}
	}

	// Add event-specific mappings
	for _, eventMapping := range m.eventMappings {
		if m.matchesEventTypes(event, eventMapping.EventTypes) {
			for _, mapping := range eventMapping.Mappings {
				// Override event types for individual mappings if not specified
				if len(mapping.EventTypes) == 0 {
					mapping.EventTypes = eventMapping.EventTypes
				}
				result = append(result, mapping)
			}
		}
	}

	return result
}

// matchesEventTypes checks if event matches any of the specified event types
func (m *FieldMapper) matchesEventTypes(event *types.Event, eventTypes []string) bool {
	eventName := event.Name
	if event.Subclass != "" {
		eventName = event.Name + " " + event.Subclass
	}

	for _, eventType := range eventTypes {
		if m.matchesEventType(eventName, eventType) {
			return true
		}
	}
	return false
}

// matchesEventType checks if event matches a specific event type pattern
func (m *FieldMapper) matchesEventType(eventName, eventType string) bool {
	// Exact match
	if eventName == eventType {
		return true
	}

	// Wildcard match
	if eventType == "*" {
		return true
	}

	// Prefix match (e.g., "CHANNEL_*")
	if strings.HasSuffix(eventType, "*") {
		prefix := strings.TrimSuffix(eventType, "*")
		return strings.HasPrefix(eventName, prefix)
	}

	// For CUSTOM events, check subclass matching
	if strings.HasPrefix(eventName, "CUSTOM ") && strings.HasPrefix(eventType, "CUSTOM ") {
		eventSubclass := strings.TrimPrefix(eventName, "CUSTOM ")
		typeSubclass := strings.TrimPrefix(eventType, "CUSTOM ")

		if strings.HasSuffix(typeSubclass, "*") {
			prefix := strings.TrimSuffix(typeSubclass, "*")
			return strings.HasPrefix(eventSubclass, prefix)
		}
		return eventSubclass == typeSubclass
	}

	return false
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

	// Apply multiple transformations in sequence
	transformedValue := m.applyTransforms(rawValue, mapping.Transforms)

	return transformedValue
}

// applyTransforms applies multiple transformations in sequence
func (m *FieldMapper) applyTransforms(value string, transforms []string) interface{} {
	result := value

	for _, transform := range transforms {
		result = m.applyTransform(result, transform)
	}

	// Check for type conversion in the last transform
	if len(transforms) > 0 {
		lastTransform := transforms[len(transforms)-1]
		return m.convertType(result, lastTransform)
	}

	return result
}

// applyTransform applies a single transformation
func (m *FieldMapper) applyTransform(value, transform string) string {
	switch transform {
	case "lowercase":
		return strings.ToLower(value)
	case "uppercase":
		return strings.ToUpper(value)
	case "trim":
		return strings.TrimSpace(value)
	case "reverse":
		runes := []rune(value)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		return string(runes)
	case "first_word":
		words := strings.Fields(value)
		if len(words) > 0 {
			return words[0]
		}
		return value
	case "last_word":
		words := strings.Fields(value)
		if len(words) > 0 {
			return words[len(words)-1]
		}
		return value
	case "url_encode":
		return url.QueryEscape(value)
	case "url_decode":
		if decoded, err := url.QueryUnescape(value); err == nil {
			return decoded
		}
		return value
	case "base64_encode":
		return base64.StdEncoding.EncodeToString([]byte(value))
	case "base64_decode":
		if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
			return string(decoded)
		}
		return value
	case "":
		return value // No transformation
	case ":int", ":float", ":bool", ":millis":
		return value // Type conversion happens in convertType, not here
	default:
		// Check for regex replacement (s/pattern/replacement/)
		if strings.HasPrefix(transform, "s/") && strings.Count(transform, "/") >= 2 {
			parts := strings.Split(transform[2:], "/")
			if len(parts) >= 2 {
				pattern := parts[0]
				replacement := parts[1]
				if regex, err := regexp.Compile(pattern); err == nil {
					return regex.ReplaceAllString(value, replacement)
				}
			}
		}
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
	case ":millis":
		// Convert date string to milliseconds timestamp
		if millis, err := m.parseToMilliseconds(value); err == nil {
			return millis
		}
		m.logger.Warn("Failed to convert to milliseconds", zap.String("value", value))
	}

	// Return as string by default
	return value
}

// parseToMilliseconds parses various date formats to milliseconds timestamp
func (m *FieldMapper) parseToMilliseconds(value string) (int64, error) {
	// List of common date formats to try
	formats := []string{
		time.RFC3339,          // "2006-01-02T15:04:05Z07:00"
		time.RFC3339Nano,      // "2006-01-02T15:04:05.999999999Z07:00"
		"2006-01-02 15:04:05", // "2006-01-02 15:04:05"
		"2006-01-02T15:04:05", // "2006-01-02T15:04:05"
		"1504569600000000",    // FreeSWITCH microseconds format
		"1504569600000",       // Unix milliseconds
		"1504569600",          // Unix seconds
	}

	// Try to parse with each format
	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			return t.UnixMilli(), nil
		}
	}

	// Try to parse as unix timestamp (seconds or milliseconds)
	if timestamp, err := strconv.ParseInt(value, 10, 64); err == nil {
		// If it looks like FreeSWITCH microseconds (16 digits), convert to milliseconds
		if len(value) == 16 {
			return timestamp / 1000, nil
		}
		// If it looks like milliseconds (13 digits), return as is
		if len(value) == 13 {
			return timestamp, nil
		}
		// If it looks like seconds (10 digits), convert to milliseconds
		if len(value) == 10 {
			return timestamp * 1000, nil
		}
	}

	return 0, fmt.Errorf("unable to parse date: %s", value)
}

// GetMappingCount returns the number of configured mappings
func (m *FieldMapper) GetMappingCount() int {
	totalMappings := len(m.globalMappings)
	for _, eventMapping := range m.eventMappings {
		totalMappings += len(eventMapping.Mappings)
	}
	return totalMappings
}

// getApplicableFilters returns field filters that apply to the given event
func (m *FieldMapper) getApplicableFilters(event *types.Event) *config.FieldFilter {
	// Check for event-specific filters first
	for _, eventMapping := range m.eventMappings {
		if m.matchesEventTypes(event, eventMapping.EventTypes) && eventMapping.FieldFilters != nil {
			return eventMapping.FieldFilters
		}
	}

	// Fall back to global filters
	return m.globalFilters
}

// filterHeaders filters raw event headers based on include/exclude patterns
func (m *FieldMapper) filterHeaders(headers map[string]string, filters *config.FieldFilter) map[string]string {
	if filters == nil {
		return headers
	}

	result := make(map[string]string)

	for headerName, headerValue := range headers {
		// Check if header should be included
		shouldInclude := true

		// Apply include patterns (if any specified, only include matching ones)
		if len(filters.IncludeHeaders) > 0 {
			shouldInclude = false
			for _, pattern := range filters.IncludeHeaders {
				if m.matchesPattern(headerName, pattern) {
					shouldInclude = true
					break
				}
			}
		}

		// Apply exclude patterns (exclude if matches)
		if shouldInclude && len(filters.ExcludeHeaders) > 0 {
			for _, pattern := range filters.ExcludeHeaders {
				if m.matchesPattern(headerName, pattern) {
					shouldInclude = false
					break
				}
			}
		}

		if shouldInclude {
			result[headerName] = headerValue
		}
	}

	return result
}

// shouldIncludeBody determines if event body should be included based on filters
func (m *FieldMapper) shouldIncludeBody(filters *config.FieldFilter) bool {
	if filters == nil {
		return true // Include by default if no filters
	}
	return filters.IncludeBody
}

// filterFields filters the final mapped fields based on include/exclude patterns
func (m *FieldMapper) filterFields(fields map[string]interface{}, filters *config.FieldFilter) map[string]interface{} {
	if filters == nil {
		return fields
	}

	result := make(map[string]interface{})

	for fieldName, fieldValue := range fields {
		// timestamp is always included as a required field for events
		if fieldName == "timestamp" {
			result[fieldName] = fieldValue
			continue
		}

		// Check if field should be included
		shouldInclude := true

		// Apply include patterns (if any specified, only include matching ones)
		if len(filters.IncludeFields) > 0 {
			shouldInclude = false
			for _, pattern := range filters.IncludeFields {
				if m.matchesPattern(fieldName, pattern) {
					shouldInclude = true
					break
				}
			}
		}

		// Apply exclude patterns (exclude if matches) - applies to all other fields
		if shouldInclude && len(filters.ExcludeFields) > 0 {
			for _, pattern := range filters.ExcludeFields {
				if m.matchesPattern(fieldName, pattern) {
					shouldInclude = false
					break
				}
			}
		}

		if shouldInclude {
			result[fieldName] = fieldValue
		}
	}

	return result
}

// matchesPattern checks if a name matches a pattern (supports wildcards)
func (m *FieldMapper) matchesPattern(name, pattern string) bool {
	// Exact match
	if name == pattern {
		return true
	}

	// Wildcard match
	if pattern == "*" {
		return true
	}

	// Prefix wildcard (e.g., "Event-*")
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(name, prefix)
	}

	// Suffix wildcard (e.g., "*-ID")
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(name, suffix)
	}

	// Contains pattern (e.g., "*caller*" -> contains "caller")
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") {
		substring := strings.Trim(pattern, "*")
		return strings.Contains(strings.ToLower(name), strings.ToLower(substring))
	}

	return false
}

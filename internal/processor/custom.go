package processor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"fsevents/internal/config"
	"fsevents/pkg/types"
)

// CustomProcessor interface for different processor types
type CustomProcessor interface {
	Process(event *types.Event, data map[string]interface{}) (map[string]interface{}, error)
	GetName() string
	GetEventTypes() []string
}

// ProcessorManager manages custom processors
type ProcessorManager struct {
	processors []CustomProcessor
	logger     *zap.Logger
}

// NewProcessorManager creates a new processor manager
func NewProcessorManager(configs []config.ProcessorConfig, logger *zap.Logger) (*ProcessorManager, error) {
	pm := &ProcessorManager{
		processors: make([]CustomProcessor, 0),
		logger:     logger.Named("processors"),
	}

	// Initialize processors from config
	for _, cfg := range configs {
		processor, err := pm.createProcessor(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to create processor %s: %w", cfg.Name, err)
		}
		pm.processors = append(pm.processors, processor)
	}

	pm.logger.Info("Initialized custom processors", zap.Int("count", len(pm.processors)))
	return pm, nil
}

// ProcessEvent runs the event through applicable custom processors
func (pm *ProcessorManager) ProcessEvent(event *types.Event, data map[string]interface{}) (map[string]interface{}, error) {
	result := data

	for _, processor := range pm.processors {
		// Check if processor should handle this event type
		if !pm.shouldProcessEvent(processor, event) {
			continue
		}

		pm.logger.Debug("Running custom processor",
			zap.String("processor", processor.GetName()),
			zap.String("event_name", event.Name),
		)

		var err error
		result, err = processor.Process(event, result)
		if err != nil {
			pm.logger.Error("Processor failed",
				zap.String("processor", processor.GetName()),
				zap.Error(err),
			)
			// Continue with other processors even if one fails
			continue
		}
	}

	return result, nil
}

// shouldProcessEvent checks if processor should handle this event
func (pm *ProcessorManager) shouldProcessEvent(processor CustomProcessor, event *types.Event) bool {
	eventTypes := processor.GetEventTypes()

	// If no event types specified, process all events
	if len(eventTypes) == 0 {
		return true
	}

	// Build event name with subclass for CUSTOM events
	eventNameWithSubclass := event.Name
	if event.Name == "CUSTOM" && event.Subclass != "" {
		eventNameWithSubclass = event.Name + " " + event.Subclass
	}

	// Check for exact match or wildcard
	for _, eventType := range eventTypes {
		if eventType == "*" {
			return true
		}

		// Exact match with event name
		if eventType == event.Name {
			return true
		}

		// Exact match with event name + subclass
		if eventType == eventNameWithSubclass {
			return true
		}

		// Support prefix matching with wildcard
		if strings.HasSuffix(eventType, "*") {
			prefix := strings.TrimSuffix(eventType, "*")
			if strings.HasPrefix(event.Name, prefix) {
				return true
			}
			// Also check against event name with subclass
			if strings.HasPrefix(eventNameWithSubclass, prefix) {
				return true
			}
		}
	}

	return false
}

// createProcessor creates a processor based on config
func (pm *ProcessorManager) createProcessor(cfg config.ProcessorConfig) (CustomProcessor, error) {
	switch cfg.Type {
	case "builtin":
		return NewBuiltinProcessor(cfg, pm.logger)
	case "javascript", "js":
		return NewJavaScriptProcessor(cfg, pm.logger)
	case "lua":
		return NewLuaProcessor(cfg, pm.logger)
	default:
		return nil, fmt.Errorf("unsupported processor type: %s", cfg.Type)
	}
}

// GetProcessorCount returns the number of active processors
func (pm *ProcessorManager) GetProcessorCount() int {
	return len(pm.processors)
}

// BuiltinProcessor implements common built-in processing logic
type BuiltinProcessor struct {
	name       string
	eventTypes []string
	config     map[string]interface{}
	logger     *zap.Logger
}

// NewBuiltinProcessor creates a new builtin processor
func NewBuiltinProcessor(cfg config.ProcessorConfig, logger *zap.Logger) (*BuiltinProcessor, error) {
	return &BuiltinProcessor{
		name:       cfg.Name,
		eventTypes: cfg.EventTypes,
		config:     cfg.Config,
		logger:     logger.Named("builtin"),
	}, nil
}

// Process implements CustomProcessor interface
func (bp *BuiltinProcessor) Process(event *types.Event, data map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	for k, v := range data {
		result[k] = v
	}

	// Apply built-in processing rules from config
	if rules, ok := bp.config["rules"].([]interface{}); ok {
		for _, rule := range rules {
			if ruleMap, ok := rule.(map[string]interface{}); ok {
				if err := bp.applyRule(event, result, ruleMap); err != nil {
					bp.logger.Warn("Failed to apply rule", zap.Error(err))
				}
			}
		}
	}

	return result, nil
}

// applyRule applies a single processing rule
func (bp *BuiltinProcessor) applyRule(event *types.Event, data map[string]interface{}, rule map[string]interface{}) error {
	action, ok := rule["action"].(string)
	if !ok {
		return fmt.Errorf("rule missing action")
	}

	switch action {
	case "add_field":
		return bp.addField(event, data, rule)
	case "remove_field":
		return bp.removeField(data, rule)
	case "rename_field":
		return bp.renameField(data, rule)
	case "transform_field":
		return bp.transformField(event, data, rule)
	case "extract_field":
		return bp.extractField(event, data, rule)
	case "conditional":
		return bp.applyConditional(event, data, rule)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

// addField adds a field to the data
func (bp *BuiltinProcessor) addField(event *types.Event, data map[string]interface{}, rule map[string]interface{}) error {
	field, ok := rule["field"].(string)
	if !ok {
		return fmt.Errorf("add_field rule missing field")
	}

	var value interface{}
	if valueStr, ok := rule["value"].(string); ok {
		// Support template-like substitution
		value = bp.substituteValue(event, valueStr)
	} else {
		value = rule["value"]
	}

	data[field] = value
	return nil
}

// removeField removes a field from the data
func (bp *BuiltinProcessor) removeField(data map[string]interface{}, rule map[string]interface{}) error {
	field, ok := rule["field"].(string)
	if !ok {
		return fmt.Errorf("remove_field rule missing field")
	}

	delete(data, field)
	return nil
}

// renameField renames a field in the data
func (bp *BuiltinProcessor) renameField(data map[string]interface{}, rule map[string]interface{}) error {
	from, ok := rule["from"].(string)
	if !ok {
		return fmt.Errorf("rename_field rule missing from")
	}

	to, ok := rule["to"].(string)
	if !ok {
		return fmt.Errorf("rename_field rule missing to")
	}

	if value, exists := data[from]; exists {
		data[to] = value
		delete(data, from)
	}

	return nil
}

// transformField transforms an existing field in the data
func (bp *BuiltinProcessor) transformField(event *types.Event, data map[string]interface{}, rule map[string]interface{}) error {
	field, ok := rule["field"].(string)
	if !ok {
		return fmt.Errorf("transform_field rule missing field")
	}

	transform, ok := rule["transform"].(string)
	if !ok {
		return fmt.Errorf("transform_field rule missing transform")
	}

	// Get current value from data or event
	var currentValue string
	if value, exists := data[field]; exists {
		currentValue = fmt.Sprintf("%v", value)
	} else {
		// Try to get from event headers
		switch field {
		case "Event-Name":
			currentValue = event.Name
		case "Event-Subclass":
			currentValue = event.Subclass
		case "Event-Body":
			currentValue = event.Body
		default:
			currentValue = event.GetHeader(field)
		}
	}

	// Apply transformation
	transformedValue := bp.applyFieldTransform(currentValue, transform)

	// Store transformed value back
	if transformedValue != nil {
		data[field] = transformedValue
	}

	return nil
}

// extractField extracts a value from event headers/body and creates a new field
func (bp *BuiltinProcessor) extractField(event *types.Event, data map[string]interface{}, rule map[string]interface{}) error {
	from, ok := rule["from"].(string)
	if !ok {
		return fmt.Errorf("extract_field rule missing from")
	}

	to, ok := rule["to"].(string)
	if !ok {
		return fmt.Errorf("extract_field rule missing to")
	}

	// Get value from event
	var value string
	switch from {
	case "Event-Name":
		value = event.Name
	case "Event-Subclass":
		value = event.Subclass
	case "Event-Body":
		value = event.Body
	default:
		value = event.GetHeader(from)
	}

	// Apply transformation if specified
	if transform, ok := rule["transform"].(string); ok && transform != "" {
		transformedValue := bp.applyFieldTransform(value, transform)
		data[to] = transformedValue
	} else {
		// Use default value if original is empty and default is provided
		if value == "" {
			if defaultValue, ok := rule["default"].(string); ok {
				value = defaultValue
			}
		}
		data[to] = value
	}

	return nil
}

// applyFieldTransform applies various transformations to a field value
func (bp *BuiltinProcessor) applyFieldTransform(value, transform string) interface{} {
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
	case ":int":
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
		bp.logger.Warn("Failed to convert to int", zap.String("value", value))
		return value
	case ":float":
		if floatVal, err := strconv.ParseFloat(value, 64); err == nil {
			return floatVal
		}
		bp.logger.Warn("Failed to convert to float", zap.String("value", value))
		return value
	case ":bool":
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
		bp.logger.Warn("Failed to convert to bool", zap.String("value", value))
		return value
	case "url_encode":
		return url.QueryEscape(value)
	case "url_decode":
		if decoded, err := url.QueryUnescape(value); err == nil {
			return decoded
		}
		bp.logger.Warn("Failed to URL decode", zap.String("value", value))
		return value
	case "base64_encode":
		return base64.StdEncoding.EncodeToString([]byte(value))
	case "base64_decode":
		if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
			return string(decoded)
		}
		bp.logger.Warn("Failed to base64 decode", zap.String("value", value))
		return value
	case "json_escape":
		escaped, _ := json.Marshal(value)
		return string(escaped[1 : len(escaped)-1]) // Remove quotes
	case "length":
		return len(value)
	case "first_word":
		words := strings.Fields(value)
		if len(words) > 0 {
			return words[0]
		}
		return ""
	case "last_word":
		words := strings.Fields(value)
		if len(words) > 0 {
			return words[len(words)-1]
		}
		return ""
	default:
		// Support regex replacements: "s/pattern/replacement/"
		if strings.HasPrefix(transform, "s/") && strings.Count(transform, "/") >= 2 {
			parts := strings.Split(transform[2:], "/")
			if len(parts) >= 2 {
				pattern := parts[0]
				replacement := parts[1]
				if regex, err := regexp.Compile(pattern); err == nil {
					return regex.ReplaceAllString(value, replacement)
				}
				bp.logger.Warn("Invalid regex pattern", zap.String("pattern", pattern))
			}
		}
		bp.logger.Warn("Unknown transform type", zap.String("transform", transform))
		return value
	}
}

// applyConditional applies conditional logic
func (bp *BuiltinProcessor) applyConditional(event *types.Event, data map[string]interface{}, rule map[string]interface{}) error {
	condition, ok := rule["condition"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("conditional rule missing condition")
	}

	if bp.evaluateCondition(event, data, condition) {
		if thenRules, ok := rule["then"].([]interface{}); ok {
			for _, thenRule := range thenRules {
				if thenRuleMap, ok := thenRule.(map[string]interface{}); ok {
					bp.applyRule(event, data, thenRuleMap)
				}
			}
		}
	} else {
		if elseRules, ok := rule["else"].([]interface{}); ok {
			for _, elseRule := range elseRules {
				if elseRuleMap, ok := elseRule.(map[string]interface{}); ok {
					bp.applyRule(event, data, elseRuleMap)
				}
			}
		}
	}

	return nil
}

// evaluateCondition evaluates a condition
func (bp *BuiltinProcessor) evaluateCondition(event *types.Event, data map[string]interface{}, condition map[string]interface{}) bool {
	field, ok := condition["field"].(string)
	if !ok {
		return false
	}

	operator, ok := condition["operator"].(string)
	if !ok {
		operator = "equals"
	}

	expectedValue, ok := condition["value"].(string)
	if !ok {
		return false
	}

	var actualValue string

	// Check special event fields first
	switch field {
	case "Event-Name":
		actualValue = event.Name
	case "Event-Subclass":
		actualValue = event.Subclass
	case "Event-Body":
		actualValue = event.Body
	default:
		// Check in processed data first, then in event headers
		if value, exists := data[field]; exists {
			actualValue = fmt.Sprintf("%v", value)
		} else {
			actualValue = event.GetHeader(field)
		}
	}

	switch operator {
	case "equals":
		return actualValue == expectedValue
	case "not_equals":
		return actualValue != expectedValue
	case "contains":
		return strings.Contains(actualValue, expectedValue)
	case "starts_with":
		return strings.HasPrefix(actualValue, expectedValue)
	case "ends_with":
		return strings.HasSuffix(actualValue, expectedValue)
	default:
		return false
	}
}

// substituteValue performs simple variable substitution
func (bp *BuiltinProcessor) substituteValue(event *types.Event, template string) string {
	result := template

	// Replace ${header:name} with event header value
	for strings.Contains(result, "${header:") {
		start := strings.Index(result, "${header:")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		headerName := result[start+9 : end] // 9 = len("${header:")
		headerValue := event.GetHeader(headerName)
		result = result[:start] + headerValue + result[end+1:]
	}

	return result
}

// GetName returns the processor name
func (bp *BuiltinProcessor) GetName() string {
	return bp.name
}

// GetEventTypes returns the event types this processor handles
func (bp *BuiltinProcessor) GetEventTypes() []string {
	return bp.eventTypes
}

// Placeholder processors for future implementation
type JavaScriptProcessor struct {
	name       string
	eventTypes []string
	script     string
	logger     *zap.Logger
}

func NewJavaScriptProcessor(cfg config.ProcessorConfig, logger *zap.Logger) (*JavaScriptProcessor, error) {
	script, _ := cfg.Config["script"].(string)
	return &JavaScriptProcessor{
		name:       cfg.Name,
		eventTypes: cfg.EventTypes,
		script:     script,
		logger:     logger.Named("javascript"),
	}, nil
}

func (jsp *JavaScriptProcessor) Process(event *types.Event, data map[string]interface{}) (map[string]interface{}, error) {
	// TODO: Implement JavaScript execution engine (e.g., using goja)
	jsp.logger.Warn("JavaScript processor not yet implemented")
	return data, nil
}

func (jsp *JavaScriptProcessor) GetName() string {
	return jsp.name
}

func (jsp *JavaScriptProcessor) GetEventTypes() []string {
	return jsp.eventTypes
}

type LuaProcessor struct {
	name       string
	eventTypes []string
	script     string
	logger     *zap.Logger
}

func NewLuaProcessor(cfg config.ProcessorConfig, logger *zap.Logger) (*LuaProcessor, error) {
	script, _ := cfg.Config["script"].(string)
	return &LuaProcessor{
		name:       cfg.Name,
		eventTypes: cfg.EventTypes,
		script:     script,
		logger:     logger.Named("lua"),
	}, nil
}

func (lp *LuaProcessor) Process(event *types.Event, data map[string]interface{}) (map[string]interface{}, error) {
	// TODO: Implement Lua execution engine (e.g., using gopher-lua)
	lp.logger.Warn("Lua processor not yet implemented")
	return data, nil
}

func (lp *LuaProcessor) GetName() string {
	return lp.name
}

func (lp *LuaProcessor) GetEventTypes() []string {
	return lp.eventTypes
}

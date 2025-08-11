package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config represents the application configuration
type Config struct {
	ESL     ESLConfig     `mapstructure:"esl"`
	Events  EventsConfig  `mapstructure:"events"`
	HTTP    HTTPConfig    `mapstructure:"http"`
	Logging LoggingConfig `mapstructure:"logging"`
	Metrics MetricsConfig `mapstructure:"metrics"`
}

// ESLConfig represents FreeSWITCH ESL connection configuration
type ESLConfig struct {
	Host                 string          `mapstructure:"host"`
	Port                 int             `mapstructure:"port"`
	Password             string          `mapstructure:"password"`
	Timeout              time.Duration   `mapstructure:"timeout"`
	ReconnectInterval    time.Duration   `mapstructure:"reconnect_interval"`
	MaxReconnectAttempts int             `mapstructure:"max_reconnect_attempts"`
	Keepalive            KeepaliveConfig `mapstructure:"keepalive" yaml:"keepalive"`
}

// KeepaliveConfig represents heartbeat/keepalive settings for ESL connection
type KeepaliveConfig struct {
	Enabled          bool          `mapstructure:"enabled" yaml:"enabled"`
	Interval         time.Duration `mapstructure:"interval" yaml:"interval"`
	Timeout          time.Duration `mapstructure:"timeout" yaml:"timeout"`
	FailureThreshold int           `mapstructure:"failure_threshold" yaml:"failure_threshold"`
}

// FieldMapping defines how to map event headers to output fields
type FieldMapping struct {
	From         string   `mapstructure:"from" yaml:"from"`                   // Source header name
	To           string   `mapstructure:"to" yaml:"to"`                       // Target field name
	DefaultValue string   `mapstructure:"default_value" yaml:"default_value"` // Default value if header missing
	Transforms   []string `mapstructure:"transforms" yaml:"transforms"`       // Multiple transformations in order
	EventTypes   []string `mapstructure:"event_types" yaml:"event_types"`     // Which events this mapping applies to (empty = all events)
}

// FieldFilter defines what fields to include/exclude from events
type FieldFilter struct {
	IncludeHeaders []string `mapstructure:"include_headers" yaml:"include_headers"` // Headers to include from raw event (supports wildcards)
	ExcludeHeaders []string `mapstructure:"exclude_headers" yaml:"exclude_headers"` // Headers to exclude from raw event (supports wildcards)
	IncludeFields  []string `mapstructure:"include_fields" yaml:"include_fields"`   // Mapped fields to include in payload (supports wildcards)
	ExcludeFields  []string `mapstructure:"exclude_fields" yaml:"exclude_fields"`   // Mapped fields to exclude from payload (supports wildcards)
	IncludeBody    bool     `mapstructure:"include_body" yaml:"include_body"`       // Whether to include event body
}

// EventFieldMappings defines field mappings for specific event types
type EventFieldMappings struct {
	EventTypes   []string       `mapstructure:"event_types" yaml:"event_types"`     // Which events this applies to
	Mappings     []FieldMapping `mapstructure:"mappings" yaml:"mappings"`           // Field mappings for these events
	FieldFilters *FieldFilter   `mapstructure:"field_filters" yaml:"field_filters"` // Field filtering rules for these events
}

// ProcessorConfig defines custom event processor configuration
type ProcessorConfig struct {
	Name       string                 `mapstructure:"name" yaml:"name"`
	Type       string                 `mapstructure:"type" yaml:"type"` // builtin, lua, javascript, etc.
	Config     map[string]interface{} `mapstructure:"config" yaml:"config"`
	EventTypes []string               `mapstructure:"event_types" yaml:"event_types"` // Which events to process
}

// PayloadTemplate defines how to structure the output payload
type PayloadTemplate struct {
	Format   string            `mapstructure:"format" yaml:"format"`     // json, xml, form, etc.
	Template string            `mapstructure:"template" yaml:"template"` // Go template string
	Headers  map[string]string `mapstructure:"headers" yaml:"headers"`   // Additional headers to set
}

// EventsConfig defines event subscription and processing configuration
type EventsConfig struct {
	SubscribeEvents    []string             `mapstructure:"subscribe_events" yaml:"subscribe_events"`
	Filters            []FilterRule         `mapstructure:"filters" yaml:"filters"`
	FilterLogic        string               `mapstructure:"filter_logic" yaml:"filter_logic"`                 // "AND" or "OR" - default is "AND"
	FieldMappings      []FieldMapping       `mapstructure:"field_mappings" yaml:"field_mappings"`             // Global field mappings
	EventFieldMappings []EventFieldMappings `mapstructure:"event_field_mappings" yaml:"event_field_mappings"` // Event-specific field mappings
	FieldFilters       *FieldFilter         `mapstructure:"field_filters" yaml:"field_filters"`               // Global field filtering rules
	Processors         []ProcessorConfig    `mapstructure:"processors" yaml:"processors"`
	PayloadTemplate    *PayloadTemplate     `mapstructure:"payload_template" yaml:"payload_template"`
}

// FilterRule represents a single event filter rule (renamed from EventFilter)
type FilterRule struct {
	Field    string `mapstructure:"field" yaml:"field"`
	Operator string `mapstructure:"operator" yaml:"operator"`
	Value    string `mapstructure:"value" yaml:"value"`
}

// HTTPConfig represents HTTP client configuration
type HTTPConfig struct {
	Destinations []HTTPDestination `mapstructure:"destinations"`
}

// HTTPDestination represents a single HTTP destination
type HTTPDestination struct {
	Name            string            `mapstructure:"name" yaml:"name"`
	URL             string            `mapstructure:"url" yaml:"url"`
	Method          string            `mapstructure:"method" yaml:"method"`
	Headers         map[string]string `mapstructure:"headers" yaml:"headers"`
	Timeout         time.Duration     `mapstructure:"timeout" yaml:"timeout"`
	Retry           RetryConfig       `mapstructure:"retry" yaml:"retry"`
	EventFilters    []string          `mapstructure:"event_filters" yaml:"event_filters"`       // Simple event name filters (legacy support)
	Filters         []FilterRule      `mapstructure:"filters" yaml:"filters"`                   // Advanced filter rules with operators
	FilterLogic     string            `mapstructure:"filter_logic" yaml:"filter_logic"`         // "AND" or "OR" logic for filters (default: "AND")
	PayloadTemplate *PayloadTemplate  `mapstructure:"payload_template" yaml:"payload_template"` // Optional template for this destination
}

// RetryConfig represents retry configuration
type RetryConfig struct {
	MaxAttempts  int           `mapstructure:"max_attempts"`
	Backoff      string        `mapstructure:"backoff"`
	InitialDelay time.Duration `mapstructure:"initial_delay"`
	MaxDelay     time.Duration `mapstructure:"max_delay"`
}

// LoggingConfig represents logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// MetricsConfig represents metrics configuration
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// LoadOptions represents configuration loading options
type LoadOptions struct {
	ConfigFile      string
	UseEnvironment  bool
	ValidateConfig  bool
	LogConfigSource bool
}

// Load loads configuration from various sources with options
func Load(configFile string) (*Config, error) {
	return LoadWithOptions(LoadOptions{
		ConfigFile:      configFile,
		UseEnvironment:  true,
		ValidateConfig:  true,
		LogConfigSource: false,
	})
}

// LoadWithOptions loads configuration with detailed options
func LoadWithOptions(opts LoadOptions) (*Config, error) {
	v := viper.New()

	// Set default values
	setDefaults(v)

	// Configure environment variables
	if opts.UseEnvironment {
		v.SetEnvPrefix("FSEVENTS")
		v.AutomaticEnv()

		// Replace dots and dashes with underscores for env vars
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	}

	// Configuration file
	configLoaded := false
	if opts.ConfigFile != "" {
		v.SetConfigFile(opts.ConfigFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("error reading config file %s: %w", opts.ConfigFile, err)
		}
		configLoaded = true
		if opts.LogConfigSource {
			fmt.Printf("Configuration loaded from file: %s\n", opts.ConfigFile)
		}
	} else {
		// Try to find config file in common locations
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath("./config")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/fsevents")

		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); ok {
				// Config file not found is OK, we'll use defaults and env vars
				if opts.LogConfigSource {
					fmt.Println("No configuration file found, using defaults and environment variables")
				}
			} else {
				return nil, fmt.Errorf("error reading config file: %w", err)
			}
		} else {
			configLoaded = true
			if opts.LogConfigSource {
				fmt.Printf("Configuration loaded from file: %s\n", v.ConfigFileUsed())
			}
		}
	}

	// Log configuration sources used
	if opts.LogConfigSource {
		if configLoaded {
			fmt.Printf("Configuration file used: %s\n", v.ConfigFileUsed())
		}
		if opts.UseEnvironment {
			fmt.Println("Environment variables enabled with prefix: FSEVENTS_")
		}
	}

	// Unmarshal configuration
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate configuration if requested
	if opts.ValidateConfig {
		if err := config.Validate(); err != nil {
			return nil, fmt.Errorf("configuration validation failed: %w", err)
		}
	}

	return &config, nil
}

// LoadFromString loads configuration from YAML string (useful for testing)
func LoadFromString(yamlContent string) (*Config, error) {
	v := viper.New()
	v.SetConfigType("yaml")

	if err := v.ReadConfig(strings.NewReader(yamlContent)); err != nil {
		return nil, fmt.Errorf("error reading config from string: %w", err)
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &config, nil
}

// SaveToFile saves the current configuration to a file
func (c *Config) SaveToFile(filename string) error {
	v := viper.New()

	// Convert config back to viper
	if err := v.MergeConfigMap(configToMap(c)); err != nil {
		return fmt.Errorf("error converting config to map: %w", err)
	}

	if err := v.WriteConfigAs(filename); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}

// GetConfigSources returns information about configuration sources
func GetConfigSources() []string {
	sources := []string{
		"Default values",
		"Configuration file (./configs/config.yaml, ./config.yaml, etc.)",
		"Environment variables (FSEVENTS_*)",
		"Command line flags (if provided)",
	}
	return sources
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	// ESL defaults
	v.SetDefault("esl.host", "localhost")
	v.SetDefault("esl.port", 8021)
	v.SetDefault("esl.password", "ClueCon")
	v.SetDefault("esl.timeout", "10s")
	v.SetDefault("esl.reconnect_interval", "5s")
	v.SetDefault("esl.max_reconnect_attempts", 10)
	// ESL keepalive defaults
	v.SetDefault("esl.keepalive.enabled", true)
	v.SetDefault("esl.keepalive.interval", "15s")
	v.SetDefault("esl.keepalive.timeout", "3s")
	v.SetDefault("esl.keepalive.failure_threshold", 2)

	// Events defaults
	v.SetDefault("events.subscribe_events", []string{"HEARTBEAT"})
	v.SetDefault("events.filter_logic", "AND")

	// HTTP defaults
	v.SetDefault("http.destinations", []map[string]interface{}{})

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.port", 9090)
	v.SetDefault("metrics.path", "/metrics")
}

// configToMap converts Config struct to map for viper
func configToMap(c *Config) map[string]interface{} {
	return map[string]interface{}{
		"esl": map[string]interface{}{
			"host":                   c.ESL.Host,
			"port":                   c.ESL.Port,
			"password":               c.ESL.Password,
			"timeout":                c.ESL.Timeout.String(),
			"reconnect_interval":     c.ESL.ReconnectInterval.String(),
			"max_reconnect_attempts": c.ESL.MaxReconnectAttempts,
			"keepalive": map[string]interface{}{
				"enabled":           c.ESL.Keepalive.Enabled,
				"interval":          c.ESL.Keepalive.Interval.String(),
				"timeout":           c.ESL.Keepalive.Timeout.String(),
				"failure_threshold": c.ESL.Keepalive.FailureThreshold,
			},
		},
		"events": map[string]interface{}{
			"subscribe_events": c.Events.SubscribeEvents,
			"filters":          c.Events.Filters,
			"filter_logic":     c.Events.FilterLogic,
			"field_mappings":   c.Events.FieldMappings,
			"processors":       c.Events.Processors,
			"payload_template": c.Events.PayloadTemplate,
		},
		"http": map[string]interface{}{
			"destinations": c.HTTP.Destinations,
		},
		"logging": map[string]interface{}{
			"level":  c.Logging.Level,
			"format": c.Logging.Format,
			"output": c.Logging.Output,
		},
		"metrics": map[string]interface{}{
			"enabled": c.Metrics.Enabled,
			"port":    c.Metrics.Port,
			"path":    c.Metrics.Path,
		},
	}
}

// CheckEnvironmentOverrides checks which configuration values are overridden by environment variables
func CheckEnvironmentOverrides() map[string]string {
	overrides := make(map[string]string)

	envVars := []string{
		"FSEVENTS_ESL_HOST",
		"FSEVENTS_ESL_PORT",
		"FSEVENTS_ESL_PASSWORD",
		"FSEVENTS_ESL_TIMEOUT",
		"FSEVENTS_ESL_RECONNECT_INTERVAL",
		"FSEVENTS_ESL_MAX_RECONNECT_ATTEMPTS",
		"FSEVENTS_LOGGING_LEVEL",
		"FSEVENTS_LOGGING_FORMAT",
		"FSEVENTS_LOGGING_OUTPUT",
		"FSEVENTS_METRICS_ENABLED",
		"FSEVENTS_METRICS_PORT",
		"FSEVENTS_METRICS_PATH",
	}

	for _, envVar := range envVars {
		if value := os.Getenv(envVar); value != "" {
			overrides[envVar] = value
		}
	}

	return overrides
}

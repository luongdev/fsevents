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
	Host                 string        `mapstructure:"host"`
	Port                 int           `mapstructure:"port"`
	Password             string        `mapstructure:"password"`
	Timeout              time.Duration `mapstructure:"timeout"`
	ReconnectInterval    time.Duration `mapstructure:"reconnect_interval"`
	MaxReconnectAttempts int           `mapstructure:"max_reconnect_attempts"`
}

// EventsConfig represents event filtering configuration
type EventsConfig struct {
	SubscribeEvents []string      `mapstructure:"subscribe_events"`
	Filters         []EventFilter `mapstructure:"filters"`
}

// EventFilter represents a single event filter rule
type EventFilter struct {
	Field    string `mapstructure:"field"`
	Operator string `mapstructure:"operator"`
	Value    string `mapstructure:"value"`
}

// HTTPConfig represents HTTP client configuration
type HTTPConfig struct {
	Destinations []HTTPDestination `mapstructure:"destinations"`
}

// HTTPDestination represents a single HTTP destination
type HTTPDestination struct {
	Name    string            `mapstructure:"name"`
	URL     string            `mapstructure:"url"`
	Method  string            `mapstructure:"method"`
	Headers map[string]string `mapstructure:"headers"`
	Timeout time.Duration     `mapstructure:"timeout"`
	Retry   RetryConfig       `mapstructure:"retry"`
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

	// Events defaults
	v.SetDefault("events.subscribe_events", []string{"HEARTBEAT"})

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
		},
		"events": map[string]interface{}{
			"subscribe_events": c.Events.SubscribeEvents,
			"filters":          c.Events.Filters,
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

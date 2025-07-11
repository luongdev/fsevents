package config

import (
	"fmt"
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

// Load loads configuration from various sources
func Load(configFile string) (*Config, error) {
	v := viper.New()

	// Set default values
	setDefaults(v)

	// Configuration file
	if configFile != "" {
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath("./configs")
		v.AddConfigPath(".")
	}

	// Environment variables
	v.SetEnvPrefix("FSEVENTS")
	v.AutomaticEnv()

	// Read configuration file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found is OK, we'll use defaults and env vars
	}

	// Unmarshal configuration
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &config, nil
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
	v.SetDefault("events.subscribe_events", []string{"CHANNEL_CREATE", "CHANNEL_DESTROY"})

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// Metrics defaults
	v.SetDefault("metrics.enabled", true)
	v.SetDefault("metrics.port", 9090)
	v.SetDefault("metrics.path", "/metrics")
}

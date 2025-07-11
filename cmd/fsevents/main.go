package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"fsevents/internal/app"
	"fsevents/internal/config"
	"fsevents/internal/logger"
)

var (
	cfgFile   string
	logLevel  string
	logFormat string
	version   = "0.1.0"
	buildTime = "unknown"
	gitCommit = "unknown"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "fsevents",
	Short: "FreeSWITCH ESL Sidecar App",
	Long: `A Go-based sidecar application that connects to FreeSWITCH Event Socket Library (ESL) 
to receive events and forward them to HTTP endpoints.

Features:
- Connect to FreeSWITCH ESL
- Receive and process events
- Forward events to HTTP destinations
- Configurable event filtering
- Retry mechanisms
- Monitoring and metrics`,
	RunE: runApp,
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  "Print version information including build time and git commit",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("fsevents version %s\n", version)
		fmt.Printf("Build time: %s\n", buildTime)
		fmt.Printf("Git commit: %s\n", gitCommit)
	},
}

// configCmd represents the config command
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration related commands",
	Long:  "Configuration related commands for managing fsevents configuration",
}

// validateConfigCmd represents the config validate command
var validateConfigCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration file",
	Long:  "Validate the configuration file syntax and values",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadWithOptions(config.LoadOptions{
			ConfigFile:      cfgFile,
			UseEnvironment:  false, // Don't use env vars for validation
			ValidateConfig:  true,
			LogConfigSource: true,
		})
		if err != nil {
			return fmt.Errorf("configuration validation failed: %w", err)
		}

		fmt.Printf("\n✅ Configuration is valid!\n\n")
		fmt.Printf("📋 Configuration Summary:\n")
		fmt.Printf("  ESL Host: %s:%d\n", cfg.ESL.Host, cfg.ESL.Port)
		fmt.Printf("  ESL Timeout: %v\n", cfg.ESL.Timeout)
		fmt.Printf("  Subscribe Events: %v\n", cfg.Events.SubscribeEvents)
		fmt.Printf("  Event Filters: %d\n", len(cfg.Events.Filters))
		fmt.Printf("  HTTP Destinations: %d\n", len(cfg.HTTP.Destinations))
		fmt.Printf("  Log Level: %s\n", cfg.Logging.Level)
		fmt.Printf("  Log Format: %s\n", cfg.Logging.Format)
		fmt.Printf("  Metrics Enabled: %t\n", cfg.Metrics.Enabled)

		if len(cfg.HTTP.Destinations) > 0 {
			fmt.Printf("\n🌐 HTTP Destinations:\n")
			for i, dest := range cfg.HTTP.Destinations {
				fmt.Printf("  %d. %s -> %s %s\n", i+1, dest.Name, dest.Method, dest.URL)
			}
		}

		if len(cfg.Events.Filters) > 0 {
			fmt.Printf("\n🔍 Event Filters:\n")
			for i, filter := range cfg.Events.Filters {
				fmt.Printf("  %d. %s %s %s\n", i+1, filter.Field, filter.Operator, filter.Value)
			}
		}

		return nil
	},
}

// showConfigCmd shows current configuration from all sources
var showConfigCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Long:  "Show the current configuration as loaded from all sources (defaults, files, env vars)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadWithOptions(config.LoadOptions{
			ConfigFile:      cfgFile,
			UseEnvironment:  true,
			ValidateConfig:  false,
			LogConfigSource: true,
		})
		if err != nil {
			return fmt.Errorf("failed to load configuration: %w", err)
		}

		fmt.Printf("\n📋 Current Configuration:\n")
		fmt.Printf("\n[ESL]\n")
		fmt.Printf("  Host: %s\n", cfg.ESL.Host)
		fmt.Printf("  Port: %d\n", cfg.ESL.Port)
		fmt.Printf("  Password: %s\n", maskPassword(cfg.ESL.Password))
		fmt.Printf("  Timeout: %v\n", cfg.ESL.Timeout)
		fmt.Printf("  Reconnect Interval: %v\n", cfg.ESL.ReconnectInterval)
		fmt.Printf("  Max Reconnect Attempts: %d\n", cfg.ESL.MaxReconnectAttempts)

		fmt.Printf("\n[Events]\n")
		fmt.Printf("  Subscribe Events: %v\n", cfg.Events.SubscribeEvents)
		fmt.Printf("  Filters: %d configured\n", len(cfg.Events.Filters))

		fmt.Printf("\n[HTTP]\n")
		fmt.Printf("  Destinations: %d configured\n", len(cfg.HTTP.Destinations))

		fmt.Printf("\n[Logging]\n")
		fmt.Printf("  Level: %s\n", cfg.Logging.Level)
		fmt.Printf("  Format: %s\n", cfg.Logging.Format)
		fmt.Printf("  Output: %s\n", cfg.Logging.Output)

		fmt.Printf("\n[Metrics]\n")
		fmt.Printf("  Enabled: %t\n", cfg.Metrics.Enabled)
		if cfg.Metrics.Enabled {
			fmt.Printf("  Port: %d\n", cfg.Metrics.Port)
			fmt.Printf("  Path: %s\n", cfg.Metrics.Path)
		}

		// Show environment overrides
		overrides := config.CheckEnvironmentOverrides()
		if len(overrides) > 0 {
			fmt.Printf("\n🌍 Environment Variable Overrides:\n")
			for envVar, value := range overrides {
				if envVar == "FSEVENTS_ESL_PASSWORD" {
					value = maskPassword(value)
				}
				fmt.Printf("  %s = %s\n", envVar, value)
			}
		}

		fmt.Printf("\n📚 Configuration Sources (in priority order):\n")
		sources := config.GetConfigSources()
		for i, source := range sources {
			fmt.Printf("  %d. %s\n", i+1, source)
		}

		return nil
	},
}

// envCmd shows environment variable information
var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Show environment variable information",
	Long:  "Show all supported environment variables and their current values",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("🌍 Environment Variables for fsevents:\n\n")
		fmt.Printf("All environment variables use the prefix 'FSEVENTS_'\n")
		fmt.Printf("Dots in config keys are replaced with underscores\n\n")

		envVars := map[string]string{
			"FSEVENTS_ESL_HOST":                   "ESL server host",
			"FSEVENTS_ESL_PORT":                   "ESL server port",
			"FSEVENTS_ESL_PASSWORD":               "ESL server password",
			"FSEVENTS_ESL_TIMEOUT":                "ESL connection timeout",
			"FSEVENTS_ESL_RECONNECT_INTERVAL":     "ESL reconnect interval",
			"FSEVENTS_ESL_MAX_RECONNECT_ATTEMPTS": "ESL max reconnect attempts",
			"FSEVENTS_LOGGING_LEVEL":              "Log level (debug, info, warn, error)",
			"FSEVENTS_LOGGING_FORMAT":             "Log format (json, console)",
			"FSEVENTS_LOGGING_OUTPUT":             "Log output (stdout, stderr, file path)",
			"FSEVENTS_METRICS_ENABLED":            "Enable metrics (true, false)",
			"FSEVENTS_METRICS_PORT":               "Metrics server port",
			"FSEVENTS_METRICS_PATH":               "Metrics endpoint path",
		}

		overrides := config.CheckEnvironmentOverrides()

		for envVar, description := range envVars {
			value := os.Getenv(envVar)
			status := "not set"
			if value != "" {
				if envVar == "FSEVENTS_ESL_PASSWORD" {
					value = maskPassword(value)
				}
				status = fmt.Sprintf("set to: %s", value)
			}
			fmt.Printf("  %-35s %s (%s)\n", envVar, description, status)
		}

		if len(overrides) > 0 {
			fmt.Printf("\n✅ Currently active overrides: %d\n", len(overrides))
		} else {
			fmt.Printf("\n📝 No environment overrides currently active\n")
		}

		fmt.Printf("\nExample usage:\n")
		fmt.Printf("  export FSEVENTS_ESL_HOST=192.168.1.100\n")
		fmt.Printf("  export FSEVENTS_LOGGING_LEVEL=debug\n")
		fmt.Printf("  ./fsevents\n")
	},
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is ./configs/config.yaml)")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "", "log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "", "log format (json, console)")

	// Add subcommands
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(validateConfigCmd)
	configCmd.AddCommand(showConfigCmd)
	configCmd.AddCommand(envCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func runApp(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Override config with command line flags
	if logLevel != "" {
		cfg.Logging.Level = logLevel
	}
	if logFormat != "" {
		cfg.Logging.Format = logFormat
	}

	// Initialize logger
	if err := logger.Initialize(cfg.Logging); err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Log startup information
	logger.Info("FreeSWITCH ESL Sidecar App starting",
		zap.String("version", version),
		zap.String("build_time", buildTime),
		zap.String("git_commit", gitCommit),
		zap.String("config_file", cfgFile),
	)

	// Create and start application
	application := app.New(cfg)
	if err := application.Start(); err != nil {
		logger.Error("Application failed", zap.Error(err))
		return err
	}

	return nil
}

// maskPassword masks password for display
func maskPassword(password string) string {
	if len(password) <= 2 {
		return "***"
	}
	return password[:2] + "***"
}

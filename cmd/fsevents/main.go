package main

import (
	"fmt"
	"os"

	"go.uber.org/zap"

	"fsevents/internal/config"
	"fsevents/internal/logger"
)

func main() {
	fmt.Println("FreeSWITCH ESL Sidecar App")
	fmt.Println("Version: 0.1.0")

	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println("fsevents version 0.1.0")
		return
	}

	fmt.Println("Starting application...")

	// Load configuration
	cfg, err := config.Load("")
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Initialize(cfg.Logging); err != nil {
		fmt.Printf("Error initializing logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// Log application startup
	logger.Info("Application starting",
		zap.String("version", "0.1.0"),
		zap.String("component", "main"),
	)

	// Display loaded configuration
	logger.Info("Configuration loaded successfully",
		zap.String("esl_host", cfg.ESL.Host),
		zap.Int("esl_port", cfg.ESL.Port),
		zap.Duration("esl_timeout", cfg.ESL.Timeout),
		zap.Strings("subscribe_events", cfg.Events.SubscribeEvents),
		zap.String("log_level", cfg.Logging.Level),
		zap.Bool("metrics_enabled", cfg.Metrics.Enabled),
	)

	// Demonstrate different log levels
	logger.Debug("This is a debug message", zap.String("test", "debug"))
	logger.Info("This is an info message", zap.String("test", "info"))
	logger.Warn("This is a warning message", zap.String("test", "warn"))

	// Demonstrate structured logging with fields
	componentLogger := logger.With(zap.String("component", "main"))
	componentLogger.Info("Using component-specific logger",
		zap.String("action", "demonstration"),
		zap.Int("count", 42),
	)

	logger.Info("Application setup complete")
	fmt.Println("Application setup complete")
}

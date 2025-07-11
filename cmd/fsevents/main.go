package main

import (
	"fmt"
	"os"

	"fsevents/internal/config"
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

	// Display loaded configuration
	fmt.Printf("Configuration loaded successfully:\n")
	fmt.Printf("  ESL Host: %s:%d\n", cfg.ESL.Host, cfg.ESL.Port)
	fmt.Printf("  ESL Timeout: %v\n", cfg.ESL.Timeout)
	fmt.Printf("  Subscribe Events: %v\n", cfg.Events.SubscribeEvents)
	fmt.Printf("  Log Level: %s\n", cfg.Logging.Level)
	fmt.Printf("  Metrics Enabled: %t\n", cfg.Metrics.Enabled)

	fmt.Println("Application setup complete")
}

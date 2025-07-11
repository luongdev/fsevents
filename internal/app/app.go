package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"

	"fsevents/internal/config"
	"fsevents/internal/logger"
)

// App represents the main application
type App struct {
	config   *config.Config
	logger   *zap.Logger
	shutdown chan os.Signal
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
}

// New creates a new application instance
func New(cfg *config.Config) *App {
	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		config:   cfg,
		logger:   logger.GetLogger(),
		shutdown: make(chan os.Signal, 1),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts the application
func (a *App) Start() error {
	a.logger.Info("Starting FreeSWITCH ESL Sidecar App",
		zap.String("version", "0.1.0"),
	)

	// Setup signal handling for graceful shutdown
	signal.Notify(a.shutdown, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Start application components
	if err := a.startComponents(); err != nil {
		return fmt.Errorf("failed to start components: %w", err)
	}

	a.logger.Info("Application started successfully")

	// Wait for shutdown signal or context cancellation
	select {
	case sig := <-a.shutdown:
		a.logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
	case <-a.ctx.Done():
		a.logger.Info("Application context cancelled")
	}

	return a.Stop()
}

// Stop stops the application gracefully
func (a *App) Stop() error {
	a.logger.Info("Shutting down application...")

	// Cancel context to signal all components to stop
	a.cancel()

	// Create a timeout context for graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		a.logger.Info("All components stopped gracefully")
	case <-shutdownCtx.Done():
		a.logger.Warn("Graceful shutdown timeout, forcing exit")
	}

	// Sync logger before exit
	if err := logger.Sync(); err != nil {
		// Don't return error on sync failure, just log it
		fmt.Printf("Failed to sync logger: %v\n", err)
	}

	a.logger.Info("Application shutdown complete")
	return nil
}

// startComponents starts all application components
func (a *App) startComponents() error {
	a.logger.Info("Starting application components")

	// TODO: Start ESL client
	a.logger.Debug("ESL client component not implemented yet")

	// TODO: Start HTTP client
	a.logger.Debug("HTTP client component not implemented yet")

	// TODO: Start metrics server
	if a.config.Metrics.Enabled {
		a.logger.Debug("Metrics server component not implemented yet")
	}

	// Demonstrate a background worker
	a.startDemoWorker()

	return nil
}

// startDemoWorker starts a demo background worker to show graceful shutdown
func (a *App) startDemoWorker() {
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		workerLogger := a.logger.With(zap.String("component", "demo-worker"))
		workerLogger.Info("Demo worker started")

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-a.ctx.Done():
				workerLogger.Info("Demo worker shutting down")
				return
			case <-ticker.C:
				workerLogger.Debug("Demo worker heartbeat")
			}
		}
	}()
}

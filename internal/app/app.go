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
	"fsevents/internal/esl"
	"fsevents/internal/logger"
	"fsevents/pkg/types"
)

// App represents the main application
type App struct {
	config    *config.Config
	logger    *zap.Logger
	shutdown  chan os.Signal
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	eslClient *esl.Client
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
	a.logger.Info("Signal handling setup complete")

	// Start application components
	if err := a.startComponents(); err != nil {
		return fmt.Errorf("failed to start components: %w", err)
	}

	a.logger.Info("Application started successfully, waiting for signals...")

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

	// Stop ESL client first
	if a.eslClient != nil {
		if err := a.eslClient.Close(); err != nil {
			a.logger.Error("Error closing ESL client", zap.Error(err))
		}
	}

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

	// Start ESL client
	if err := a.startESLClient(); err != nil {
		return fmt.Errorf("failed to start ESL client: %w", err)
	}

	// Start event processor
	a.logger.Info("Starting event processor...")
	a.startEventProcessor()
	a.logger.Info("Event processor started")

	// TODO: Start HTTP client
	a.logger.Debug("HTTP client component not implemented yet")

	// TODO: Start metrics server
	if a.config.Metrics.Enabled {
		a.logger.Debug("Metrics server component not implemented yet")
	}

	a.logger.Info("All components started successfully")
	return nil
}

// startESLClient starts the ESL client
func (a *App) startESLClient() error {
	a.logger.Info("Starting ESL client",
		zap.String("host", a.config.ESL.Host),
		zap.Int("port", a.config.ESL.Port),
	)

	// Create ESL client
	a.eslClient = esl.NewClient(&a.config.ESL, a.logger)

	// Connect to FreeSWITCH with configured events
	events := a.config.Events.SubscribeEvents
	if err := a.eslClient.ConnectWithEvents(events); err != nil {
		return fmt.Errorf("failed to connect to FreeSWITCH ESL server: %w", err)
	}

	a.logger.Info("ESL client started successfully")
	return nil
}

// startEventProcessor starts processing events from ESL client
func (a *App) startEventProcessor() {
	a.logger.Info("Starting event processor")

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		eventChan := a.eslClient.Events()

		for {
			select {
			case <-a.ctx.Done():
				a.logger.Debug("Event processor shutting down")
				return
			case event, ok := <-eventChan:
				if !ok {
					a.logger.Debug("Event channel closed, shutting down processor")
					return
				}
				a.processEvent(event)
			}
		}
	}()
}

// processEvent processes a single event
func (a *App) processEvent(event *types.Event) {
	a.logger.Info("Processing event",
		zap.String("event_name", event.Name),
		zap.String("event_subclass", event.Subclass),
		zap.String("unique_id", event.GetHeader("Unique-ID")),
		zap.String("caller_id_number", event.GetHeader("Caller-Caller-ID-Number")),
		zap.String("destination_number", event.GetHeader("Caller-Destination-Number")),
	)

	// TODO: Apply event filters
	// TODO: Forward to HTTP endpoints

	// For now, just log the event details
	if event.IsChannelEvent() {
		if channel := event.GetChannelInfo(); channel != nil {
			a.logger.Info("Channel event details",
				zap.String("channel_uuid", channel.UUID),
				zap.String("direction", channel.Direction),
				zap.String("state", channel.State),
				zap.String("caller_id", channel.CallerIDNumber),
				zap.String("destination", channel.DestinationNumber),
			)
		}
	}

	if event.IsCustomEvent() {
		a.logger.Info("Custom event received",
			zap.String("subclass", event.Subclass),
			zap.Any("headers", event.Headers),
		)
	}
}

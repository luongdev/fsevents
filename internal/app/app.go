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
	"fsevents/internal/http"
	"fsevents/internal/logger"
	"fsevents/internal/processor"
	"fsevents/internal/worker"
	"fsevents/pkg/types"
)

// App represents the main application
type App struct {
	config           *config.Config
	logger           *zap.Logger
	shutdown         chan os.Signal
	wg               sync.WaitGroup
	ctx              context.Context
	cancel           context.CancelFunc
	eslClient        *esl.Client
	eventFilter      *processor.EventFilter
	fieldMapper      *processor.FieldMapper
	processorManager *processor.ProcessorManager
	httpClient       *http.Client
	workerPool       *worker.Pool
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

	// Stop ESL client first (stops accepting new events)
	if a.eslClient != nil {
		if err := a.eslClient.Close(); err != nil {
			a.logger.Error("Error closing ESL client", zap.Error(err))
		}
	}

	// Create a timeout context for graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Stop worker pool (waits for workers to finish)
	if a.workerPool != nil {
		done := make(chan struct{})
		go func() {
			if err := a.workerPool.Stop(); err != nil {
				a.logger.Error("Error stopping worker pool", zap.Error(err))
			}
			close(done)
		}()

		select {
		case <-done:
			a.logger.Info("Worker pool stopped gracefully")
		case <-shutdownCtx.Done():
			a.logger.Warn("Worker pool shutdown timeout, forcing exit")
		}
	}

	// Wait for all other goroutines to finish with timeout
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

	// Initialize event filter
	a.eventFilter = processor.NewEventFilter(a.config.Events.Filters, a.config.Events.FilterLogic, a.logger)
	a.logger.Info("Event filter initialized",
		zap.Int("filter_count", a.eventFilter.GetFilterCount()))

	// Initialize field mapper
	a.fieldMapper = processor.NewFieldMapper(a.config.Events.FieldMappings, a.config.Events.EventFieldMappings, a.config.Events.FieldFilters, a.logger)
	a.logger.Info("Field mapper initialized",
		zap.Int("mapping_count", a.fieldMapper.GetMappingCount()))

	// Initialize processor manager
	var err error
	a.processorManager, err = processor.NewProcessorManager(a.config.Events.Processors, a.logger)
	if err != nil {
		return fmt.Errorf("failed to initialize processor manager: %w", err)
	}
	a.logger.Info("Processor manager initialized",
		zap.Int("processor_count", a.processorManager.GetProcessorCount()))

	// Initialize HTTP client
	a.httpClient = http.NewClient(a.config.HTTP.Destinations, a.fieldMapper, a.processorManager, a.logger)
	a.logger.Info("HTTP client initialized",
		zap.Int("destination_count", a.httpClient.GetDestinationCount()))

	// Initialize worker pool
	eventProcessor := &EventProcessorImpl{
		eventFilter:      a.eventFilter,
		fieldMapper:      a.fieldMapper,
		processorManager: a.processorManager,
		httpClient:       a.httpClient,
		logger:           a.logger,
		ctx:              a.ctx,
	}

	workerCfg := worker.Config{
		WorkerCount: a.config.Processing.WorkerCount,
		QueueSize:   a.config.Processing.EventBufferSize,
	}
	a.workerPool = worker.NewPool(workerCfg, eventProcessor, a.logger)

	// Start worker pool
	if err := a.workerPool.Start(); err != nil {
		return fmt.Errorf("failed to start worker pool: %w", err)
	}

	// Start ESL client
	if err := a.startESLClient(); err != nil {
		return fmt.Errorf("failed to start ESL client: %w", err)
	}

	// Start event processor
	a.logger.Info("Starting event processor...")
	a.startEventProcessor()
	a.logger.Info("Event processor started")

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
		zap.Int("buffer_size", a.config.Processing.EventBufferSize),
	)

	// Create ESL client with configured buffer size
	a.eslClient = esl.NewClientWithBuffer(&a.config.ESL, a.config.Processing.EventBufferSize, a.logger)

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
				// Submit to worker pool instead of processing directly
				if err := a.workerPool.Submit(event); err != nil {
					a.logger.Warn("Failed to submit event to worker pool",
						zap.String("event_name", event.Name),
						zap.Error(err))
				}
			}
		}
	}()
}

// EventProcessorImpl implements worker.EventProcessor interface
type EventProcessorImpl struct {
	eventFilter      *processor.EventFilter
	fieldMapper      *processor.FieldMapper
	processorManager *processor.ProcessorManager
	httpClient       *http.Client
	logger           *zap.Logger
	ctx              context.Context
}

// ProcessEvent processes a single event (implements worker.EventProcessor)
func (p *EventProcessorImpl) ProcessEvent(ctx context.Context, event *types.Event) error {
	// Apply event filters first
	if !p.eventFilter.ShouldProcess(event) {
		// Event was filtered out - log at debug level
		p.logger.Debug("Event filtered out",
			zap.String("event_name", event.Name),
			zap.String("unique_id", event.GetHeader("Unique-ID")),
		)
		return nil
	}

	// Forward to HTTP endpoints
	httpCtx, cancel := context.WithTimeout(p.ctx, 60*time.Second)
	defer cancel()

	if err := p.httpClient.ForwardEvent(httpCtx, event); err != nil {
		p.logger.Error("Failed to forward event to HTTP destinations",
			zap.String("event_name", event.Name),
			zap.String("unique_id", event.GetHeader("Unique-ID")),
			zap.Error(err))
		// Don't return error - we want to continue processing other events
	}

	return nil
}

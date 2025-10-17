package worker

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"fsevents/pkg/types"
)

// EventProcessor defines the interface for processing events
type EventProcessor interface {
	ProcessEvent(ctx context.Context, event *types.Event) error
}

// Pool manages a pool of worker goroutines for concurrent event processing
type Pool struct {
	workerCount   int
	eventQueue    chan *types.Event
	wg            sync.WaitGroup
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *zap.Logger
	processor     EventProcessor
	activeWorkers atomic.Int32

	// Metrics
	eventsReceived  atomic.Uint64
	eventsProcessed atomic.Uint64
	eventsDropped   atomic.Uint64
}

// Config holds worker pool configuration
type Config struct {
	WorkerCount int // Number of worker goroutines (0 = CPU count)
	QueueSize   int // Size of event queue buffer
}

// NewPool creates a new worker pool
func NewPool(cfg Config, processor EventProcessor, logger *zap.Logger) *Pool {
	ctx, cancel := context.WithCancel(context.Background())

	// Default to CPU count if not specified
	workerCount := cfg.WorkerCount
	if workerCount <= 0 {
		workerCount = runtime.NumCPU()
	}

	// Ensure queue size is positive
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = 5000
	}

	return &Pool{
		workerCount: workerCount,
		eventQueue:  make(chan *types.Event, queueSize),
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger.Named("worker-pool"),
		processor:   processor,
	}
}

// Start starts all workers in the pool
func (p *Pool) Start() error {
	p.logger.Info("Starting worker pool",
		zap.Int("worker_count", p.workerCount),
		zap.Int("queue_size", cap(p.eventQueue)))

	for i := 0; i < p.workerCount; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}

	// Start metrics logger
	p.wg.Add(1)
	go p.metricsLogger()

	p.logger.Info("Worker pool started successfully")
	return nil
}

// worker processes events from the queue
func (p *Pool) worker(id int) {
	defer func() {
		if r := recover(); r != nil {
			p.logger.Error("Worker panic recovered",
				zap.Int("worker_id", id),
				zap.Any("panic", r),
				zap.Stack("stack"))
			p.eventsDropped.Add(1)
		}
		p.wg.Done()
	}()

	p.logger.Debug("Worker started", zap.Int("worker_id", id))

	for {
		select {
		case <-p.ctx.Done():
			p.logger.Debug("Worker stopping", zap.Int("worker_id", id))
			return
		case event, ok := <-p.eventQueue:
			if !ok {
				p.logger.Debug("Event queue closed, worker stopping", zap.Int("worker_id", id))
				return
			}

			// Track active workers
			p.activeWorkers.Add(1)

			// Process event
			if err := p.processor.ProcessEvent(p.ctx, event); err != nil {
				p.logger.Error("Failed to process event",
					zap.Int("worker_id", id),
					zap.String("event_name", event.Name),
					zap.Error(err))
			}

			p.eventsProcessed.Add(1)
			p.activeWorkers.Add(-1)
		}
	}
}

// Submit submits an event to the worker pool for processing
func (p *Pool) Submit(event *types.Event) error {
	p.eventsReceived.Add(1)

	select {
	case p.eventQueue <- event:
		return nil
	default:
		// Queue is full, drop event
		p.eventsDropped.Add(1)
		queueDepth := len(p.eventQueue)
		queueCap := cap(p.eventQueue)
		p.logger.Warn("Worker queue full, dropping event",
			zap.String("event_name", event.Name),
			zap.Int("queue_depth", queueDepth),
			zap.Int("queue_capacity", queueCap),
			zap.Float64("queue_usage_percent", float64(queueDepth)/float64(queueCap)*100))
		return fmt.Errorf("worker queue full")
	}
}

// Stop gracefully stops the worker pool
func (p *Pool) Stop() error {
	p.logger.Info("Stopping worker pool...")

	// Stop accepting new events
	close(p.eventQueue)

	// Cancel context to signal workers
	p.cancel()

	// Wait for all workers to finish
	p.wg.Wait()

	p.logger.Info("Worker pool stopped",
		zap.Uint64("events_received", p.eventsReceived.Load()),
		zap.Uint64("events_processed", p.eventsProcessed.Load()),
		zap.Uint64("events_dropped", p.eventsDropped.Load()))

	return nil
}

// GetQueueDepth returns current queue depth
func (p *Pool) GetQueueDepth() int {
	return len(p.eventQueue)
}

// GetQueueCapacity returns queue capacity
func (p *Pool) GetQueueCapacity() int {
	return cap(p.eventQueue)
}

// GetActiveWorkers returns number of currently active workers
func (p *Pool) GetActiveWorkers() int {
	return int(p.activeWorkers.Load())
}

// GetMetrics returns current metrics
func (p *Pool) GetMetrics() map[string]interface{} {
	queueDepth := p.GetQueueDepth()
	queueCap := p.GetQueueCapacity()

	return map[string]interface{}{
		"events_received":     p.eventsReceived.Load(),
		"events_processed":    p.eventsProcessed.Load(),
		"events_dropped":      p.eventsDropped.Load(),
		"queue_depth":         queueDepth,
		"queue_capacity":      queueCap,
		"queue_usage_percent": float64(queueDepth) / float64(queueCap) * 100,
		"active_workers":      p.GetActiveWorkers(),
		"total_workers":       p.workerCount,
	}
}

// metricsLogger periodically logs metrics
func (p *Pool) metricsLogger() {
	defer p.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	lastReceived := uint64(0)
	lastProcessed := uint64(0)
	lastTime := time.Now()

	for {
		select {
		case <-p.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			elapsed := now.Sub(lastTime).Seconds()

			received := p.eventsReceived.Load()
			processed := p.eventsProcessed.Load()
			dropped := p.eventsDropped.Load()

			receivedRate := float64(received-lastReceived) / elapsed
			processedRate := float64(processed-lastProcessed) / elapsed

			queueDepth := p.GetQueueDepth()
			queueCap := p.GetQueueCapacity()
			queueUsage := float64(queueDepth) / float64(queueCap) * 100

			p.logger.Info("Worker pool metrics",
				zap.Float64("events_per_sec_received", receivedRate),
				zap.Float64("events_per_sec_processed", processedRate),
				zap.Uint64("total_received", received),
				zap.Uint64("total_processed", processed),
				zap.Uint64("total_dropped", dropped),
				zap.Int("queue_depth", queueDepth),
				zap.Float64("queue_usage_percent", queueUsage),
				zap.Int("active_workers", p.GetActiveWorkers()),
				zap.Int("total_workers", p.workerCount))

			// Warn if queue is getting full
			if queueUsage > 80 {
				p.logger.Warn("Worker queue usage high",
					zap.Float64("queue_usage_percent", queueUsage),
					zap.Int("queue_depth", queueDepth),
					zap.Int("queue_capacity", queueCap))
			}

			lastReceived = received
			lastProcessed = processed
			lastTime = now
		}
	}
}

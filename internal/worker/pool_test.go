package worker

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"fsevents/pkg/types"
)

// MockEventProcessor is a mock implementation of EventProcessor for testing
type MockEventProcessor struct {
	processedCount atomic.Uint64
	processDelay   time.Duration
	shouldError    bool
	mu             sync.Mutex
	events         []*types.Event
}

func (m *MockEventProcessor) ProcessEvent(ctx context.Context, event *types.Event) error {
	m.processedCount.Add(1)

	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()

	if m.processDelay > 0 {
		time.Sleep(m.processDelay)
	}

	if m.shouldError {
		return context.DeadlineExceeded
	}

	return nil
}

func (m *MockEventProcessor) GetProcessedCount() uint64 {
	return m.processedCount.Load()
}

func (m *MockEventProcessor) GetEvents() []*types.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*types.Event{}, m.events...)
}

func TestNewPool(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockEventProcessor{}

	cfg := Config{
		WorkerCount: 4,
		QueueSize:   100,
	}

	pool := NewPool(cfg, processor, logger)

	if pool == nil {
		t.Fatal("Expected pool to be created")
	}

	if pool.workerCount != 4 {
		t.Errorf("Expected worker count 4, got %d", pool.workerCount)
	}

	if cap(pool.eventQueue) != 100 {
		t.Errorf("Expected queue size 100, got %d", cap(pool.eventQueue))
	}
}

func TestNewPoolDefaultWorkerCount(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockEventProcessor{}

	cfg := Config{
		WorkerCount: 0, // Should default to CPU count
		QueueSize:   100,
	}

	pool := NewPool(cfg, processor, logger)

	if pool.workerCount <= 0 {
		t.Errorf("Expected positive worker count, got %d", pool.workerCount)
	}
}

func TestPoolStartStop(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockEventProcessor{}

	cfg := Config{
		WorkerCount: 2,
		QueueSize:   10,
	}

	pool := NewPool(cfg, processor, logger)

	// Start pool
	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}

	// Give workers time to start
	time.Sleep(100 * time.Millisecond)

	// Stop pool
	if err := pool.Stop(); err != nil {
		t.Fatalf("Failed to stop pool: %v", err)
	}
}

func TestPoolSubmitAndProcess(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockEventProcessor{}

	cfg := Config{
		WorkerCount: 2,
		QueueSize:   10,
	}

	pool := NewPool(cfg, processor, logger)

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// Submit events
	numEvents := 5
	for i := 0; i < numEvents; i++ {
		event := &types.Event{
			Name:      "TEST_EVENT",
			Timestamp: time.Now(),
			Headers:   map[string]string{"test": "value"},
		}

		if err := pool.Submit(event); err != nil {
			t.Errorf("Failed to submit event: %v", err)
		}
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Check processed count
	processed := processor.GetProcessedCount()
	if processed != uint64(numEvents) {
		t.Errorf("Expected %d events processed, got %d", numEvents, processed)
	}

	// Check metrics
	metrics := pool.GetMetrics()
	if metrics["events_received"].(uint64) != uint64(numEvents) {
		t.Errorf("Expected %d events received in metrics, got %d", numEvents, metrics["events_received"])
	}
}

func TestPoolQueueFull(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockEventProcessor{
		processDelay: 100 * time.Millisecond, // Slow processing
	}

	cfg := Config{
		WorkerCount: 1,
		QueueSize:   5, // Small queue
	}

	pool := NewPool(cfg, processor, logger)

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// Submit more events than queue can hold
	numEvents := 20
	droppedCount := 0

	for i := 0; i < numEvents; i++ {
		event := &types.Event{
			Name:      "TEST_EVENT",
			Timestamp: time.Now(),
		}

		if err := pool.Submit(event); err != nil {
			droppedCount++
		}
	}

	// Should have dropped some events
	if droppedCount == 0 {
		t.Error("Expected some events to be dropped when queue is full")
	}

	// Check metrics
	time.Sleep(200 * time.Millisecond)
	metrics := pool.GetMetrics()
	dropped := metrics["events_dropped"].(uint64)

	if dropped == 0 {
		t.Error("Expected dropped events counter to be > 0")
	}

	t.Logf("Dropped %d events out of %d submitted", droppedCount, numEvents)
}

func TestPoolConcurrentProcessing(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockEventProcessor{
		processDelay: 50 * time.Millisecond,
	}

	cfg := Config{
		WorkerCount: 4, // Multiple workers
		QueueSize:   100,
	}

	pool := NewPool(cfg, processor, logger)

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// Submit many events
	numEvents := 20
	start := time.Now()

	for i := 0; i < numEvents; i++ {
		event := &types.Event{
			Name:      "TEST_EVENT",
			Timestamp: time.Now(),
		}

		if err := pool.Submit(event); err != nil {
			t.Errorf("Failed to submit event: %v", err)
		}
	}

	// Wait for all to process
	time.Sleep(2 * time.Second)
	elapsed := time.Since(start)

	// With 4 workers and 50ms delay, should process much faster than sequential
	// Sequential would take: 20 * 50ms = 1000ms
	// Concurrent should take: ~250-500ms (depending on scheduling)
	if elapsed > 1*time.Second {
		t.Logf("Warning: Processing took %v, might not be concurrent", elapsed)
	}

	processed := processor.GetProcessedCount()
	if processed != uint64(numEvents) {
		t.Errorf("Expected %d events processed, got %d", numEvents, processed)
	}

	t.Logf("Processed %d events in %v with %d workers", numEvents, elapsed, cfg.WorkerCount)
}

func TestPoolGracefulShutdown(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockEventProcessor{
		processDelay: 100 * time.Millisecond,
	}

	cfg := Config{
		WorkerCount: 2,
		QueueSize:   10,
	}

	pool := NewPool(cfg, processor, logger)

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}

	// Submit events
	numEvents := 5
	for i := 0; i < numEvents; i++ {
		event := &types.Event{
			Name:      "TEST_EVENT",
			Timestamp: time.Now(),
		}
		pool.Submit(event)
	}

	// Stop immediately (should wait for in-flight events)
	start := time.Now()
	if err := pool.Stop(); err != nil {
		t.Fatalf("Failed to stop pool: %v", err)
	}
	elapsed := time.Since(start)

	// Should have waited for events to finish
	if elapsed < 50*time.Millisecond {
		t.Error("Stop returned too quickly, might not have waited for workers")
	}

	// All events should be processed
	processed := processor.GetProcessedCount()
	if processed != uint64(numEvents) {
		t.Errorf("Expected %d events processed after shutdown, got %d", numEvents, processed)
	}
}

func TestPoolMetrics(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockEventProcessor{}

	cfg := Config{
		WorkerCount: 2,
		QueueSize:   10,
	}

	pool := NewPool(cfg, processor, logger)

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}
	defer pool.Stop()

	// Submit events
	numEvents := 3
	for i := 0; i < numEvents; i++ {
		event := &types.Event{
			Name:      "TEST_EVENT",
			Timestamp: time.Now(),
		}
		pool.Submit(event)
	}

	time.Sleep(200 * time.Millisecond)

	// Check metrics
	metrics := pool.GetMetrics()

	if metrics["events_received"].(uint64) != uint64(numEvents) {
		t.Errorf("Expected events_received=%d, got %d", numEvents, metrics["events_received"])
	}

	if metrics["events_processed"].(uint64) != uint64(numEvents) {
		t.Errorf("Expected events_processed=%d, got %d", numEvents, metrics["events_processed"])
	}

	if metrics["total_workers"].(int) != 2 {
		t.Errorf("Expected total_workers=2, got %d", metrics["total_workers"])
	}

	queueDepth := metrics["queue_depth"].(int)
	if queueDepth < 0 {
		t.Errorf("Queue depth should be non-negative, got %d", queueDepth)
	}
}

// Test with race detector: go test -race
func TestPoolRaceConditions(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	processor := &MockEventProcessor{}

	cfg := Config{
		WorkerCount: 4,
		QueueSize:   50,
	}

	pool := NewPool(cfg, processor, logger)

	if err := pool.Start(); err != nil {
		t.Fatalf("Failed to start pool: %v", err)
	}

	// Submit from multiple goroutines
	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				event := &types.Event{
					Name:      "TEST_EVENT",
					Timestamp: time.Now(),
				}
				pool.Submit(event)
			}
		}()
	}

	wg.Wait()
	time.Sleep(500 * time.Millisecond)

	pool.Stop()

	// Verify counts
	totalExpected := uint64(numGoroutines * eventsPerGoroutine)
	processed := processor.GetProcessedCount()

	if processed != totalExpected {
		t.Errorf("Expected %d events processed, got %d", totalExpected, processed)
	}
}

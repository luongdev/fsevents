package esl

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/percipia/eslgo"
	"github.com/percipia/eslgo/command"
	"go.uber.org/zap"

	"fsevents/internal/config"
	"fsevents/pkg/types"
)

// Client represents an ESL client connection to FreeSWITCH
type Client struct {
	config     *config.ESLConfig
	logger     *zap.Logger
	conn       *eslgo.Conn
	events     chan *types.Event
	isRunning  atomic.Bool
	mu         sync.RWMutex
	stats      *ConnectionStats
	ctx        context.Context
	cancel     context.CancelFunc
	listenerID string
}

// ConnectionStats tracks ESL connection statistics
type ConnectionStats struct {
	ConnectedAt       time.Time
	EventsReceived    uint64
	EventsProcessed   uint64
	ReconnectAttempts uint64
	LastError         error
	mu                sync.RWMutex
}

// NewClient creates a new ESL client
func NewClient(config *config.ESLConfig, logger *zap.Logger) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		config: config,
		logger: logger.Named("esl"),
		events: make(chan *types.Event, 1000), // Buffer for events
		stats: &ConnectionStats{
			ConnectedAt: time.Now(),
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

// Connect establishes connection to FreeSWITCH ESL with events to subscribe
func (c *Client) Connect() error {
	return c.ConnectWithEvents(nil)
}

// ConnectWithEvents establishes connection to FreeSWITCH ESL with specific events
func (c *Client) ConnectWithEvents(events []string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isRunning.Load() {
		return fmt.Errorf("ESL client is already connected")
	}

	c.logger.Info("Connecting to FreeSWITCH ESL",
		zap.String("host", c.config.Host),
		zap.Int("port", c.config.Port))

	// Create connection address
	addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)

	// Connect with disconnect handler
	conn, err := eslgo.Dial(addr, c.config.Password, func() {
		c.logger.Warn("ESL connection disconnected")
		c.handleDisconnect()
	})
	if err != nil {
		c.updateStats(func(s *ConnectionStats) {
			s.LastError = err
			s.ReconnectAttempts++
		})
		return fmt.Errorf("failed to connect to ESL: %w", err)
	}

	c.conn = conn
	c.isRunning.Store(true)

	c.updateStats(func(s *ConnectionStats) {
		s.ConnectedAt = time.Now()
		s.LastError = nil
	})

	c.logger.Info("Successfully connected to FreeSWITCH ESL")

	// Register event listener for all events
	c.listenerID = c.conn.RegisterEventListener(eslgo.EventListenAll, c.handleEvent)

	// Enable events
	if err := c.subscribeToEvents(events); err != nil {
		c.Close()
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	return nil
}

// subscribeToEvents subscribes to configured FreeSWITCH events
func (c *Client) subscribeToEvents(configuredEvents []string) error {
	// Get events from configuration or use defaults
	eventNames := c.getConfiguredEvents(configuredEvents)

	c.logger.Info("Subscribing to events", zap.Strings("events", eventNames))

	// Use specific event subscription instead of EnableEvents (which gets all events)
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	// Build event command for specific events
	eventCmd := command.Event{
		Format: "plain",
		Listen: eventNames,
	}

	c.logger.Info("Sending event command...")
	response, err := c.conn.SendCommand(ctx, eventCmd)
	if err != nil {
		c.logger.Error("Event command failed", zap.Error(err))
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	if !response.IsOk() {
		replyText := response.GetHeader("Reply-Text")
		c.logger.Error("Event subscription failed", zap.String("reply", replyText))
		return fmt.Errorf("event subscription failed: %s", replyText)
	}

	c.logger.Info("Successfully subscribed to FreeSWITCH events")
	return nil
}

// handleEvent handles incoming ESL events
func (c *Client) handleEvent(event *eslgo.Event) {
	if !c.isRunning.Load() {
		return
	}

	// Convert to our event format
	fsEvent := c.convertEvent(event)
	if fsEvent != nil {
		c.updateStats(func(s *ConnectionStats) {
			s.EventsReceived++
		})

		// Send to event channel (non-blocking)
		select {
		case c.events <- fsEvent:
			c.updateStats(func(s *ConnectionStats) {
				s.EventsProcessed++
			})
		default:
			c.logger.Warn("Event channel is full, dropping event",
				zap.String("event_type", fsEvent.Name))
		}
	}
}

// convertEvent converts ESL event to our internal format
func (c *Client) convertEvent(event *eslgo.Event) *types.Event {
	if event == nil {
		return nil
	}

	// Get event name
	eventName := event.GetHeader("Event-Name")
	if eventName == "" {
		return nil
	}

	fsEvent := &types.Event{
		Name:      eventName,
		Timestamp: time.Now(),
		Headers:   make(map[string]string),
		Body:      string(event.Body),
	}

	// Copy all headers
	for key, values := range event.Headers {
		if len(values) > 0 {
			fsEvent.Headers[key] = values[0]
		}
	}

	// Extract subclass for CUSTOM events
	if eventName == "CUSTOM" {
		if subclass := event.GetHeader("Event-Subclass"); subclass != "" {
			fsEvent.Subclass = subclass
		}
	}

	c.logger.Debug("Converted ESL event",
		zap.String("event_name", fsEvent.Name),
		zap.String("event_subclass", fsEvent.Subclass),
		zap.String("unique_id", fsEvent.GetHeader("Unique-ID")),
		zap.Int("headers_count", len(fsEvent.Headers)))

	return fsEvent
}

// getConfiguredEvents returns events to subscribe to from config or defaults
func (c *Client) getConfiguredEvents(configuredEvents []string) []string {
	// Use configured events if provided
	if len(configuredEvents) > 0 {
		return configuredEvents
	}

	// Default events if none configured
	defaultEvents := []string{"HEARTBEAT"}

	return defaultEvents
}

// Events returns the channel for receiving events
func (c *Client) Events() <-chan *types.Event {
	return c.events
}

// IsConnected returns true if client is connected
func (c *Client) IsConnected() bool {
	return c.isRunning.Load()
}

// GetStats returns connection statistics
func (c *Client) GetStats() ConnectionStats {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()

	// Copy individual fields to avoid copying the mutex
	return ConnectionStats{
		ConnectedAt:       c.stats.ConnectedAt,
		EventsReceived:    c.stats.EventsReceived,
		EventsProcessed:   c.stats.EventsProcessed,
		ReconnectAttempts: c.stats.ReconnectAttempts,
		LastError:         c.stats.LastError,
		// Don't copy the mutex
	}
}

// handleDisconnect handles ESL disconnection
func (c *Client) handleDisconnect() {
	if !c.isRunning.Load() {
		return // Already disconnected
	}

	c.logger.Warn("ESL connection lost")
	c.isRunning.Store(false)

	c.updateStats(func(s *ConnectionStats) {
		s.LastError = fmt.Errorf("connection lost")
	})
}

// updateStats safely updates connection statistics
func (c *Client) updateStats(updateFn func(*ConnectionStats)) {
	c.stats.mu.Lock()
	defer c.stats.mu.Unlock()
	updateFn(c.stats)
}

// Close closes the ESL connection
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isRunning.Load() {
		return nil
	}

	c.logger.Info("Closing ESL connection")

	c.isRunning.Store(false)
	c.cancel()

	if c.conn != nil {
		// Remove event listener
		if c.listenerID != "" {
			c.conn.RemoveEventListener(eslgo.EventListenAll, c.listenerID)
		}

		// Close connection gracefully
		c.conn.ExitAndClose()
		c.conn = nil
	}

	close(c.events)

	c.logger.Info("ESL connection closed")
	return nil
}

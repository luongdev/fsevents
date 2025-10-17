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
	"strings"
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
	// runtime
	configuredEvents []string
	hbStopCh         chan struct{}
	reconnecting     atomic.Bool
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

// NewClient creates a new ESL client with default buffer size
func NewClient(config *config.ESLConfig, logger *zap.Logger) *Client {
	return NewClientWithBuffer(config, 1000, logger)
}

// NewClientWithBuffer creates a new ESL client with configurable buffer size
func NewClientWithBuffer(config *config.ESLConfig, bufferSize int, logger *zap.Logger) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	return &Client{
		config: config,
		logger: logger.Named("esl"),
		events: make(chan *types.Event, bufferSize), // Configurable buffer for events
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
	c.configuredEvents = c.getConfiguredEvents(events)
	if err := c.subscribeToEvents(c.configuredEvents); err != nil {
		// Do not call Close() here since we are holding c.mu; close connection directly
		if c.conn != nil {
			c.conn.ExitAndClose()
			c.conn = nil
		}
		c.isRunning.Store(false)
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	// Start keepalive/heartbeat if enabled
	if c.config.Keepalive.Enabled {
		c.startHeartbeat()
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

	c.logger.Info(fmt.Sprintf("Sending event command... %v", eventCmd.BuildMessage()))
	response, err := c.conn.SendCommand(ctx, eventCmd)
	if err != nil {
		c.logger.Error("Event command failed", zap.Error(err))
		return fmt.Errorf("failed to subscribe to events: %w", err)
	}

	// Treat as success unless Reply-Text begins with -ERR
	if !c.responseIndicatesSuccess(response) {
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

	// Attempt reconnection
	c.triggerReconnect()
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
	if c.hbStopCh != nil {
		close(c.hbStopCh)
		c.hbStopCh = nil
	}

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

// startHeartbeat launches a goroutine that periodically calls FreeSWITCH status API
func (c *Client) startHeartbeat() {
	if c.hbStopCh != nil {
		return
	}
	c.hbStopCh = make(chan struct{})
	failureCount := 0
	interval := c.config.Keepalive.Interval
	timeout := c.config.Keepalive.Timeout
	threshold := c.config.Keepalive.FailureThreshold

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-c.hbStopCh:
				return
			case <-c.ctx.Done():
				return
			case <-ticker.C:
				if !c.isRunning.Load() {
					continue
				}
				// Send status API
				ctx, cancel := context.WithTimeout(c.ctx, timeout)
				resp, err := c.conn.SendCommand(ctx, command.API{Command: "status"})
				cancel()
				if err != nil || resp == nil || !c.responseIndicatesSuccess(resp) {
					failureCount++
					c.logger.Warn("ESL heartbeat failed", zap.Int("consecutive_failures", failureCount), zap.Error(err))
					if failureCount >= threshold {
						c.logger.Error("ESL heartbeat threshold exceeded, triggering reconnect")
						c.handleDisconnect()
					}
					continue
				}
				// success
				failureCount = 0
			}
		}
	}()
}

// responseIndicatesSuccess returns true unless Reply-Text begins with "-ERR"
func (c *Client) responseIndicatesSuccess(resp *eslgo.RawResponse) bool {
	if resp == nil {
		return false
	}
	reply := resp.GetHeader("Reply-Text")
	if len(reply) == 0 {
		// If there is no explicit reply, consider it success by default
		return true
	}
	return !strings.HasPrefix(reply, "-ERR")
}

// triggerReconnect starts a reconnection loop if not already running
func (c *Client) triggerReconnect() {
	if !c.reconnecting.CompareAndSwap(false, true) {
		return // already reconnecting
	}

	go func() {
		defer c.reconnecting.Store(false)

		attempts := 0
		for {
			if c.ctx.Err() != nil {
				return
			}
			// Respect max attempts (0 means infinite)
			if c.config.MaxReconnectAttempts > 0 && attempts >= c.config.MaxReconnectAttempts {
				c.logger.Error("Max reconnect attempts reached; giving up")
				return
			}

			attempts++
			c.updateStats(func(s *ConnectionStats) { s.ReconnectAttempts++ })
			c.logger.Info("Attempting to reconnect to FreeSWITCH ESL", zap.Int("attempt", attempts))

			// Establish new connection
			addr := fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
			conn, err := eslgo.Dial(addr, c.config.Password, func() {
				c.logger.Warn("ESL connection disconnected (during reconnect)")
				c.handleDisconnect()
			})
			if err != nil {
				c.logger.Warn("Reconnect attempt failed", zap.Error(err))
				time.Sleep(c.config.ReconnectInterval)
				continue
			}

			// Swap connection safely
			c.mu.Lock()
			c.conn = conn
			c.isRunning.Store(true)
			// re-register listener
			c.listenerID = c.conn.RegisterEventListener(eslgo.EventListenAll, c.handleEvent)
			c.mu.Unlock()

			// Re-subscribe to events using cached list
			if err := c.subscribeToEvents(c.configuredEvents); err != nil {
				c.logger.Error("Failed to re-subscribe to events after reconnect", zap.Error(err))
				// close and try again
				c.conn.ExitAndClose()
				time.Sleep(c.config.ReconnectInterval)
				continue
			}

			c.logger.Info("Reconnected to FreeSWITCH ESL successfully")

			// restart heartbeat if enabled
			if c.config.Keepalive.Enabled {
				c.startHeartbeat()
			}
			return
		}
	}()
}

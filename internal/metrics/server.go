package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"fsevents/internal/config"
	"fsevents/internal/esl"
	"fsevents/internal/worker"
)

// Server represents the metrics HTTP server
type Server struct {
	config     *config.MetricsConfig
	logger     *zap.Logger
	server     *http.Server
	eslClient  *esl.Client
	workerPool *worker.Pool
	mu         sync.RWMutex
	startTime  time.Time
}

// MetricsData represents the complete metrics response
type MetricsData struct {
	// Application info
	StartTime time.Time `json:"start_time"`
	Uptime    string    `json:"uptime"`

	// ESL connection metrics
	ESLConnected      bool      `json:"esl_connected"`
	ESLConnectedAt    time.Time `json:"esl_connected_at,omitempty"`
	ESLEventsReceived uint64    `json:"esl_events_received"`
	ESLReconnects     uint64    `json:"esl_reconnect_attempts"`

	// Worker pool metrics
	EventsReceived    uint64  `json:"events_received"`
	EventsProcessed   uint64  `json:"events_processed"`
	EventsDropped     uint64  `json:"events_dropped"`
	QueueDepth        int     `json:"queue_depth"`
	QueueCapacity     int     `json:"queue_capacity"`
	QueueUsagePercent float64 `json:"queue_usage_percent"`
	ActiveWorkers     int     `json:"active_workers"`
	TotalWorkers      int     `json:"total_workers"`

	// Performance metrics
	EventsPerSecond float64 `json:"events_per_second"`
}

// NewServer creates a new metrics server
func NewServer(cfg *config.MetricsConfig, eslClient *esl.Client, workerPool *worker.Pool, logger *zap.Logger) *Server {
	return &Server{
		config:     cfg,
		logger:     logger.Named("metrics"),
		eslClient:  eslClient,
		workerPool: workerPool,
		startTime:  time.Now(),
	}
}

// Start starts the metrics HTTP server
func (s *Server) Start() error {
	if !s.config.Enabled {
		s.logger.Info("Metrics server disabled")
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc(s.config.Path, s.handleMetrics)
	mux.HandleFunc("/health", s.handleHealth)

	addr := fmt.Sprintf(":%d", s.config.Port)
	s.server = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("Starting metrics server",
		zap.String("address", addr),
		zap.String("metrics_path", s.config.Path))

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Metrics server error", zap.Error(err))
		}
	}()

	return nil
}

// Stop stops the metrics server gracefully
func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}

	s.logger.Info("Stopping metrics server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("Error shutting down metrics server", zap.Error(err))
		return err
	}

	s.logger.Info("Metrics server stopped")
	return nil
}

// handleMetrics handles the metrics endpoint
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := s.collectMetrics()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")

	if err := json.NewEncoder(w).Encode(metrics); err != nil {
		s.logger.Error("Failed to encode metrics", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	s.logger.Debug("Metrics served",
		zap.String("remote_addr", r.RemoteAddr),
		zap.String("user_agent", r.UserAgent()))
}

// handleHealth handles the health check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now(),
		"uptime":    time.Since(s.startTime).String(),
	}

	// Check ESL connection health
	if s.eslClient != nil {
		health["esl_connected"] = s.eslClient.IsConnected()
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
		s.logger.Error("Failed to encode health", zap.Error(err))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// collectMetrics collects all metrics from various sources
func (s *Server) collectMetrics() *MetricsData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	uptime := time.Since(s.startTime)
	metrics := &MetricsData{
		StartTime: s.startTime,
		Uptime:    uptime.String(),
	}

	// ESL metrics
	if s.eslClient != nil {
		eslStats := s.eslClient.GetStats()
		metrics.ESLConnected = s.eslClient.IsConnected()
		metrics.ESLConnectedAt = eslStats.ConnectedAt
		metrics.ESLEventsReceived = eslStats.EventsReceived
		metrics.ESLReconnects = eslStats.ReconnectAttempts
	}

	// Worker pool metrics
	if s.workerPool != nil {
		poolMetrics := s.workerPool.GetMetrics()

		metrics.EventsReceived = poolMetrics["events_received"].(uint64)
		metrics.EventsProcessed = poolMetrics["events_processed"].(uint64)
		metrics.EventsDropped = poolMetrics["events_dropped"].(uint64)
		metrics.QueueDepth = poolMetrics["queue_depth"].(int)
		metrics.QueueCapacity = poolMetrics["queue_capacity"].(int)
		metrics.QueueUsagePercent = poolMetrics["queue_usage_percent"].(float64)
		metrics.ActiveWorkers = poolMetrics["active_workers"].(int)
		metrics.TotalWorkers = poolMetrics["total_workers"].(int)

		// Calculate events per second
		if uptime.Seconds() > 0 {
			metrics.EventsPerSecond = float64(metrics.EventsProcessed) / uptime.Seconds()
		}
	}

	return metrics
}

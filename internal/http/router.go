package http

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"fsevents/internal/config"
)

// DestinationRouter handles URL selection based on strategy
type DestinationRouter struct {
	destination config.HTTPDestination

	// Round-robin state
	rrIndex atomic.Uint32

	// Random state
	rng *rand.Rand
	mu  sync.Mutex
}

// NewDestinationRouter creates a new router for a destination
func NewDestinationRouter(dest config.HTTPDestination) *DestinationRouter {
	return &DestinationRouter{
		destination: dest,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// SelectURL selects a URL based on the configured strategy
func (r *DestinationRouter) SelectURL() string {
	urls := r.destination.GetURLs()

	// Single URL - no routing needed
	if len(urls) == 1 {
		return urls[0]
	}

	strategy := r.destination.GetStrategy()

	switch strategy {
	case "round-robin":
		return r.selectRoundRobin(urls)
	case "random":
		return r.selectRandom(urls)
	case "failover":
		// For failover, return first URL (will try others on failure)
		return urls[0]
	default:
		return r.selectRoundRobin(urls)
	}
}

// GetAllURLs returns all URLs for broadcast strategy
func (r *DestinationRouter) GetAllURLs() []string {
	return r.destination.GetURLs()
}

// GetURLsForFailover returns URLs in order for failover attempts
func (r *DestinationRouter) GetURLsForFailover() []string {
	return r.destination.GetURLs()
}

// selectRoundRobin selects the next URL in round-robin fashion
func (r *DestinationRouter) selectRoundRobin(urls []string) string {
	if len(urls) == 0 {
		return ""
	}

	index := r.rrIndex.Add(1) - 1
	return urls[index%uint32(len(urls))]
}

// selectRandom selects a random URL from the list
func (r *DestinationRouter) selectRandom(urls []string) string {
	if len(urls) == 0 {
		return ""
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	index := r.rng.Intn(len(urls))
	return urls[index]
}

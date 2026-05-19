package limiter

import (
	"time"

	"wcg-ratelimiter/internal/server"
	"wcg-ratelimiter/internal/sim"
)

// Centralized is a single token bucket shared by every server that
// holds a reference to it. It models a Redis-style global rate
// limiter: all admission decisions consult one counter.
//
// Phase-1 simulation is single-threaded, so no locking is needed.
// In a real deployment this is the structure that becomes a
// network/coordination bottleneck.
type CentralizedConfig struct {
	Capacity   float64 // bucket size (max burst)
	RefillRate float64 // tokens per second
}

type Centralized struct {
	cfg        CentralizedConfig
	clock      *sim.Clock
	tokens     float64
	lastRefill time.Duration
}

func NewCentralized(clock *sim.Clock, cfg CentralizedConfig) *Centralized {
	return &Centralized{cfg: cfg, clock: clock, tokens: cfg.Capacity}
}

func (c *Centralized) refill(now time.Duration) {
	elapsed := (now - c.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	c.tokens += elapsed * c.cfg.RefillRate
	if c.tokens > c.cfg.Capacity {
		c.tokens = c.cfg.Capacity
	}
	c.lastRefill = now
}

func (c *Centralized) Admit(_ *server.Request, _ int) bool {
	c.refill(c.clock.Now())
	if c.tokens >= 1 {
		c.tokens--
		return true
	}
	return false
}

func (c *Centralized) OnComplete(_ *server.Request, _ time.Duration) {}

func (c *Centralized) Limit() int { return int(c.cfg.RefillRate) }

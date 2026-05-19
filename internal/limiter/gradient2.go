package limiter

import (
	"math"
	"time"

	"wcg-ratelimiter/internal/server"
)

// Gradient2 is a port of Netflix concurrency-limits' Gradient2Limit.
// It tracks two EMAs of latency: a long-window minimum (the "floor"
// under no load) and a short-window average (current). When the
// short-window value rises above the floor, a queue is forming and
// the limit shrinks; when the gradient is healthy the limit grows.
//
// Update rule, executed once every ~Limit completions:
//
//	gradient   = clamp(rtt_min / rtt_actual, MinGradient, 1.0)
//	queue_size = QueueSize(limit)                  // headroom
//	new_limit  = limit*gradient + queue_size
//
// Multiplicative decrease is applied on explicit timeouts.
type Gradient2Config struct {
	Initial         float64
	Min             float64
	Max             float64
	MinGradient     float64       // gradient floor (e.g. 0.5) — caps shrink speed
	LongSmoothing   float64       // EMA factor for rtt_min  (smaller = slower)
	ShortSmoothing  float64       // EMA factor for rtt_actual (larger = more reactive)
	QueueSize       func(int) int // headroom term, e.g. sqrt(limit)
	TimeoutLatency  time.Duration // beyond this, treat as overload signal
	TimeoutShrink   float64       // multiplicative decrease on timeout (e.g. 0.9)
}

func DefaultGradient2() Gradient2Config {
	return Gradient2Config{
		Initial:        10,
		Min:            1,
		Max:            1000,
		MinGradient:    0.5,
		LongSmoothing:  0.01,
		ShortSmoothing: 0.2,
		QueueSize:      func(limit int) int { return int(math.Ceil(math.Sqrt(float64(limit)))) },
		TimeoutLatency: 2 * time.Second,
		TimeoutShrink:  0.9,
	}
}

type Gradient2 struct {
	cfg            Gradient2Config
	limit          float64
	rttMinEMA      float64 // long-term latency floor (nanoseconds, float)
	rttActualEMA   float64 // short-term latency (nanoseconds, float)
	sinceLastTune  int
}

func NewGradient2(cfg Gradient2Config) *Gradient2 {
	if cfg.Initial <= 0 {
		cfg.Initial = 1
	}
	if cfg.QueueSize == nil {
		cfg.QueueSize = func(limit int) int { return int(math.Ceil(math.Sqrt(float64(limit)))) }
	}
	return &Gradient2{cfg: cfg, limit: cfg.Initial}
}

func (g *Gradient2) Admit(req *server.Request, inFlight int) bool {
	return inFlight < int(g.limit)
}

func (g *Gradient2) OnComplete(req *server.Request, latency time.Duration) {
	lat := float64(latency.Nanoseconds())

	if g.rttActualEMA == 0 {
		g.rttActualEMA = lat
	} else {
		g.rttActualEMA = g.cfg.ShortSmoothing*lat + (1-g.cfg.ShortSmoothing)*g.rttActualEMA
	}

	// rtt_min is the latency floor — minimum observed under no load.
	// It only ratchets down; an upward EMA would let the floor follow
	// the queue and collapse the gradient signal.
	if g.rttMinEMA == 0 || lat < g.rttMinEMA {
		g.rttMinEMA = lat
	}

	if latency > g.cfg.TimeoutLatency {
		g.limit = math.Max(g.cfg.Min, g.limit*g.cfg.TimeoutShrink)
		g.sinceLastTune = 0
		return
	}

	g.sinceLastTune++
	if g.sinceLastTune < int(g.limit) {
		return
	}
	g.sinceLastTune = 0

	gradient := g.rttMinEMA / g.rttActualEMA
	if gradient < g.cfg.MinGradient {
		gradient = g.cfg.MinGradient
	}
	if gradient > 1.0 {
		gradient = 1.0
	}
	queue := float64(g.cfg.QueueSize(int(g.limit)))
	newLimit := g.limit*gradient + queue

	if newLimit < g.cfg.Min {
		newLimit = g.cfg.Min
	}
	if newLimit > g.cfg.Max {
		newLimit = g.cfg.Max
	}
	g.limit = newLimit
}

func (g *Gradient2) Limit() int { return int(g.limit) }

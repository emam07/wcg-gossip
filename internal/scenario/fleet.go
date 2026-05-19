package scenario

import (
	"math/rand/v2"
	"time"

	"wcg-ratelimiter/internal/server"
	"wcg-ratelimiter/internal/sim"
)

// ServerSpec describes one node in a fleet.
type ServerSpec struct {
	ID       string
	Workers  int
	MaxQueue int
	// ServiceTime is the per-request service time generator. Scenarios
	// vary this across servers to model heterogeneous capacity, or
	// across time to model a sudden shock.
	ServiceTime func(now time.Duration, req *server.Request) time.Duration
}

// Fleet is a collection of simulated servers sharing one clock.
type Fleet struct {
	Clock   *sim.Clock
	Servers []*server.Server
}

// BuildFleet constructs servers from specs and wires the time-aware
// service-time function. Limiter wiring is left to the caller because
// it differs per scenario (centralized shares one bucket; WCG shares
// one gossip mesh; aimd/gradient2 are per-server).
func BuildFleet(clock *sim.Clock, specs []ServerSpec) *Fleet {
	f := &Fleet{Clock: clock}
	for _, sp := range specs {
		sp := sp
		srv := &server.Server{
			ID:       sp.ID,
			Clock:    clock,
			Workers:  sp.Workers,
			MaxQueue: sp.MaxQueue,
			ServiceTime: func(req *server.Request) time.Duration {
				return sp.ServiceTime(clock.Now(), req)
			},
		}
		f.Servers = append(f.Servers, srv)
	}
	return f
}

// DefaultServiceTime mirrors the single-server scenarios: 80ms +
// uniform(0,40ms) jitter. baseService is the floor; scale > 1
// stretches the entire distribution proportionally (used to model a
// slow server).
func DefaultServiceTime(rng *rand.Rand, baseService time.Duration, scale float64) func(time.Duration, *server.Request) time.Duration {
	return func(_ time.Duration, _ *server.Request) time.Duration {
		jitter := time.Duration(rng.Float64() * float64(40*time.Millisecond))
		return time.Duration(float64(baseService+jitter) * scale)
	}
}

// ShockServiceTime is like DefaultServiceTime but applies a
// multiplier to service time starting at shockAt. Models a sudden
// dependency slowdown.
func ShockServiceTime(rng *rand.Rand, baseService time.Duration, shockAt time.Duration, shockScale float64) func(time.Duration, *server.Request) time.Duration {
	return func(now time.Duration, _ *server.Request) time.Duration {
		jitter := time.Duration(rng.Float64() * float64(40*time.Millisecond))
		base := baseService + jitter
		if now >= shockAt {
			return time.Duration(float64(base) * shockScale)
		}
		return base
	}
}

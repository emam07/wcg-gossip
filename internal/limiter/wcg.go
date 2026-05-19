package limiter

import (
	"time"

	"wcg-ratelimiter/internal/fairness"
	"wcg-ratelimiter/internal/gossip"
	"wcg-ratelimiter/internal/server"
	"wcg-ratelimiter/internal/sim"
)

// WCG (Weighted Capacity Gossip) composes the three layers from the
// design doc:
//
//  1. A per-server adaptive local limiter (Gradient2) produces C_i.
//  2. A gossip node publishes C_i and aggregates a view of C_total.
//  3. A weighted fairness allocator hands out per-tenant tokens
//     proportional to C_i / C_total.
//
// Admission requires both: the tenant must have a token in its
// fairness bucket AND the server's in-flight count must be below
// C_i. The conjunction is the safety net — a tenant having budget
// does not matter if the server itself is overloaded.
type WCG struct {
	clock      *sim.Clock
	local      *Gradient2
	node       *gossip.Node
	allocator  *fairness.WeightedAllocator
	tickEvery  time.Duration
}

type WCGConfig struct {
	Clock     *sim.Clock
	Local     *Gradient2
	Node      *gossip.Node
	Allocator *fairness.WeightedAllocator
	TickEvery time.Duration // how often to republish C_i and reweight
}

func NewWCG(cfg WCGConfig) *WCG {
	if cfg.TickEvery <= 0 {
		cfg.TickEvery = 100 * time.Millisecond
	}
	w := &WCG{
		clock:     cfg.Clock,
		local:     cfg.Local,
		node:      cfg.Node,
		allocator: cfg.Allocator,
		tickEvery: cfg.TickEvery,
	}
	// Seed the allocator and gossip layer immediately so the first
	// request sees non-zero buckets.
	w.publishAndReweight()
	return w
}

// Start schedules the periodic publish + reweight tick until `until`.
// Called once per server at scenario startup.
func (w *WCG) Start(until time.Duration) {
	w.schedule(until)
}

func (w *WCG) schedule(until time.Duration) {
	w.clock.Schedule(w.tickEvery, func() {
		w.publishAndReweight()
		if w.clock.Now() < until {
			w.schedule(until)
		}
	})
}

func (w *WCG) publishAndReweight() {
	ci := w.local.Limit()
	w.node.Publish(ci)
	w.allocator.Reweight(ci, w.node.TotalCapacity())
}

func (w *WCG) Admit(req *server.Request, inFlight int) bool {
	if !w.allocator.Admit(req.TenantID) {
		return false
	}
	return w.local.Admit(req, inFlight)
}

func (w *WCG) OnComplete(req *server.Request, latency time.Duration) {
	w.local.OnComplete(req, latency)
}

func (w *WCG) Limit() int { return w.local.Limit() }

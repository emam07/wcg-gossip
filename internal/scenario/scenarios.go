package scenario

import (
	"fmt"
	"math/rand/v2"
	"time"

	"wcg-ratelimiter/internal/fairness"
	"wcg-ratelimiter/internal/gossip"
	"wcg-ratelimiter/internal/limiter"
	"wcg-ratelimiter/internal/metrics"
	"wcg-ratelimiter/internal/server"
	"wcg-ratelimiter/internal/sim"
	"wcg-ratelimiter/internal/workload"
)

// LimiterKind enumerates the limiters we compare in Phase-1 results.
type LimiterKind string

const (
	KindNone               LimiterKind = "none"
	KindGradient2Local     LimiterKind = "gradient2_local"     // per-server, no fairness
	KindPerTenantCentral   LimiterKind = "centralized_pertenant" // Redis with per-tenant buckets
	KindWCG                LimiterKind = "wcg"
)

// TenantSpec is one tenant's offered-load profile.
type TenantSpec struct {
	ID           string
	GlobalBudget float64 // RPS — what the operator wants to allocate
	OfferedRate  workload.RateAt
}

// Spec is a runnable scenario.
type Spec struct {
	Name     string
	Servers  []ServerSpec
	Tenants  []TenantSpec
	Duration time.Duration
	Sample   time.Duration

	// Gossip parameters (used only by WCG).
	GossipInterval time.Duration
	GossipDelay    time.Duration

	// Reweight tick interval inside each WCG limiter.
	WCGTickEvery time.Duration
}

// Result is the path of the CSV produced by a single run.
type Result struct {
	Scenario string
	Limiter  LimiterKind
	CSVPath  string
}

// Run executes a scenario under one LimiterKind and writes a CSV.
func Run(spec Spec, kind LimiterKind, outDir string, seed uint64) Result {
	clock := sim.NewClock()
	rng := rand.New(rand.NewPCG(seed, 0x9E3779B9))

	// Resolve service-time funcs (specs were already built by caller).
	fleet := BuildFleet(clock, spec.Servers)

	coll := metrics.NewFleetCollector(clock, spec.Sample, spec.Duration)

	// Build budgets map from tenants.
	budgets := make(map[string]float64, len(spec.Tenants))
	for _, t := range spec.Tenants {
		budgets[t.ID] = t.GlobalBudget
	}

	// Wire limiters per server. Some kinds share state (gossip mesh,
	// central bucket), some are independent — handled inline.
	var mesh *gossip.Mesh
	var sharedLim limiter.Limiter
	switch kind {
	case KindWCG:
		mesh = &gossip.Mesh{
			Clock:    clock,
			Interval: spec.GossipInterval,
			Delay:    spec.GossipDelay,
			Rng:      rng,
		}
	case KindPerTenantCentral:
		sharedLim = limiter.NewPerTenantCentralized(clock, limiter.PerTenantConfig{
			GlobalBudgets: budgets,
			BurstSeconds:  1,
		})
	}

	wcgInstances := make([]*limiter.WCG, 0, len(fleet.Servers))
	for _, srv := range fleet.Servers {
		srv := srv
		var lim limiter.Limiter
		switch kind {
		case KindNone:
			lim = nil
		case KindGradient2Local:
			lim = limiter.NewGradient2(limiter.DefaultGradient2())
		case KindPerTenantCentral:
			lim = sharedLim
		case KindWCG:
			grad := limiter.NewGradient2(limiter.DefaultGradient2())
			node := mesh.AddNode(srv.ID, grad.Limit())
			alloc := fairness.NewWeightedAllocator(clock, fairness.Config{
				GlobalBudgets: budgets,
				BurstSeconds:  1,
			})
			wcg := limiter.NewWCG(limiter.WCGConfig{
				Clock:     clock,
				Local:     grad,
				Node:      node,
				Allocator: alloc,
				TickEvery: spec.WCGTickEvery,
			})
			wcgInstances = append(wcgInstances, wcg)
			lim = wcg
		}
		if lim != nil {
			srv.Limiter = lim
			if l, ok := lim.(limiter.Limiter); ok {
				coll.LimitFns[srv.ID] = l.Limit
			}
		}

		// Hook metrics with tenant labels from the request.
		srvID := srv.ID
		srv.OnAccept = func(r *server.Request) { coll.OnAccept(srvID, r.TenantID) }
		srv.OnDone = func(r *server.Request, lat time.Duration) { coll.OnDone(srvID, r.TenantID, lat) }
		srv.OnReject = func(r *server.Request, reason string) { coll.OnReject(srvID, r.TenantID, reason) }
	}

	// Start gossip + WCG ticks after limiter wiring.
	if mesh != nil {
		mesh.Start(spec.Duration)
	}
	for _, w := range wcgInstances {
		w.Start(spec.Duration)
	}

	// Workload: one generator per tenant, each picking targets at
	// random across the fleet (uniform LB).
	var nextID int64
	for _, t := range spec.Tenants {
		t := t
		gen := &workload.PoissonGenerator{
			Clock:    clock,
			Rng:      rng,
			TenantID: t.ID,
			Rate:     t.OfferedRate,
			Targets:  fleet.Servers,
			NextID:   &nextID,
		}
		gen.Start(spec.Duration)
	}

	coll.Start()
	clock.Run(spec.Duration)

	path := fmt.Sprintf("%s/scenario_%s__%s.csv", outDir, spec.Name, kind)
	if err := coll.WriteCSV(path); err != nil {
		panic(err)
	}
	return Result{Scenario: spec.Name, Limiter: kind, CSVPath: path}
}

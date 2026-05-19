package fairness

import (
	"time"

	"wcg-ratelimiter/internal/sim"
)

// WeightedAllocator maintains one token bucket per tenant on a single
// server. Bucket refill rate is the tenant's local share of fleet
// capacity:
//
//	rate_{i,t} = G_t × (C_i / C_total)
//
// where G_t is the tenant's global RPS budget, C_i is this server's
// current safe concurrency (from Gradient2), and C_total is the
// fleet-wide sum of C_j as seen via gossip.
//
// Reweight() is called periodically (typically on each gossip tick)
// to refresh per-tenant rates as local/fleet capacity drifts.
type Config struct {
	GlobalBudgets map[string]float64 // tenant_id -> RPS
	BurstSeconds  float64            // bucket capacity = rate * BurstSeconds
}

type bucket struct {
	rate       float64
	cap        float64
	tokens     float64
	lastRefill time.Duration
}

type WeightedAllocator struct {
	cfg     Config
	clock   *sim.Clock
	tenants map[string]*bucket
}

func NewWeightedAllocator(clock *sim.Clock, cfg Config) *WeightedAllocator {
	if cfg.BurstSeconds <= 0 {
		cfg.BurstSeconds = 1
	}
	wa := &WeightedAllocator{cfg: cfg, clock: clock, tenants: make(map[string]*bucket)}
	for t := range cfg.GlobalBudgets {
		wa.tenants[t] = &bucket{}
	}
	return wa
}

// Reweight recomputes per-tenant refill rates given this server's
// current local capacity and the fleet total.
func (a *WeightedAllocator) Reweight(localCap, totalCap int) {
	if totalCap <= 0 {
		return
	}
	weight := float64(localCap) / float64(totalCap)
	for t, G := range a.cfg.GlobalBudgets {
		b, ok := a.tenants[t]
		if !ok {
			b = &bucket{}
			a.tenants[t] = b
		}
		b.rate = G * weight
		b.cap = b.rate * a.cfg.BurstSeconds
		if b.cap < 1 {
			b.cap = 1
		}
		if b.tokens > b.cap {
			b.tokens = b.cap
		}
	}
}

func (a *WeightedAllocator) Admit(tenantID string) bool {
	b, ok := a.tenants[tenantID]
	if !ok {
		// Unknown tenant: deny rather than implicit-allow. Tenants
		// must be configured up-front so accounting is exact.
		return false
	}
	now := a.clock.Now()
	if b.rate <= 0 {
		return false
	}
	elapsed := (now - b.lastRefill).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.cap {
			b.tokens = b.cap
		}
		b.lastRefill = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

func (a *WeightedAllocator) TenantRate(tenantID string) float64 {
	if b, ok := a.tenants[tenantID]; ok {
		return b.rate
	}
	return 0
}

package limiter

import (
	"time"

	"wcg-ratelimiter/internal/server"
	"wcg-ratelimiter/internal/sim"
)

// PerTenantCentralized is a single global allocator with one bucket
// per tenant — the realistic shape of production Redis-based limiters
// ("plan tier A gets 100 RPS, plan tier B gets 1000 RPS"). It is
// fair across tenants by construction but cannot react to local
// load: the rate G_t is set by config, not by observed capacity.
type PerTenantConfig struct {
	GlobalBudgets map[string]float64 // tenant_id -> RPS
	BurstSeconds  float64
}

type ptBucket struct {
	rate, cap, tokens float64
	lastRefill        time.Duration
}

type PerTenantCentralized struct {
	cfg     PerTenantConfig
	clock   *sim.Clock
	tenants map[string]*ptBucket
}

func NewPerTenantCentralized(clock *sim.Clock, cfg PerTenantConfig) *PerTenantCentralized {
	if cfg.BurstSeconds <= 0 {
		cfg.BurstSeconds = 1
	}
	pt := &PerTenantCentralized{cfg: cfg, clock: clock, tenants: make(map[string]*ptBucket)}
	for t, G := range cfg.GlobalBudgets {
		pt.tenants[t] = &ptBucket{
			rate:   G,
			cap:    G * cfg.BurstSeconds,
			tokens: G * cfg.BurstSeconds,
		}
	}
	return pt
}

func (p *PerTenantCentralized) Admit(req *server.Request, _ int) bool {
	b, ok := p.tenants[req.TenantID]
	if !ok {
		return false
	}
	now := p.clock.Now()
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

func (p *PerTenantCentralized) OnComplete(_ *server.Request, _ time.Duration) {}

func (p *PerTenantCentralized) Limit() int {
	var total float64
	for _, b := range p.tenants {
		total += b.rate
	}
	return int(total)
}

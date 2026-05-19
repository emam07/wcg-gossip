package scenario

import (
	"math/rand/v2"
	"time"

	"wcg-ratelimiter/internal/workload"
)

// Partition: 3 healthy servers. At t=20s the gossip mesh splits into
// {srv-a, srv-b} vs {srv-c}; at t=40s the partition heals. Both
// tenants offer steady load throughout. Measures whether each
// partition stabilises at a fair per-partition allocation while
// split, and how fast the fleet re-converges after heal.
func Partition(rng *rand.Rand) Spec {
	const splitAt = 20 * time.Second
	const healAt = 40 * time.Second
	return Spec{
		Name: "partition",
		Servers: []ServerSpec{
			{ID: "srv-a", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: DefaultServiceTime(rng, defaultBaseService, 1.0)},
			{ID: "srv-b", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: DefaultServiceTime(rng, defaultBaseService, 1.0)},
			{ID: "srv-c", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: DefaultServiceTime(rng, defaultBaseService, 1.0)},
		},
		Tenants: []TenantSpec{
			{ID: "tA", GlobalBudget: 30, OfferedRate: workload.ConstantRate(30)},
			{ID: "tB", GlobalBudget: 30, OfferedRate: workload.ConstantRate(30)},
		},
		Duration:       defaultDuration,
		Sample:         defaultSample,
		GossipInterval: 500 * time.Millisecond,
		GossipDelay:    50 * time.Millisecond,
		WCGTickEvery:   100 * time.Millisecond,
		Hooks: []Hook{
			{
				At: splitAt,
				Apply: func(ctx *HookContext) {
					if ctx.Mesh != nil {
						ctx.Mesh.SetPartition([][]string{{"srv-a", "srv-b"}, {"srv-c"}})
					}
				},
			},
			{
				At: healAt,
				Apply: func(ctx *HookContext) {
					if ctx.Mesh != nil {
						ctx.Mesh.HealPartition()
					}
				},
			},
		},
	}
}

// Named scenarios from design doc §5. Each returns a Spec that can be
// fed to Run() under any LimiterKind.

const (
	defaultWorkers     = 4
	defaultMaxQueue    = 200
	defaultBaseService = 100 * time.Millisecond
	defaultDuration    = 60 * time.Second
	defaultSample      = 1 * time.Second
)

// HeterogeneousCapacity: 3 servers, one is 3x slower (i.e. 1/3
// capacity). Two tenants with equal global budget, each offering
// load just under aggregate fleet capacity. Tests whether the
// limiter routes more of each tenant's budget through the healthy
// servers.
func HeterogeneousCapacity(rng *rand.Rand) Spec {
	return Spec{
		Name: "heterogeneous",
		Servers: []ServerSpec{
			{ID: "srv-a", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: DefaultServiceTime(rng, defaultBaseService, 1.0)},
			{ID: "srv-b", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: DefaultServiceTime(rng, defaultBaseService, 1.0)},
			{ID: "srv-c", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: DefaultServiceTime(rng, defaultBaseService, 3.0)},
		},
		Tenants: []TenantSpec{
			{ID: "tA", GlobalBudget: 40, OfferedRate: workload.ConstantRate(40)},
			{ID: "tB", GlobalBudget: 40, OfferedRate: workload.ConstantRate(40)},
		},
		Duration:       defaultDuration,
		Sample:         defaultSample,
		GossipInterval: 500 * time.Millisecond,
		GossipDelay:    50 * time.Millisecond,
		WCGTickEvery:   100 * time.Millisecond,
	}
}

// NoisyTenant: 3 healthy servers, two tenants. Tenant A's budget is
// 20 RPS but it offers 100 RPS (5x its share). Tenant B offers
// exactly its 20 RPS budget. A fair limiter must clamp A at 20 RPS
// and let B through unhindered. An unfair limiter lets A starve B
// of slots.
func NoisyTenant(rng *rand.Rand) Spec {
	return Spec{
		Name: "noisy_tenant",
		Servers: []ServerSpec{
			{ID: "srv-a", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: DefaultServiceTime(rng, defaultBaseService, 1.0)},
			{ID: "srv-b", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: DefaultServiceTime(rng, defaultBaseService, 1.0)},
			{ID: "srv-c", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: DefaultServiceTime(rng, defaultBaseService, 1.0)},
		},
		Tenants: []TenantSpec{
			{ID: "tA_noisy", GlobalBudget: 20, OfferedRate: workload.ConstantRate(100)},
			{ID: "tB_quiet", GlobalBudget: 20, OfferedRate: workload.ConstantRate(20)},
		},
		Duration:       defaultDuration,
		Sample:         defaultSample,
		GossipInterval: 500 * time.Millisecond,
		GossipDelay:    50 * time.Millisecond,
		WCGTickEvery:   100 * time.Millisecond,
	}
}

// Shock: 3 healthy servers; at t=20s one server's service time
// triples (e.g. a downstream slowdown affecting only that node).
// Measures how fast each limiter rebalances and the peak transient
// latency during the shock.
func Shock(rng *rand.Rand) Spec {
	const shockAt = 20 * time.Second
	const shockScale = 3.0
	return Spec{
		Name: "shock",
		Servers: []ServerSpec{
			{ID: "srv-a", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: DefaultServiceTime(rng, defaultBaseService, 1.0)},
			{ID: "srv-b", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: DefaultServiceTime(rng, defaultBaseService, 1.0)},
			{ID: "srv-c", Workers: defaultWorkers, MaxQueue: defaultMaxQueue,
				ServiceTime: ShockServiceTime(rng, defaultBaseService, shockAt, shockScale)},
		},
		Tenants: []TenantSpec{
			{ID: "tA", GlobalBudget: 30, OfferedRate: workload.ConstantRate(30)},
			{ID: "tB", GlobalBudget: 30, OfferedRate: workload.ConstantRate(30)},
		},
		Duration:       defaultDuration,
		Sample:         defaultSample,
		GossipInterval: 500 * time.Millisecond,
		GossipDelay:    50 * time.Millisecond,
		WCGTickEvery:   100 * time.Millisecond,
	}
}

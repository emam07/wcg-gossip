package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"time"

	"wcg-ratelimiter/internal/limiter"
	"wcg-ratelimiter/internal/metrics"
	"wcg-ratelimiter/internal/scenario"
	"wcg-ratelimiter/internal/server"
	"wcg-ratelimiter/internal/sim"
	"wcg-ratelimiter/internal/workload"
)

const (
	workers     = 4
	baseService = 100 * time.Millisecond
	maxQueue    = 200
	duration    = 60 * time.Second
	sampleEvery = 1 * time.Second
	seed        = uint64(42)
)

var loadSteps = []struct {
	After time.Duration
	Rate  float64
}{
	{0, 10},
	{15 * time.Second, 30},
	{30 * time.Second, 50},
	{45 * time.Second, 30},
}

type scenarioResult struct {
	name string
	path string
}

// runSingleServerScenario keeps the original single-server, single-tenant
// runs that prove each per-server algorithm works in isolation. Output
// CSV schema is the legacy one (one row per second, no tenant dim).
func runSingleServerScenario(name string, build func(c *sim.Clock) limiter.Limiter) scenarioResult {
	clock := sim.NewClock()
	rng := rand.New(rand.NewPCG(seed, 1024))

	srv := &server.Server{
		ID:       "srv-0",
		Clock:    clock,
		Workers:  workers,
		MaxQueue: maxQueue,
		ServiceTime: func(_ *server.Request) time.Duration {
			jitter := time.Duration(rng.Float64() * float64(40*time.Millisecond))
			return 80*time.Millisecond + jitter
		},
	}
	var lim limiter.Limiter
	if build != nil {
		lim = build(clock)
		srv.Limiter = lim
	}

	coll := &metrics.Collector{
		Clock:      clock,
		Interval:   sampleEvery,
		Until:      duration,
		InFlightFn: srv.InFlight,
	}
	if lim != nil {
		coll.LimitFn = lim.Limit
	}
	srv.OnAccept = func(_ *server.Request) { coll.OnAccept() }
	srv.OnDone = func(_ *server.Request, lat time.Duration) { coll.OnDone(lat) }
	srv.OnReject = func(_ *server.Request, reason string) { coll.OnReject(reason) }

	var nextID int64
	steps := make([]struct {
		After time.Duration
		Rate  float64
	}, len(loadSteps))
	copy(steps, loadSteps)
	gen := &workload.PoissonGenerator{
		Clock:    clock,
		Rng:      rng,
		TenantID: "t1",
		Rate:     workload.StepRate(steps),
		Targets:  []*server.Server{srv},
		NextID:   &nextID,
	}

	coll.Start()
	gen.Start(duration)
	clock.Run(duration)

	out := fmt.Sprintf("results/single_server_%s.csv", name)
	if err := coll.WriteCSV(out); err != nil {
		fmt.Fprintf(os.Stderr, "write csv: %v\n", err)
		os.Exit(1)
	}
	return scenarioResult{name: name, path: out}
}

func main() {
	if err := os.MkdirAll("results", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}

	// Single-server baselines (Phase-1a: prove each algorithm works
	// in isolation under a step-load workload).
	single := []scenarioResult{
		runSingleServerScenario("no_limiter", nil),
		runSingleServerScenario("aimd", func(_ *sim.Clock) limiter.Limiter {
			return limiter.NewAIMD(limiter.DefaultAIMD())
		}),
		runSingleServerScenario("gradient2", func(_ *sim.Clock) limiter.Limiter {
			return limiter.NewGradient2(limiter.DefaultGradient2())
		}),
		runSingleServerScenario("centralized", func(c *sim.Clock) limiter.Limiter {
			return limiter.NewCentralized(c, limiter.CentralizedConfig{
				Capacity: 60, RefillRate: 40,
			})
		}),
	}
	for _, r := range single {
		fmt.Printf("wrote %s\n", r.path)
	}

	// Multi-server multi-tenant fleet experiments (Phase-1b: the
	// actual WCG comparison from design doc §5).
	kinds := []scenario.LimiterKind{
		scenario.KindNone,
		scenario.KindPerTenantCentral,
		scenario.KindGradient2Local,
		scenario.KindWCG,
	}

	specs := []func(*rand.Rand) scenario.Spec{
		scenario.HeterogeneousCapacity,
		scenario.NoisyTenant,
		scenario.Shock,
	}

	for _, specBuilder := range specs {
		for _, kind := range kinds {
			// Fresh RNG per (scenario, limiter) so service-time and
			// arrival jitter are reproducible across limiter swaps.
			rng := rand.New(rand.NewPCG(seed, 0xDEADBEEF))
			spec := specBuilder(rng)
			res := scenario.Run(spec, kind, "results", seed)
			fmt.Printf("wrote %s\n", res.CSVPath)
		}
	}
}

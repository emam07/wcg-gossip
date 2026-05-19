package workload

import (
	"math"
	"math/rand/v2"
	"time"

	"wcg-ratelimiter/internal/server"
	"wcg-ratelimiter/internal/sim"
)

type RateAt func(t time.Duration) float64

type PoissonGenerator struct {
	Clock    *sim.Clock
	Rng      *rand.Rand
	TenantID string
	Rate     RateAt
	Targets  []*server.Server
	NextID   *int64
}

func (p *PoissonGenerator) Start(until time.Duration) {
	p.schedule(until)
}

func (p *PoissonGenerator) schedule(until time.Duration) {
	now := p.Clock.Now()
	rate := p.Rate(now)
	if rate <= 0 {
		p.Clock.Schedule(100*time.Millisecond, func() {
			if p.Clock.Now() < until {
				p.schedule(until)
			}
		})
		return
	}
	gap := -math.Log(1.0-p.Rng.Float64()) / rate
	delay := time.Duration(gap * float64(time.Second))
	p.Clock.Schedule(delay, func() {
		if p.Clock.Now() >= until {
			return
		}
		*p.NextID++
		req := &server.Request{ID: *p.NextID, TenantID: p.TenantID}
		target := p.Targets[p.Rng.IntN(len(p.Targets))]
		target.Receive(req)
		p.schedule(until)
	})
}

func ConstantRate(r float64) RateAt {
	return func(_ time.Duration) float64 { return r }
}

func StepRate(steps []struct {
	After time.Duration
	Rate  float64
}) RateAt {
	return func(t time.Duration) float64 {
		current := 0.0
		for _, s := range steps {
			if t >= s.After {
				current = s.Rate
			}
		}
		return current
	}
}

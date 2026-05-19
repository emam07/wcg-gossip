package metrics

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"time"

	"wcg-ratelimiter/internal/sim"
)

type Sample struct {
	TimeMs           int64
	Accepted         int
	RejectedLimiter  int
	RejectedOverload int
	InFlight         int
	Limit            int
	P50Ms            float64
	P99Ms            float64
}

type Collector struct {
	Clock    *sim.Clock
	Interval time.Duration
	Until    time.Duration

	InFlightFn func() int
	LimitFn    func() int

	accepted   int
	rejLimiter int
	rejOverld  int
	latencies  []time.Duration
	samples    []Sample
}

func (c *Collector) OnAccept()                          { c.accepted++ }
func (c *Collector) OnDone(latency time.Duration)       { c.latencies = append(c.latencies, latency) }
func (c *Collector) OnReject(reason string) {
	switch reason {
	case "limiter":
		c.rejLimiter++
	default:
		c.rejOverld++
	}
}

func (c *Collector) Start() {
	c.scheduleNext()
}

func (c *Collector) scheduleNext() {
	c.Clock.Schedule(c.Interval, func() {
		c.flush()
		if c.Clock.Now() < c.Until {
			c.scheduleNext()
		}
	})
}

func (c *Collector) flush() {
	s := Sample{
		TimeMs:           c.Clock.Now().Milliseconds(),
		Accepted:         c.accepted,
		RejectedLimiter:  c.rejLimiter,
		RejectedOverload: c.rejOverld,
	}
	if c.InFlightFn != nil {
		s.InFlight = c.InFlightFn()
	}
	if c.LimitFn != nil {
		s.Limit = c.LimitFn()
	}
	if len(c.latencies) > 0 {
		sorted := make([]time.Duration, len(c.latencies))
		copy(sorted, c.latencies)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		s.P50Ms = float64(sorted[len(sorted)*50/100].Milliseconds())
		p99idx := len(sorted) * 99 / 100
		if p99idx >= len(sorted) {
			p99idx = len(sorted) - 1
		}
		s.P99Ms = float64(sorted[p99idx].Milliseconds())
	}
	c.samples = append(c.samples, s)
	c.accepted = 0
	c.rejLimiter = 0
	c.rejOverld = 0
	c.latencies = c.latencies[:0]
}

func (c *Collector) WriteCSV(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	if err := w.Write([]string{"time_ms", "accepted", "rej_limiter", "rej_overload", "inflight", "limit", "p50_ms", "p99_ms"}); err != nil {
		return err
	}
	for _, s := range c.samples {
		row := []string{
			fmt.Sprintf("%d", s.TimeMs),
			fmt.Sprintf("%d", s.Accepted),
			fmt.Sprintf("%d", s.RejectedLimiter),
			fmt.Sprintf("%d", s.RejectedOverload),
			fmt.Sprintf("%d", s.InFlight),
			fmt.Sprintf("%d", s.Limit),
			fmt.Sprintf("%.1f", s.P50Ms),
			fmt.Sprintf("%.1f", s.P99Ms),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

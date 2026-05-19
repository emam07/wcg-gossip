package metrics

import (
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"time"

	"wcg-ratelimiter/internal/sim"
)

// FleetSample is one row of the per-tenant per-server CSV. The CSV is
// long-format; downstream pandas pivots it as needed.
type FleetSample struct {
	TimeMs           int64
	Server           string
	Tenant           string
	Accepted         int
	RejectedLimiter  int
	RejectedOverload int
	P50Ms            float64
	P99Ms            float64
	LocalLimit       int // C_i (snapshot at flush time)
}

type fleetKey struct {
	server, tenant string
}

type fleetBucket struct {
	accepted, rejLim, rejOvl int
	latencies                []time.Duration
}

type FleetCollector struct {
	Clock    *sim.Clock
	Interval time.Duration
	Until    time.Duration

	// LimitFns map a server ID to its current local-limit accessor.
	LimitFns map[string]func() int

	buckets map[fleetKey]*fleetBucket
	samples []FleetSample
}

func NewFleetCollector(clock *sim.Clock, interval, until time.Duration) *FleetCollector {
	return &FleetCollector{
		Clock:    clock,
		Interval: interval,
		Until:    until,
		LimitFns: make(map[string]func() int),
		buckets:  make(map[fleetKey]*fleetBucket),
	}
}

func (c *FleetCollector) get(server, tenant string) *fleetBucket {
	k := fleetKey{server, tenant}
	b, ok := c.buckets[k]
	if !ok {
		b = &fleetBucket{}
		c.buckets[k] = b
	}
	return b
}

func (c *FleetCollector) OnAccept(server, tenant string) {
	c.get(server, tenant).accepted++
}
func (c *FleetCollector) OnDone(server, tenant string, latency time.Duration) {
	b := c.get(server, tenant)
	b.latencies = append(b.latencies, latency)
}
func (c *FleetCollector) OnReject(server, tenant, reason string) {
	b := c.get(server, tenant)
	if reason == "limiter" {
		b.rejLim++
	} else {
		b.rejOvl++
	}
}

func (c *FleetCollector) Start() { c.scheduleNext() }

func (c *FleetCollector) scheduleNext() {
	c.Clock.Schedule(c.Interval, func() {
		c.flush()
		if c.Clock.Now() < c.Until {
			c.scheduleNext()
		}
	})
}

func (c *FleetCollector) flush() {
	t := c.Clock.Now().Milliseconds()
	for k, b := range c.buckets {
		s := FleetSample{
			TimeMs:           t,
			Server:           k.server,
			Tenant:           k.tenant,
			Accepted:         b.accepted,
			RejectedLimiter:  b.rejLim,
			RejectedOverload: b.rejOvl,
		}
		if fn := c.LimitFns[k.server]; fn != nil {
			s.LocalLimit = fn()
		}
		if len(b.latencies) > 0 {
			sorted := make([]time.Duration, len(b.latencies))
			copy(sorted, b.latencies)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
			s.P50Ms = float64(sorted[len(sorted)*50/100].Milliseconds())
			p99idx := len(sorted) * 99 / 100
			if p99idx >= len(sorted) {
				p99idx = len(sorted) - 1
			}
			s.P99Ms = float64(sorted[p99idx].Milliseconds())
		}
		c.samples = append(c.samples, s)
		b.accepted = 0
		b.rejLim = 0
		b.rejOvl = 0
		b.latencies = b.latencies[:0]
	}
}

func (c *FleetCollector) WriteCSV(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	defer w.Flush()
	header := []string{"time_ms", "server", "tenant", "accepted", "rej_limiter", "rej_overload", "p50_ms", "p99_ms", "local_limit"}
	if err := w.Write(header); err != nil {
		return err
	}
	sort.Slice(c.samples, func(i, j int) bool {
		if c.samples[i].TimeMs != c.samples[j].TimeMs {
			return c.samples[i].TimeMs < c.samples[j].TimeMs
		}
		if c.samples[i].Server != c.samples[j].Server {
			return c.samples[i].Server < c.samples[j].Server
		}
		return c.samples[i].Tenant < c.samples[j].Tenant
	})
	for _, s := range c.samples {
		row := []string{
			fmt.Sprintf("%d", s.TimeMs),
			s.Server,
			s.Tenant,
			fmt.Sprintf("%d", s.Accepted),
			fmt.Sprintf("%d", s.RejectedLimiter),
			fmt.Sprintf("%d", s.RejectedOverload),
			fmt.Sprintf("%.1f", s.P50Ms),
			fmt.Sprintf("%.1f", s.P99Ms),
			fmt.Sprintf("%d", s.LocalLimit),
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	return nil
}

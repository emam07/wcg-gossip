package limiter

import (
	"time"

	"wcg-ratelimiter/internal/server"
)

type AIMDConfig struct {
	Initial        int
	Min            int
	Max            int
	TargetLatency  time.Duration
	IncreaseEvery  int
	DecreaseFactor float64
}

func DefaultAIMD() AIMDConfig {
	return AIMDConfig{
		Initial:        10,
		Min:            1,
		Max:            1000,
		TargetLatency:  800 * time.Millisecond,
		IncreaseEvery:  5,
		DecreaseFactor: 0.5,
	}
}

type AIMD struct {
	cfg       AIMDConfig
	limit     int
	successes int
}

func NewAIMD(cfg AIMDConfig) *AIMD {
	if cfg.Initial <= 0 {
		cfg.Initial = 1
	}
	return &AIMD{cfg: cfg, limit: cfg.Initial}
}

func (a *AIMD) Admit(req *server.Request, inFlight int) bool {
	return inFlight < a.limit
}

func (a *AIMD) OnComplete(req *server.Request, latency time.Duration) {
	if latency > a.cfg.TargetLatency {
		newLimit := int(float64(a.limit) * a.cfg.DecreaseFactor)
		if newLimit < a.cfg.Min {
			newLimit = a.cfg.Min
		}
		a.limit = newLimit
		a.successes = 0
		return
	}
	a.successes++
	if a.successes >= a.cfg.IncreaseEvery {
		if a.limit < a.cfg.Max {
			a.limit++
		}
		a.successes = 0
	}
}

func (a *AIMD) Limit() int { return a.limit }

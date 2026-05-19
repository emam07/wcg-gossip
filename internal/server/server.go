package server

import (
	"time"

	"wcg-ratelimiter/internal/sim"
)

type Request struct {
	ID         int64
	TenantID   string
	ArrivedAt  time.Duration
	StartedAt  time.Duration
	FinishedAt time.Duration
}

type Limiter interface {
	Admit(req *Request, inFlight int) bool
	OnComplete(req *Request, latency time.Duration)
}

type ServiceTime func(req *Request) time.Duration

type Server struct {
	ID          string
	Clock       *sim.Clock
	Limiter     Limiter
	ServiceTime ServiceTime
	Workers     int
	MaxQueue    int

	OnAccept func(*Request)
	OnReject func(*Request, string)
	OnDone   func(*Request, time.Duration)

	busy  int
	queue []*Request
}

func (s *Server) InFlight() int { return s.busy + len(s.queue) }

func (s *Server) Receive(req *Request) {
	req.ArrivedAt = s.Clock.Now()
	if s.Limiter != nil && !s.Limiter.Admit(req, s.InFlight()) {
		s.reject(req, "limiter")
		return
	}
	if s.busy >= s.Workers {
		if len(s.queue) >= s.MaxQueue {
			s.reject(req, "queue_full")
			return
		}
		s.queue = append(s.queue, req)
		return
	}
	s.start(req)
}

func (s *Server) reject(req *Request, reason string) {
	if s.OnReject != nil {
		s.OnReject(req, reason)
	}
}

func (s *Server) start(req *Request) {
	s.busy++
	req.StartedAt = s.Clock.Now()
	if s.OnAccept != nil {
		s.OnAccept(req)
	}
	dur := s.ServiceTime(req)
	s.Clock.Schedule(dur, func() {
		req.FinishedAt = s.Clock.Now()
		s.busy--
		latency := req.FinishedAt - req.ArrivedAt
		if s.Limiter != nil {
			s.Limiter.OnComplete(req, latency)
		}
		if s.OnDone != nil {
			s.OnDone(req, latency)
		}
		if len(s.queue) > 0 {
			next := s.queue[0]
			s.queue = s.queue[1:]
			s.start(next)
		}
	})
}

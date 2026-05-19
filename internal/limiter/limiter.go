package limiter

import "wcg-ratelimiter/internal/server"

type Limiter interface {
	server.Limiter
	Limit() int
}

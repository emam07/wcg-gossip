package sim

import (
	"container/heap"
	"time"
)

type Event struct {
	At  time.Duration
	Run func()
	seq int64
}

type eventHeap []*Event

func (h eventHeap) Len() int { return len(h) }
func (h eventHeap) Less(i, j int) bool {
	if h[i].At != h[j].At {
		return h[i].At < h[j].At
	}
	return h[i].seq < h[j].seq
}
func (h eventHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *eventHeap) Push(x any)   { *h = append(*h, x.(*Event)) }
func (h *eventHeap) Pop() any {
	old := *h
	n := len(old)
	e := old[n-1]
	*h = old[:n-1]
	return e
}

type Clock struct {
	now    time.Duration
	events *eventHeap
	seq    int64
}

func NewClock() *Clock {
	h := &eventHeap{}
	heap.Init(h)
	return &Clock{events: h}
}

func (c *Clock) Now() time.Duration { return c.now }

func (c *Clock) Schedule(after time.Duration, run func()) {
	c.seq++
	heap.Push(c.events, &Event{At: c.now + after, Run: run, seq: c.seq})
}

func (c *Clock) Run(until time.Duration) {
	for c.events.Len() > 0 {
		next := (*c.events)[0]
		if next.At > until {
			break
		}
		heap.Pop(c.events)
		c.now = next.At
		next.Run()
	}
	if c.now < until {
		c.now = until
	}
}

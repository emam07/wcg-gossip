package gossip

import (
	"math/rand/v2"
	"time"

	"wcg-ratelimiter/internal/sim"
)

// Mesh is an in-memory gossip layer used in Phase-1 simulation.
// Every Interval each Node broadcasts its current capacity to every
// peer. Delivery is scheduled on the simulator clock with Delay,
// and individual messages may be dropped with LossProb.
//
// This is intentionally simpler than SWIM/HyParView: the goal is
// to model the propagation properties (delay, loss, view staleness)
// that the WCG limiter has to tolerate, not to benchmark a real
// gossip protocol. Phase-2 swaps this for hashicorp/memberlist
// without touching the limiter.
type Mesh struct {
	Clock    *sim.Clock
	Interval time.Duration
	Delay    time.Duration
	LossProb float64
	Rng      *rand.Rand

	nodes []*Node
}

type Node struct {
	ID    string
	mesh  *Mesh
	self  int            // own current capacity (last published)
	peers map[string]int // last-known peer capacities
}

func (m *Mesh) AddNode(id string, initialCap int) *Node {
	n := &Node{
		ID:    id,
		mesh:  m,
		self:  initialCap,
		peers: make(map[string]int),
	}
	m.nodes = append(m.nodes, n)
	return n
}

// Start schedules periodic broadcasts for every node until `until`.
func (m *Mesh) Start(until time.Duration) {
	for _, n := range m.nodes {
		m.scheduleBroadcast(n, until)
	}
}

func (m *Mesh) scheduleBroadcast(n *Node, until time.Duration) {
	m.Clock.Schedule(m.Interval, func() {
		n.broadcast()
		if m.Clock.Now() < until {
			m.scheduleBroadcast(n, until)
		}
	})
}

func (n *Node) Publish(cap int) { n.self = cap }

// TotalCapacity is the node's best estimate of fleet capacity:
// own self value plus its last-known view of every peer.
func (n *Node) TotalCapacity() int {
	total := n.self
	for _, c := range n.peers {
		total += c
	}
	return total
}

func (n *Node) SelfCapacity() int { return n.self }

func (n *Node) PeerView() map[string]int {
	out := make(map[string]int, len(n.peers))
	for k, v := range n.peers {
		out[k] = v
	}
	return out
}

func (n *Node) broadcast() {
	for _, peer := range n.mesh.nodes {
		if peer.ID == n.ID {
			continue
		}
		if n.mesh.LossProb > 0 && n.mesh.Rng != nil && n.mesh.Rng.Float64() < n.mesh.LossProb {
			continue
		}
		from := n.ID
		cap := n.self
		dest := peer
		n.mesh.Clock.Schedule(n.mesh.Delay, func() {
			dest.peers[from] = cap
		})
	}
}

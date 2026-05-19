# Weighted Capacity Gossip (WCG): A Rate-Limiting Algorithm for Fleet-Local Adaptivity with Global Fairness

## 1. Problem

Rate limiters in distributed systems must satisfy two requirements that pull in opposite directions:

- **Global fairness across tenants.** Across a fleet of `N` servers serving `M` tenants, each tenant `t` should receive at most its configured share `G_t` of total capacity, regardless of which servers it lands on.
- **Local load awareness.** Each individual server's safe capacity fluctuates moment-to-moment (GC pauses, noisy neighbors, slow downstream dependencies, cold caches after a deploy). Static limits are always wrong — either wasting capacity or causing cascading failure.

Existing algorithms pick one side:

| Algorithm | Fair? | Load-aware? | Bottleneck? |
|---|---|---|---|
| Centralized token bucket (Redis) | yes | no | yes (the central store) |
| Sharded counters | partial | no | hot shards |
| Pure local adaptive (Netflix concurrency-limits) | no | yes | none |
| Client-side adaptive throttling (Google) | partial | indirect | requires cooperative clients |

There is no widely-deployed algorithm that delivers both at fleet scale without a coordination bottleneck.

## 2. Insight

**Local adaptive measurements can produce the weights that drive global fairness.**

If each server already knows its own real-time safe capacity `C_i` (via TCP-Vegas-style measurement), then a server that is healthy today should automatically receive a larger share of every tenant's global budget, and a degraded server should receive less. The local adaptive signal becomes the input to the fairness math — no central coordinator required, no per-request RPC.

## 3. Algorithm

### 3.1 Components

1. **Local adaptive limiter** (per server) — Gradient2 or AIMD on latency + queue depth produces `C_i`, this server's estimated safe concurrency. Updated every ~100 ms.
2. **Capacity gossip** — each server publishes `C_i` every ~500 ms via SWIM/HyParView. Every server maintains a view of all peers and computes `C_total = Σ C_j`.
3. **Per-tenant local allocation** — for tenant `t` with global budget `G_t`, this server's local allocation is:

       L_{i,t} = G_t × (C_i / C_total)

4. **Admission decision** (per request) — accept iff:

       (tenant_t has a token in its local bucket of size L_{i,t})
       AND
       (server's overall in-flight count < C_i)

   The conjunction is the safety net: tenant budget does not matter if the server itself is overloaded.

### 3.2 Properties

- **No central bottleneck.** Gossip is decentralized; the per-server view is updated asynchronously.
- **Self-healing.** A degraded server's `C_i` shrinks; its share of every tenant's budget shrinks; traffic naturally migrates to healthier nodes (assuming a load balancer that retries or routes to less-loaded peers).
- **Composable.** The local-limiter and gossip layers are pluggable. Gradient2 can be swapped for CoDel; in-memory gossip can be swapped for `hashicorp/memberlist`.
- **Tunable.** Gossip interval is the single knob trading coordination overhead vs. fairness precision.

## 4. Known Weaknesses

- **Gossip lag.** A sudden capacity drop on server `i` is not immediately visible to peers, so peers may over-direct traffic for one gossip cycle. *Mitigation:* the local limiter at `i` still hard-stops admission at `C_i`, so over-direction translates into rejections, not overload.
- **Bad routing.** If the load balancer concentrates one tenant's traffic on one server, fleet-wide fairness math does not help. *Assumption:* LB is round-robin or least-loaded.
- **Cold start.** A new server needs an initial `C_i`. *Mitigation:* conservative bootstrap value, fast ramp-up.
- **Network partition.** Gossip views diverge. *Mitigation:* each partition operates as its own mini-fleet — graceful degradation, not failure.
- **Tenant skew within a server.** If a tenant's local bucket `L_{i,t}` is small but their traffic is bursty, they will see frequent rejections even when fleet-wide capacity exists. *Future work:* short-window borrowing from a global pool.

## 5. Experimental Questions

Phase-1 simulation must answer:

1. **Goodput under heterogeneous capacity.** When servers have unequal `C_i` (e.g., one is degraded), does WCG achieve higher fleet goodput than centralized token bucket?
2. **Fairness under tenant skew.** When one tenant generates outsized load, does WCG maintain higher Jain's fairness index than pure-local adaptive?
3. **Gossip-interval tradeoff.** How does fairness precision and over-admission rate change as gossip interval varies from 100 ms to 5 s?
4. **Behavior under shocks.** When a server's capacity drops 80% instantaneously, how long until fleet allocation re-converges? What is peak p99 latency during the transient?
5. **Behavior under partition.** When the gossip mesh splits, does each partition stabilize at a per-partition fair allocation?

## 6. Baselines

- **Centralized token bucket** — simulated global atomic counter. Represents Redis-style limiters.
- **Pure local adaptive** — each server runs Gradient2 with no fairness layer. Represents Netflix concurrency-limits as deployed without coordination.
- **Sharded counter** — each tenant pinned to one shard. Represents naive horizontal scaling of centralized.

## 7. Metrics

- **Goodput** — successful requests per second across fleet.
- **Jain's fairness index** — across tenants, computed on accepted-request rate.
- **p99 latency** — both per server and fleet-wide.
- **Reject rate per tenant** — to detect tenant-level unfairness.
- **Convergence time** — wall-clock from a shock until fleet allocation is within 5% of new steady state.
- **Gossip bandwidth** — bytes/sec/node, to confirm overhead stays bounded.

## 8. Out of Scope (Phase 1)

- Real gossip transport (use in-memory simulated mesh).
- Real network — single-process discrete-event simulation.
- Cost-based limiting (per-request weights). Future work.
- Client-side cooperation. Future work.

## 9. References

- Netflix concurrency-limits: https://github.com/Netflix/concurrency-limits
- Envoy adaptive concurrency filter (Gradient algorithm).
- Google SRE Book, Chapter 21: Handling Overload (client-side adaptive throttling).
- Jain, R. K., Chiu, D. W., Hawe, W. R. (1984). *A Quantitative Measure of Fairness and Discrimination for Resource Allocation in Shared Computer Systems.*
- Brakmo, L. S., Peterson, L. L. (1995). *TCP Vegas: End-to-end congestion avoidance on a global Internet.*
- Nichols, K., Jacobson, V. (2012). *Controlling Queue Delay (CoDel).*

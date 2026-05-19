"""Render Q3 (gossip-interval sweep) and Q5 (partition) figures."""

from pathlib import Path
import pandas as pd
import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

RESULTS = Path(__file__).resolve().parent.parent / "results"


def main():
    # ---- Fig 5: Gossip-interval sweep ----
    noisy = pd.read_csv(RESULTS / "summary_gossipsweep_noisy.csv")
    shock = pd.read_csv(RESULTS / "summary_gossipsweep_shock.csv")

    fig, axes = plt.subplots(1, 2, figsize=(12, 4))
    ax = axes[0]
    ax.plot(noisy["interval_ms"], noisy["jain"], "o-", label="noisy_tenant", color="#1f77b4")
    ax.plot(shock["interval_ms"], shock["jain"], "s-", label="shock", color="#d62728")
    ax.set_xlabel("gossip interval (ms, log scale)")
    ax.set_ylabel("Jain's fairness index")
    ax.set_xscale("log")
    ax.set_ylim(0.99, 1.001)
    ax.grid(alpha=0.3)
    ax.legend()
    ax.set_title("Fairness vs gossip interval")

    ax = axes[1]
    ax.plot(noisy["interval_ms"], noisy["p99_mean_ms"], "o-", label="noisy_tenant", color="#1f77b4")
    ax.plot(shock["interval_ms"], shock["p99_mean_ms"], "s-", label="shock", color="#d62728")
    ax.set_xlabel("gossip interval (ms, log scale)")
    ax.set_ylabel("mean p99 latency (ms)")
    ax.set_xscale("log")
    ax.grid(alpha=0.3)
    ax.legend()
    ax.set_title("Latency vs gossip interval")

    fig.suptitle("Q3: Gossip-interval sweep (100 ms to 5 s)")
    fig.tight_layout()
    fig.savefig(RESULTS / "fig5_gossip_interval_sweep.png", dpi=120)
    plt.close(fig)

    # ---- Fig 6: Partition timeline ----
    def fleet_goodput(scenario, kind):
        df = pd.read_csv(RESULTS / f"scenario_{scenario}__{kind}.csv")
        return df.groupby("time_ms")["accepted"].sum()

    fig, ax = plt.subplots(figsize=(10, 4))
    colors = {
        "none": "#888888",
        "centralized_pertenant": "#1f77b4",
        "gradient2_local": "#2ca02c",
        "wcg": "#d62728",
    }
    for kind, color in colors.items():
        g = fleet_goodput("partition", kind)
        ax.plot(g.index / 1000, g.values, label=kind, color=color, lw=1.5)
    ax.axvspan(20, 40, alpha=0.15, color="red", label="partition window")
    ax.set_xlabel("time (s)")
    ax.set_ylabel("fleet goodput (accepted/s)")
    ax.set_title("Q5: Partition scenario — fleet goodput over time")
    ax.legend(loc="lower right", fontsize=8)
    ax.grid(alpha=0.3)
    fig.tight_layout()
    fig.savefig(RESULTS / "fig6_partition_timeline.png", dpi=120)
    plt.close(fig)

    print("wrote fig5_gossip_interval_sweep.png + fig6_partition_timeline.png")


if __name__ == "__main__":
    main()

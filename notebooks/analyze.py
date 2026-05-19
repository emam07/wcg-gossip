"""
Aggregate fleet-scenario CSVs into summary tables used by docs/results.md.

Computes for each (scenario, limiter):
  - per-tenant accepted RPS (mean over warm window)
  - Jain's fairness index across tenants
  - fleet p99 latency (max sample) and mean p99
  - per-server local-limit trajectory min/max

The warm window skips the first 5 seconds so token buckets and Gradient2
EMAs have time to settle.
"""

from __future__ import annotations

import glob
import os
import sys
from pathlib import Path

import pandas as pd

RESULTS_DIR = Path(__file__).resolve().parent.parent / "results"
WARM_MS = 5_000


def jains_index(values: list[float]) -> float:
    vs = [v for v in values if v >= 0]
    n = len(vs)
    if n == 0:
        return float("nan")
    s = sum(vs)
    sq = sum(v * v for v in vs)
    if sq == 0:
        return float("nan")
    return (s * s) / (n * sq)


def load_fleet(path: Path) -> pd.DataFrame:
    df = pd.read_csv(path)
    return df[df["time_ms"] >= WARM_MS]


def summarize_fleet(path: Path) -> dict:
    df = load_fleet(path)
    if df.empty:
        return {}
    duration_s = (df["time_ms"].max() - df["time_ms"].min()) / 1000 + 1
    per_tenant_total = df.groupby("tenant")["accepted"].sum()
    per_tenant_rps = (per_tenant_total / duration_s).to_dict()
    jain = jains_index(list(per_tenant_rps.values()))

    p99_series = df["p99_ms"]
    return {
        "per_tenant_rps": per_tenant_rps,
        "jains_index": jain,
        "p99_max_ms": float(p99_series.max()),
        "p99_mean_ms": float(p99_series.mean()),
        "fleet_goodput_rps": float(df["accepted"].sum() / duration_s),
        "rejects_limiter": int(df["rej_limiter"].sum()),
        "rejects_overload": int(df["rej_overload"].sum()),
    }


def summarize_sweep(prefix: str) -> pd.DataFrame:
    """Aggregate gossipsweep_<prefix>_<N>ms__wcg.csv files into per-interval rows."""
    rows = []
    for path in sorted(RESULTS_DIR.glob(f"scenario_gossipsweep_{prefix}_*ms__wcg.csv")):
        # e.g. gossipsweep_noisy_500ms
        stem = path.stem[len("scenario_") :]
        # strip __wcg suffix
        name = stem[: stem.index("__")]
        ms = int(name.rsplit("_", 1)[1].rstrip("ms"))
        s = summarize_fleet(path)
        rows.append({
            "interval_ms": ms,
            "jain": s.get("jains_index", float("nan")),
            "fleet_rps": s.get("fleet_goodput_rps", 0),
            "p99_max_ms": s.get("p99_max_ms", 0),
            "p99_mean_ms": s.get("p99_mean_ms", 0),
            **{f"rps[{k}]": v for k, v in s.get("per_tenant_rps", {}).items()},
        })
    return pd.DataFrame(rows).sort_values("interval_ms").reset_index(drop=True)


def main():
    rows = []
    for path in sorted(RESULTS_DIR.glob("scenario_*__*.csv")):
        # Skip gossip-sweep CSVs — they get their own summary.
        if path.stem.startswith("scenario_gossipsweep_"):
            continue
        stem = path.stem  # scenario_<name>__<limiter>
        body = stem[len("scenario_") :]
        name, _, kind = body.partition("__")
        s = summarize_fleet(path)
        rows.append({
            "scenario": name,
            "limiter": kind,
            **{f"rps[{k}]": round(v, 1) for k, v in s.get("per_tenant_rps", {}).items()},
            "jain": round(s.get("jains_index", float("nan")), 3),
            "fleet_rps": round(s.get("fleet_goodput_rps", 0), 1),
            "p99_max_ms": round(s.get("p99_max_ms", 0), 0),
            "p99_mean_ms": round(s.get("p99_mean_ms", 0), 0),
            "rej_lim": s.get("rejects_limiter", 0),
            "rej_ovl": s.get("rejects_overload", 0),
        })
    summary = pd.DataFrame(rows)
    print("=== Main scenarios ===")
    print(summary.to_string(index=False))
    summary.to_csv(RESULTS_DIR / "summary.csv", index=False)

    for prefix in ("noisy", "shock"):
        sweep = summarize_sweep(prefix)
        print(f"\n=== Gossip-interval sweep ({prefix}) ===")
        print(sweep.to_string(index=False))
        sweep.to_csv(RESULTS_DIR / f"summary_gossipsweep_{prefix}.csv", index=False)

    print(f"\nwrote {RESULTS_DIR / 'summary.csv'}")


if __name__ == "__main__":
    main()

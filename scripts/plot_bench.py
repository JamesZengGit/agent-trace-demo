#!/usr/bin/env python3
"""Render the bench sweep: storage throughput vs offered rate, and tail
latency, as two stacked panels sharing one x axis (never a dual axis)."""
import csv
import sys

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

BLUE, RED, INK, MUTED, GRID = "#1a73e8", "#d93025", "#202124", "#5f6368", "#e8eaed"

def main(csv_path: str, out_svg: str) -> None:
    rows = list(csv.DictReader(open(csv_path)))
    offered = [float(r["offered_rps"]) for r in rows]
    stored = [float(r["stored_rps"]) for r in rows]
    p95 = [float(r["p95_ms"]) for r in rows]

    fig, (ax1, ax2) = plt.subplots(
        2, 1, figsize=(8, 5.4), sharex=True,
        gridspec_kw={"height_ratios": [3, 2], "hspace": 0.12})
    fig.patch.set_facecolor("white")

    for ax in (ax1, ax2):
        ax.set_facecolor("white")
        ax.grid(axis="y", color=GRID, linewidth=0.8)
        for side in ("top", "right"):
            ax.spines[side].set_visible(False)
        for side in ("left", "bottom"):
            ax.spines[side].set_color(GRID)
        ax.tick_params(colors=MUTED, labelsize=9)

    ax1.plot(offered, offered, color=MUTED, linewidth=1.2, linestyle="--",
             label="offered (ideal)")
    ax1.plot(offered, stored, color=BLUE, linewidth=2, marker="o",
             markersize=5, label="pipeline rate incl. drain (zero loss)")
    ax1.annotate(f"{stored[-1]:.0f}/s", (offered[-1], stored[-1]),
                 textcoords="offset points", xytext=(-6, 10),
                 color=INK, fontsize=9, ha="right")
    ax1.set_ylabel("spans/sec to Postgres", color=INK, fontsize=10)
    ax1.legend(frameon=False, fontsize=9, labelcolor=INK, loc="upper left")
    ax1.set_title("Capture-path throughput — loadgen → proxy → collector → NATS → processor → Postgres",
                  color=INK, fontsize=11, loc="left", pad=12)

    ax2.plot(offered, p95, color=BLUE, linewidth=2, marker="o", markersize=5)
    ax2.set_ylabel("p95 round-trip (ms)", color=INK, fontsize=10)
    ax2.set_xlabel("offered load (requests/sec)", color=INK, fontsize=10)

    fig.savefig(out_svg, bbox_inches="tight")
    print(f"wrote {out_svg}")

if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])

#!/usr/bin/env python3
"""Render the chaos recovery timeline: spans reaching storage over time while
the collector is killed and restarted, with reattachment off vs on."""
import csv
import json
import sys

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt

BLUE, RED, INK, MUTED, GRID = "#1a73e8", "#d93025", "#202124", "#5f6368", "#e8eaed"

def series(path: str):
    rows = list(csv.DictReader(open(path)))
    return ([int(r["second"]) for r in rows], [int(r["stored"]) for r in rows])

def main(out_dir: str, out_svg: str, kill_at: int, restart_at: int) -> None:
    t_off, s_off = series(f"{out_dir}/timeline-off.csv")
    t_on, s_on = series(f"{out_dir}/timeline-on.csv")
    sent_on = json.load(open(f"{out_dir}/loadgen-on.json"))["sent"]
    summary = {r["mode"]: r for r in csv.DictReader(open(f"{out_dir}/summary.csv"))}

    fig, ax = plt.subplots(figsize=(8, 4.4))
    fig.patch.set_facecolor("white")
    ax.set_facecolor("white")
    ax.grid(axis="y", color=GRID, linewidth=0.8)
    for side in ("top", "right"):
        ax.spines[side].set_visible(False)
    for side in ("left", "bottom"):
        ax.spines[side].set_color(GRID)
    ax.tick_params(colors=MUTED, labelsize=9)

    ax.axvspan(kill_at, restart_at, color=GRID, alpha=0.6, zorder=0)
    ax.text((kill_at + restart_at) / 2, ax.get_ylim()[1], "collector down",
            ha="center", va="bottom", fontsize=8, color=MUTED)

    ax.plot(t_on, s_on, color=BLUE, linewidth=2, label="reattach ON (fix)")
    ax.plot(t_off, s_off, color=RED, linewidth=2, label="reattach OFF (original bug)")
    ax.axhline(sent_on, color=MUTED, linewidth=1.2, linestyle="--")
    ax.text(t_on[-1], sent_on, " sent", va="center", fontsize=8, color=MUTED)

    off_pct = summary["off"]["orphaned_pct"]
    on_lost = max(0.0, float(summary["on"]["orphaned_pct"]))
    on_label = "0% lost — replay complete" if on_lost == 0 else f"{on_lost}% orphaned"
    ax.annotate(f"{off_pct}% orphaned", (t_off[-1], s_off[-1]),
                textcoords="offset points", xytext=(-4, -14),
                color=RED, fontsize=9, ha="right")
    ax.annotate(on_label, (t_on[-1], s_on[-1]),
                textcoords="offset points", xytext=(-4, 8),
                color=BLUE, fontsize=9, ha="right")

    ax.set_xlabel("seconds", color=INK, fontsize=10)
    ax.set_ylabel("spans in storage (cumulative)", color=INK, fontsize=10)
    ax.set_title("Orphan incident reproduced — collector killed under load",
                 color=INK, fontsize=11, loc="left", pad=12)
    ax.legend(frameon=False, fontsize=9, labelcolor=INK, loc="lower right")

    fig.savefig(out_svg, bbox_inches="tight")
    print(f"wrote {out_svg}")

if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2], int(sys.argv[3]), int(sys.argv[4]))

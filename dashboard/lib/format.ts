// Display formatting helpers. Times in local time HH:MM:SS; durations humanized.

/** Local wall-clock time, HH:MM:SS. */
export function fmtTime(iso: string | Date | undefined): string {
  if (!iso) return '';
  const d = typeof iso === 'string' ? new Date(iso) : iso;
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

/** Local date + time for header tiles. */
export function fmtDateTime(iso: string | Date | undefined): string {
  if (!iso) return '';
  const d = typeof iso === 'string' ? new Date(iso) : iso;
  if (Number.isNaN(d.getTime())) return '';
  return (
    d.toLocaleDateString([], { month: 'short', day: 'numeric' }) +
    ' ' +
    fmtTime(d)
  );
}

/** Humanize a duration in milliseconds: ms under 1s, else seconds. */
export function fmtDuration(ms: number | undefined): string {
  if (ms === undefined || ms === null || Number.isNaN(ms)) return '—';
  if (ms < 1000) return `${Math.round(ms)} ms`;
  const s = ms / 1000;
  if (s < 60) return `${s >= 10 ? s.toFixed(1) : s.toFixed(2)} s`;
  const m = Math.floor(s / 60);
  const rem = s - m * 60;
  return `${m}m ${rem.toFixed(0)}s`;
}

/** USD cost with 4–5 decimals. */
export function fmtCost(usd: number | undefined): string {
  if (usd === undefined || usd === null || Number.isNaN(usd)) return '—';
  return `$${usd.toFixed(usd < 0.01 ? 5 : 4)}`;
}

/** Compact tick label for the heatmap X axis. */
export function fmtTick(d: Date, windowMs: number): string {
  if (windowMs > 6 * 3600_000) {
    return d.toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    });
  }
  return fmtTime(d);
}

// Shared bucketing math for the heatmap grid and cell-selection filtering.

import type { TraceSummary } from './api';

export interface Preset {
  key: string;
  label: string;
  ms: number;
  cols: number;
}

export const PRESETS: Preset[] = [
  { key: '5m', label: 'Last 5m', ms: 5 * 60_000, cols: 60 },
  { key: '15m', label: 'Last 15m', ms: 15 * 60_000, cols: 60 },
  { key: '1h', label: 'Last 1h', ms: 60 * 60_000, cols: 60 },
  { key: '6h', label: 'Last 6h', ms: 6 * 3600_000, cols: 48 },
  { key: '24h', label: 'Last 24h', ms: 24 * 3600_000, cols: 48 },
];

export const DEFAULT_PRESET_KEY = '15m';

export function presetByKey(key: string): Preset {
  return PRESETS.find((p) => p.key === key) ?? PRESETS[1];
}

// Fixed log-scaled latency bands (upper bounds in ms; last is open-ended).
export const LATENCY_BOUNDS_MS = [250, 500, 1000, 2000, 4000, 8000, 16000];

export const LATENCY_LABELS = [
  '<250ms',
  '500ms',
  '1s',
  '2s',
  '4s',
  '8s',
  '16s',
  '>16s',
];

export const N_BANDS = LATENCY_LABELS.length; // 8

/** Band index (0 = fastest) for a final latency. */
export function latencyBand(latencyMs: number): number {
  for (let i = 0; i < LATENCY_BOUNDS_MS.length; i++) {
    if (latencyMs < LATENCY_BOUNDS_MS[i]) return i;
  }
  return N_BANDS - 1;
}

export interface GridWindow {
  /** epoch ms of the first (oldest) column's start */
  fromMs: number;
  /** epoch ms of the end of the last column (>= now) */
  toMs: number;
  bucketMs: number;
  cols: number;
}

/**
 * Bucket-aligned grid window ending at "now". Columns are aligned to
 * bucket boundaries so cells are stable identities as the axis tracks now.
 */
export function gridWindow(nowMs: number, preset: Preset): GridWindow {
  const bucketMs = preset.ms / preset.cols;
  const lastStart = Math.floor(nowMs / bucketMs) * bucketMs;
  const toMs = lastStart + bucketMs;
  const fromMs = toMs - preset.cols * bucketMs;
  return { fromMs, toMs, bucketMs, cols: preset.cols };
}

/** Column index for a timestamp, or -1 when outside the grid. */
export function timeCol(iso: string, grid: GridWindow): number {
  const t = new Date(iso).getTime();
  if (Number.isNaN(t) || t < grid.fromMs || t >= grid.toMs) return -1;
  return Math.min(
    grid.cols - 1,
    Math.floor((t - grid.fromMs) / grid.bucketMs),
  );
}

/** A selected heatmap cell, keyed by absolute bucket start so it stays put. */
export interface CellSelection {
  bucketStartMs: number;
  band: number;
}

export function selectionCol(
  sel: CellSelection,
  grid: GridWindow,
): number {
  const col = Math.round((sel.bucketStartMs - grid.fromMs) / grid.bucketMs);
  return col >= 0 && col < grid.cols ? col : -1;
}

/** Does a closed trace fall inside the selected cell? */
export function traceInCell(
  t: TraceSummary,
  sel: CellSelection,
  grid: GridWindow,
): boolean {
  if (t.status !== 'closed') return false;
  const col = timeCol(t.start_time, grid);
  return (
    col === selectionCol(sel, grid) && latencyBand(t.latency_ms) === sel.band
  );
}

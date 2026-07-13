'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Paper from '@mui/material/Paper';
import Skeleton from '@mui/material/Skeleton';
import AppHeader from './AppHeader';
import MetricsBar from './MetricsBar';
import FiltersPanel, { DEFAULT_FILTERS, type TraceFilters } from './FiltersPanel';
import Heatmap from './Heatmap';
import TraceTable from './TraceTable';
import {
  API_URL,
  fetchMetrics,
  fetchTraces,
  type LiveEvent,
  type Metrics,
  type TraceSummary,
} from '@/lib/api';
import { useLive } from '@/lib/useLive';
import {
  DEFAULT_PRESET_KEY,
  gridWindow,
  presetByKey,
  selectionCol,
  traceInCell,
  type CellSelection,
} from '@/lib/buckets';

const METRICS_REFRESH_MS = 15_000;
const RETRY_MS = 5_000;

export default function HomeClient() {
  const [presetKey, setPresetKey] = useState(DEFAULT_PRESET_KEY);
  const [filters, setFilters] = useState<TraceFilters>(DEFAULT_FILTERS);
  const [selection, setSelection] = useState<CellSelection | null>(null);
  const [traces, setTraces] = useState<Record<string, TraceSummary>>({});
  const [metrics, setMetrics] = useState<Metrics | null>(null);
  const [loading, setLoading] = useState(true);
  const [apiDown, setApiDown] = useState(false);
  const [nowMs, setNowMs] = useState(() => Date.now());
  // The page is statically prerendered; time-derived UI (heatmap ticks) must
  // not render until after hydration or the server/client HTML mismatch.
  const [mounted, setMounted] = useState(false);

  const preset = presetByKey(presetKey);
  const grid = useMemo(() => gridWindow(nowMs, preset), [nowMs, preset]);

  // 1s ticker: in live mode the X axis tracks "now" (presets always track now).
  useEffect(() => {
    setMounted(true);
    setNowMs(Date.now());
    const t = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);

  // HTTP loads the focus window; re-query on window change and WS reconnect.
  const loadWindow = useCallback(async () => {
    const to = new Date();
    const from = new Date(to.getTime() - preset.ms);
    try {
      const [list, m] = await Promise.all([
        fetchTraces(from, to),
        fetchMetrics(from, to),
      ]);
      const map: Record<string, TraceSummary> = {};
      for (const t of list) map[t.trace_id] = t;
      setTraces(map);
      setMetrics(m);
      setApiDown(false);
    } catch {
      setApiDown(true);
    } finally {
      setLoading(false);
    }
  }, [preset.ms]);

  useEffect(() => {
    setLoading(true);
    loadWindow();
  }, [loadWindow]);

  // Auto-retry while the API is down.
  useEffect(() => {
    if (!apiDown) return;
    const t = setInterval(loadWindow, RETRY_MS);
    return () => clearInterval(t);
  }, [apiDown, loadWindow]);

  // Keep the metrics tiles honest as the live window slides.
  useEffect(() => {
    const t = setInterval(async () => {
      const to = new Date();
      const from = new Date(to.getTime() - preset.ms);
      try {
        setMetrics(await fetchMetrics(from, to));
        setApiDown(false);
      } catch {
        setApiDown(true);
      }
      // Bound memory: drop traces that scrolled far out of every window.
      const cutoff = Date.now() - 25 * 3600_000;
      setTraces((prev) => {
        const next: Record<string, TraceSummary> = {};
        for (const [id, tr] of Object.entries(prev)) {
          if (new Date(tr.start_time).getTime() >= cutoff) next[id] = tr;
        }
        return Object.keys(next).length === Object.keys(prev).length
          ? prev
          : next;
      });
    }, METRICS_REFRESH_MS);
    return () => clearInterval(t);
  }, [preset.ms]);

  // WebSocket carries the live edge only.
  const onEvent = useCallback((ev: LiveEvent) => {
    if (ev.type !== 'trace_upsert') return;
    setTraces((prev) => ({ ...prev, [ev.summary.trace_id]: ev.summary }));
  }, []);

  const { connected } = useLive({ onEvent, onReconnect: loadWindow });

  // Clear a selection once its bucket scrolls off the left edge.
  const selectionGone =
    selection !== null && selectionCol(selection, grid) === -1;
  useEffect(() => {
    if (selectionGone) setSelection(null);
  }, [selectionGone]);

  // ---- derived data -------------------------------------------------------

  const inWindow = useMemo(() => {
    const list = Object.values(traces).filter((t) => {
      const ts = new Date(t.start_time).getTime();
      return ts >= grid.fromMs && ts < grid.toMs;
    });
    list.sort(
      (a, b) =>
        new Date(b.start_time).getTime() - new Date(a.start_time).getTime(),
    );
    return list;
  }, [traces, grid]);

  const filtered = useMemo(
    () =>
      inWindow.filter(
        (t) =>
          (t.error_count > 0 ? filters.withErrors : filters.withoutErrors) &&
          (t.warning_count > 0
            ? filters.withWarnings
            : filters.withoutWarnings),
      ),
    [inWindow, filters],
  );

  const closedFiltered = useMemo(
    () => filtered.filter((t) => t.status === 'closed'),
    [filtered],
  );

  // With a cell selected, the table shows just that bucket's closed traces;
  // otherwise all filtered traces, running ones included.
  const tableTraces = useMemo(() => {
    if (!selection || selectionGone) return filtered;
    return closedFiltered.filter((t) => traceInCell(t, selection, grid));
  }, [filtered, closedFiltered, selection, selectionGone, grid]);

  return (
    <Box sx={{ minHeight: '100vh', bgcolor: 'background.default' }}>
      <AppHeader live={connected} />
      <Container maxWidth="xl" sx={{ py: 2.5 }}>
        {apiDown && (
          <Alert
            severity="error"
            sx={{ mb: 2 }}
            action={
              <Button color="inherit" size="small" onClick={loadWindow}>
                Retry now
              </Button>
            }
          >
            Cannot reach the AgentTrace API at {API_URL} — retrying
            automatically every {RETRY_MS / 1000}s.
          </Alert>
        )}

        <MetricsBar metrics={metrics} loading={loading} />

        <Box sx={{ display: 'flex', gap: 2, mt: 2, alignItems: 'flex-start' }}>
          <FiltersPanel
            presetKey={presetKey}
            onPresetChange={(k) => {
              setPresetKey(k);
              setSelection(null);
            }}
            filters={filters}
            onFiltersChange={setFilters}
          />

          <Box
            sx={{
              flex: 1,
              minWidth: 0,
              display: 'flex',
              flexDirection: 'column',
              gap: 2,
            }}
          >
            {mounted ? (
              <Heatmap
                traces={closedFiltered}
                grid={grid}
                selection={selectionGone ? null : selection}
                onSelect={setSelection}
              />
            ) : (
              <Paper sx={{ p: 2 }}>
                <Skeleton variant="rectangular" height={200} />
              </Paper>
            )}
            <TraceTable
              traces={tableTraces}
              loading={loading || !mounted}
              emptyHint={
                selection
                  ? 'No closed traces in the selected cell.'
                  : 'Waiting for fleet traffic…'
              }
            />
          </Box>
        </Box>
      </Container>
    </Box>
  );
}

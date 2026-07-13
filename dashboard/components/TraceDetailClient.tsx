'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import Link from 'next/link';
import Box from '@mui/material/Box';
import Container from '@mui/material/Container';
import Typography from '@mui/material/Typography';
import IconButton from '@mui/material/IconButton';
import Chip from '@mui/material/Chip';
import Paper from '@mui/material/Paper';
import Tabs from '@mui/material/Tabs';
import Tab from '@mui/material/Tab';
import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Skeleton from '@mui/material/Skeleton';
import ArrowBackIcon from '@mui/icons-material/ArrowBack';
import AppHeader from './AppHeader';
import Waterfall from './Waterfall';
import BehaviorTree from './BehaviorTree';
import SpanSheet, { SHEET_WIDTH } from './SpanSheet';
import {
  API_URL,
  fetchTrace,
  type LiveEvent,
  type Span,
  type TraceDetail,
} from '@/lib/api';
import { useLive } from '@/lib/useLive';
import { fmtCost, fmtDateTime, fmtDuration } from '@/lib/format';

function HeaderTile({ label, value }: { label: string; value: string }) {
  return (
    <Paper sx={{ flex: 1, minWidth: 150, px: 2, py: 1.5 }}>
      <Typography
        variant="caption"
        sx={{ textTransform: 'uppercase', letterSpacing: 0.5, fontSize: 11 }}
      >
        {label}
      </Typography>
      <Typography sx={{ fontSize: 22, fontWeight: 400, lineHeight: 1.4 }}>
        {value}
      </Typography>
    </Paper>
  );
}

export default function TraceDetailClient({ traceId }: { traceId: string }) {
  const [detail, setDetail] = useState<TraceDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [tab, setTab] = useState(0);
  const [selectedSpan, setSelectedSpan] = useState<Span | null>(null);
  const [nowMs, setNowMs] = useState(() => Date.now());

  const load = useCallback(async () => {
    try {
      const d = await fetchTrace(traceId);
      setDetail(d);
      setError(null);
    } catch (e) {
      setError(e instanceof Error ? e.message : 'request failed');
    } finally {
      setLoading(false);
    }
  }, [traceId]);

  useEffect(() => {
    setLoading(true);
    load();
  }, [load]);

  // Auto-retry while the API is down.
  useEffect(() => {
    if (!error) return;
    const t = setInterval(load, 5000);
    return () => clearInterval(t);
  }, [error, load]);

  const running = detail?.summary.status === 'running';

  // While running, tick the duration tile once per second.
  useEffect(() => {
    if (!running) return;
    const t = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(t);
  }, [running]);

  // Live edge: append spans and update the header while the trace runs.
  const onEvent = useCallback(
    (ev: LiveEvent) => {
      if (ev.type === 'span') {
        if (ev.span.trace_id !== traceId) return;
        setDetail((prev) => {
          if (!prev) return prev;
          const idx = prev.spans.findIndex(
            (s) => s.span_id === ev.span.span_id,
          );
          const spans =
            idx >= 0
              ? prev.spans.map((s, i) => (i === idx ? ev.span : s))
              : [...prev.spans, ev.span];
          return { ...prev, spans };
        });
      } else if (ev.type === 'trace_upsert') {
        if (ev.summary.trace_id !== traceId) return;
        setDetail((prev) => (prev ? { ...prev, summary: ev.summary } : prev));
        if (ev.summary.status === 'closed') {
          // Closing may finalize the behavior tree — pick it up once.
          load();
        }
      }
    },
    [traceId, load],
  );

  useLive({ onEvent, onReconnect: load, enabled: running === true });

  const summary = detail?.summary;
  const durationMs = useMemo(() => {
    if (!summary) return undefined;
    const start = new Date(summary.start_time).getTime();
    const end = summary.end_time
      ? new Date(summary.end_time).getTime()
      : nowMs;
    return Math.max(0, end - start);
  }, [summary, nowMs]);

  const keepSheetSelectionFresh = detail?.spans;
  useEffect(() => {
    // If a live update replaced the selected span object, keep the sheet
    // pointed at the fresh copy.
    if (!selectedSpan || !keepSheetSelectionFresh) return;
    const fresh = keepSheetSelectionFresh.find(
      (s) => s.span_id === selectedSpan.span_id,
    );
    if (fresh && fresh !== selectedSpan) setSelectedSpan(fresh);
  }, [keepSheetSelectionFresh, selectedSpan]);

  return (
    <Box sx={{ minHeight: '100vh', bgcolor: 'background.default' }}>
      <AppHeader live={running ? true : undefined} />
      <Box
        sx={{
          transition: 'margin-right 225ms cubic-bezier(0, 0, 0.2, 1)',
          marginRight: selectedSpan ? `${SHEET_WIDTH}px` : 0,
        }}
      >
        <Container maxWidth="xl" sx={{ py: 2.5 }}>
          {error && (
            <Alert
              severity="error"
              sx={{ mb: 2 }}
              action={
                <Button color="inherit" size="small" onClick={load}>
                  Retry now
                </Button>
              }
            >
              Cannot reach the AgentTrace API at {API_URL} — retrying
              automatically every 5s. ({error})
            </Alert>
          )}

          {/* header line */}
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
            <IconButton component={Link} href="/" size="small" aria-label="back">
              <ArrowBackIcon fontSize="small" />
            </IconButton>
            <Typography
              variant="h6"
              sx={{ fontFamily: 'Roboto Mono, monospace', fontSize: 17 }}
            >
              {traceId}
            </Typography>
            {summary && (
              <>
                <Chip size="small" label={summary.agent_id} variant="outlined" />
                <Chip
                  size="small"
                  label={summary.status}
                  sx={{
                    bgcolor: running ? '#e8f0fe' : '#e6f4ea',
                    color: running ? 'primary.main' : '#188038',
                  }}
                />
              </>
            )}
          </Box>

          {/* metric tiles */}
          <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'wrap', mb: 2 }}>
            {loading || !summary ? (
              Array.from({ length: 4 }, (_, i) => (
                <Paper key={i} sx={{ flex: 1, minWidth: 150, px: 2, py: 1.5 }}>
                  <Skeleton width={80} />
                  <Skeleton width={110} height={32} />
                </Paper>
              ))
            ) : (
              <>
                <HeaderTile
                  label="Start time"
                  value={fmtDateTime(summary.start_time)}
                />
                <HeaderTile label="Duration" value={fmtDuration(durationMs)} />
                <HeaderTile
                  label="Latency"
                  value={
                    summary.status === 'closed'
                      ? fmtDuration(summary.latency_ms)
                      : '— running'
                  }
                />
                <HeaderTile
                  label="Total cost"
                  value={fmtCost(summary.total_cost_usd)}
                />
              </>
            )}
          </Box>

          <Paper sx={{ mb: 2 }}>
            <Tabs
              value={tab}
              onChange={(_, v) => setTab(v)}
              sx={{ borderBottom: '1px solid #dadce0', minHeight: 42 }}
            >
              <Tab
                label="Trace Details"
                sx={{ textTransform: 'none', minHeight: 42 }}
              />
              <Tab
                label="Decision Tree"
                sx={{ textTransform: 'none', minHeight: 42 }}
              />
            </Tabs>
          </Paper>

          {loading && (
            <Paper sx={{ p: 2 }}>
              {Array.from({ length: 6 }, (_, i) => (
                <Skeleton key={i} height={30} />
              ))}
            </Paper>
          )}

          {!loading && detail && tab === 0 && (
            <Waterfall
              spans={detail.spans}
              missionLabel={detail.summary.purpose || `Trace ${traceId}`}
              selectedSpanId={selectedSpan?.span_id ?? null}
              onSelectSpan={setSelectedSpan}
            />
          )}

          {!loading && detail && tab === 1 && (
            <BehaviorTree
              root={detail.behavior_tree ?? null}
              spans={detail.spans}
              selectedSpanId={selectedSpan?.span_id ?? null}
              onSelectSpan={setSelectedSpan}
            />
          )}

          {!loading && !detail && !error && (
            <Paper sx={{ p: 5, textAlign: 'center' }}>
              <Typography variant="body2" color="text.secondary">
                Trace not found.
              </Typography>
            </Paper>
          )}
        </Container>
      </Box>

      <SpanSheet span={selectedSpan} onClose={() => setSelectedSpan(null)} />
    </Box>
  );
}

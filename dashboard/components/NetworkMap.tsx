'use client';

// Live network map of the fleet: every agent observed in the window on the
// left, every backend it called on the right, an edge = "this agent has
// called this backend". The topology is not configured anywhere — it is
// derived entirely from captured traffic, which is the product's point: plug
// in the proxy and the map draws itself. Data re-syncs every 15s (same
// liveness pattern as the metrics tiles); edges are static connectivity.

import { useEffect, useMemo, useState } from 'react';
import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import Skeleton from '@mui/material/Skeleton';
import { fetchTopology, type Topology } from '@/lib/api';
import { ERROR_RED, GOOGLE_BLUE, WARNING_AMBER } from '@/lib/theme';
import { SPAN_META } from './spanMeta';

const NODE_W = 190;
const NODE_H = 40;
const ROW_GAP = 16;
const COL_GAP = 330; // horizontal space the edges cross
const PAD = 16;
const DIMMED = '#dadce0';
const INK = '#202124';
const MUTED = '#5f6368';

interface Props {
  /** Agent to highlight (the trace's own agent); its edges draw in blue. */
  highlightAgent?: string;
}

export default function NetworkMap({ highlightAgent }: Props) {
  const [topo, setTopo] = useState<Topology | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    const load = () =>
      fetchTopology()
        .then((t) => {
          if (!alive) return;
          setTopo(t);
          setError(null);
        })
        .catch((e) => {
          if (alive) setError(e instanceof Error ? e.message : 'failed');
        });
    load();
    const t = setInterval(load, 15_000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, []);

  const layout = useMemo(() => {
    if (!topo) return null;
    const rows = Math.max(topo.agents.length, topo.backends.length, 1);
    const height = PAD * 2 + rows * NODE_H + (rows - 1) * ROW_GAP;
    const width = PAD * 2 + NODE_W * 2 + COL_GAP;
    const colY = (count: number, i: number) => {
      // vertically center the shorter column
      const columnHeight = count * NODE_H + (count - 1) * ROW_GAP;
      const top = (height - columnHeight) / 2;
      return top + i * (NODE_H + ROW_GAP);
    };
    const agentPos = new Map(
      topo.agents.map((a, i) => [
        a.id,
        { x: PAD, y: colY(topo.agents.length, i) },
      ]),
    );
    const backendPos = new Map(
      topo.backends.map((b, i) => [
        b.id,
        { x: PAD + NODE_W + COL_GAP, y: colY(topo.backends.length, i) },
      ]),
    );
    return { width, height, agentPos, backendPos };
  }, [topo]);

  if (error) {
    return (
      <Paper sx={{ p: 5, textAlign: 'center' }}>
        <Typography variant="body2" color="text.secondary">
          Network map unavailable — {error}
        </Typography>
      </Paper>
    );
  }
  if (!topo || !layout) {
    return (
      <Paper sx={{ p: 2 }}>
        {Array.from({ length: 5 }, (_, i) => (
          <Skeleton key={i} height={36} />
        ))}
      </Paper>
    );
  }
  if (topo.agents.length === 0) {
    return (
      <Paper sx={{ p: 5, textAlign: 'center' }}>
        <Typography variant="body2" color="text.secondary">
          No traffic captured in the last 24 hours — the map draws itself once
          agents start talking through the proxy.
        </Typography>
      </Paper>
    );
  }

  const hasHighlight =
    !!highlightAgent && topo.agents.some((a) => a.id === highlightAgent);
  const { width, height, agentPos, backendPos } = layout;

  return (
    <Paper sx={{ p: 2 }}>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'baseline',
          justifyContent: 'space-between',
          flexWrap: 'wrap',
          gap: 1,
          mb: 1,
        }}
      >
        <Typography sx={{ fontSize: 14, fontWeight: 500 }}>
          Live network map
          <Typography component="span" sx={{ fontSize: 12, color: MUTED, ml: 1 }}>
            every agent observed in the last 24 h · derived from captured
            traffic, nothing configured
          </Typography>
        </Typography>
        <Typography sx={{ fontSize: 12, color: MUTED }}>
          <Box component="span" sx={{ color: ERROR_RED }}>●</Box> errors on
          edge&nbsp;&nbsp;
          <Box component="span" sx={{ color: WARNING_AMBER }}>●</Box> warnings
          on edge
          {hasHighlight && (
            <>
              &nbsp;&nbsp;
              <Box component="span" sx={{ color: GOOGLE_BLUE }}>—</Box> this
              trace&apos;s agent
            </>
          )}
        </Typography>
      </Box>

      <Box sx={{ overflowX: 'auto' }}>
        <svg
          width={width}
          height={height}
          viewBox={`0 0 ${width} ${height}`}
          role="img"
          aria-label="Fleet network map: agents and the backends they call"
        >
          {/* edges under nodes */}
          {topo.edges.map((e) => {
            const a = agentPos.get(e.agent);
            const b = backendPos.get(e.backend);
            if (!a || !b) return null;
            const x1 = a.x + NODE_W;
            const y1 = a.y + NODE_H / 2;
            const x2 = b.x;
            const y2 = b.y + NODE_H / 2;
            const mx = (x1 + x2) / 2;
            const active = !hasHighlight || e.agent === highlightAgent;
            const stroke = !active
              ? DIMMED
              : hasHighlight
                ? GOOGLE_BLUE
                : '#9aa0a6';
            return (
              <g key={`${e.agent}->${e.backend}`} opacity={active ? 1 : 0.45}>
                <path
                  d={`M ${x1} ${y1} C ${mx} ${y1}, ${mx} ${y2}, ${x2} ${y2}`}
                  fill="none"
                  stroke={stroke}
                  strokeWidth={active && hasHighlight ? 2 : 1.5}
                >
                  <title>
                    {`${e.agent} → ${e.backend}\n${e.calls} calls · ${e.errors} errors · ${e.warnings} warnings`}
                  </title>
                </path>
                {e.errors > 0 && (
                  <circle cx={mx} cy={(y1 + y2) / 2} r={4} fill={ERROR_RED}>
                    <title>{`${e.errors} errors on this edge`}</title>
                  </circle>
                )}
                {e.warnings > 0 && (
                  <circle
                    cx={mx + (e.errors > 0 ? 12 : 0)}
                    cy={(y1 + y2) / 2}
                    r={4}
                    fill={WARNING_AMBER}
                  >
                    <title>{`${e.warnings} warnings on this edge`}</title>
                  </circle>
                )}
              </g>
            );
          })}

          {/* agent nodes (left) */}
          {topo.agents.map((a) => {
            const p = agentPos.get(a.id)!;
            const isHighlight = hasHighlight && a.id === highlightAgent;
            const dim = hasHighlight && !isHighlight;
            return (
              <g key={a.id} opacity={dim ? 0.5 : 1}>
                <rect
                  x={p.x}
                  y={p.y}
                  width={NODE_W}
                  height={NODE_H}
                  rx={8}
                  fill={isHighlight ? '#e8f0fe' : '#ffffff'}
                  stroke={isHighlight ? GOOGLE_BLUE : '#dadce0'}
                  strokeWidth={isHighlight ? 2 : 1}
                />
                <text
                  x={p.x + 12}
                  y={p.y + 17}
                  fontSize={12.5}
                  fontWeight={isHighlight ? 600 : 500}
                  fill={INK}
                >
                  {a.id.length > 24 ? a.id.slice(0, 23) + '…' : a.id}
                  <title>{a.id}</title>
                </text>
                <text x={p.x + 12} y={p.y + 32} fontSize={10.5} fill={MUTED}>
                  {a.calls.toLocaleString()} calls
                </text>
              </g>
            );
          })}

          {/* backend nodes (right) */}
          {topo.backends.map((b) => {
            const p = backendPos.get(b.id)!;
            const tint = (SPAN_META[b.kind] ?? SPAN_META.external).tint;
            return (
              <g key={b.id}>
                <rect
                  x={p.x}
                  y={p.y}
                  width={NODE_W}
                  height={NODE_H}
                  rx={8}
                  fill="#ffffff"
                  stroke="#dadce0"
                />
                <rect
                  x={p.x}
                  y={p.y}
                  width={4}
                  height={NODE_H}
                  rx={2}
                  fill={tint}
                />
                <text
                  x={p.x + 14}
                  y={p.y + 17}
                  fontSize={12.5}
                  fontWeight={500}
                  fill={INK}
                >
                  {b.label.length > 24 ? b.label.slice(0, 23) + '…' : b.label}
                  <title>{b.label}</title>
                </text>
                <text x={p.x + 14} y={p.y + 32} fontSize={10.5} fill={MUTED}>
                  {(SPAN_META[b.kind] ?? SPAN_META.external).label}
                </text>
              </g>
            );
          })}

          {/* column headings */}
          <text x={PAD} y={12} fontSize={11} fill={MUTED} letterSpacing={0.8}>
            AGENTS
          </text>
          <text
            x={PAD + NODE_W + COL_GAP}
            y={12}
            fontSize={11}
            fill={MUTED}
            letterSpacing={0.8}
          >
            BACKENDS
          </text>
        </svg>
      </Box>
    </Paper>
  );
}

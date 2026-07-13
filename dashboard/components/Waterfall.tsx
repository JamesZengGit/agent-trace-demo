'use client';

// Jaeger-style waterfall: left collapsible span tree (mission root + span
// rows in time order), right timeline bars sharing row alignment, time ruler.

import { useMemo, useState } from 'react';
import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import type { Span } from '@/lib/api';
import { fmtDuration } from '@/lib/format';
import { spanMeta, spanStatusColor } from './spanMeta';

const LEFT_W = 300;
const ROW_H = 34;
const RULER_TICKS = 6;

export default function Waterfall({
  spans,
  missionLabel,
  selectedSpanId,
  onSelectSpan,
}: {
  spans: Span[];
  missionLabel: string;
  selectedSpanId: string | null;
  onSelectSpan: (span: Span) => void;
}) {
  const [expanded, setExpanded] = useState(true);

  const ordered = useMemo(
    () =>
      [...spans].sort(
        (a, b) =>
          new Date(a.start_time).getTime() - new Date(b.start_time).getTime(),
      ),
    [spans],
  );

  const bounds = useMemo(() => {
    let min = Infinity;
    let max = -Infinity;
    for (const s of ordered) {
      const st = new Date(s.start_time).getTime();
      const en = new Date(s.end_time).getTime();
      if (!Number.isNaN(st)) min = Math.min(min, st);
      if (!Number.isNaN(en)) max = Math.max(max, en);
    }
    if (!Number.isFinite(min) || !Number.isFinite(max) || max <= min) {
      return { min: 0, total: 1 };
    }
    return { min, total: max - min };
  }, [ordered]);

  const pct = (t: number) =>
    Math.max(0, Math.min(100, ((t - bounds.min) / bounds.total) * 100));

  const ticks = Array.from({ length: RULER_TICKS + 1 }, (_, i) => i / RULER_TICKS);

  return (
    <Paper sx={{ overflow: 'hidden' }}>
      <Box sx={{ overflowX: 'auto' }}>
        <Box sx={{ minWidth: 760 }}>
          {/* ruler */}
          <Box
            sx={{
              display: 'flex',
              borderBottom: '1px solid #dadce0',
              bgcolor: '#f8f9fa',
            }}
          >
            <Box
              sx={{
                width: LEFT_W,
                flexShrink: 0,
                px: 1.5,
                py: 0.75,
                borderRight: '1px solid #dadce0',
              }}
            >
              <Typography variant="caption" sx={{ fontWeight: 500 }}>
                Span
              </Typography>
            </Box>
            <Box sx={{ flex: 1, position: 'relative', py: 0.75 }}>
              {ticks.map((f) => (
                <Typography
                  key={f}
                  variant="caption"
                  sx={{
                    position: 'absolute',
                    left: `${f * 100}%`,
                    transform:
                      f === 0
                        ? 'translateX(2px)'
                        : f === 1
                          ? 'translateX(calc(-100% - 2px))'
                          : 'translateX(-50%)',
                    fontSize: 10,
                    whiteSpace: 'nowrap',
                  }}
                >
                  {fmtDuration(f * bounds.total)}
                </Typography>
              ))}
              {/* keeps the ruler row from collapsing */}
              <Typography variant="caption" sx={{ visibility: 'hidden' }}>
                0
              </Typography>
            </Box>
          </Box>

          {/* rows */}
          <Box sx={{ maxHeight: '56vh', overflowY: 'auto', position: 'relative' }}>
            {/* mission root row */}
            <Box
              sx={{
                display: 'flex',
                alignItems: 'stretch',
                borderBottom: '1px solid #f1f3f4',
              }}
            >
              <Box
                sx={{
                  width: LEFT_W,
                  flexShrink: 0,
                  display: 'flex',
                  alignItems: 'center',
                  px: 0.5,
                  height: ROW_H,
                  borderRight: '1px solid #dadce0',
                }}
              >
                <IconButton
                  size="small"
                  onClick={() => setExpanded((e) => !e)}
                  aria-label={expanded ? 'collapse spans' : 'expand spans'}
                >
                  {expanded ? (
                    <ExpandMoreIcon fontSize="small" />
                  ) : (
                    <ChevronRightIcon fontSize="small" />
                  )}
                </IconButton>
                <Typography
                  variant="body2"
                  sx={{
                    fontWeight: 500,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                  title={missionLabel}
                >
                  {missionLabel}
                </Typography>
              </Box>
              <Box sx={{ flex: 1, position: 'relative' }}>
                <GridLines ticks={ticks} />
                <Box
                  sx={{
                    position: 'absolute',
                    top: '50%',
                    transform: 'translateY(-50%)',
                    left: 0,
                    right: 0,
                    height: 6,
                    borderRadius: 3,
                    bgcolor: '#e8eaed',
                  }}
                />
              </Box>
            </Box>

            {expanded &&
              ordered.map((s) => {
                const st = new Date(s.start_time).getTime();
                const en = new Date(s.end_time).getTime();
                const left = pct(st);
                const width = Math.max(0.4, pct(en) - left);
                const color = spanStatusColor(s);
                const { Icon, tint, label } = spanMeta(s.type);
                const selected = s.span_id === selectedSpanId;
                const dur = fmtDuration(en - st);
                const labelInside = left + width > 82;
                return (
                  <Box
                    key={s.span_id}
                    onClick={() => onSelectSpan(s)}
                    sx={{
                      display: 'flex',
                      alignItems: 'stretch',
                      cursor: 'pointer',
                      bgcolor: selected ? '#e8f0fe' : 'transparent',
                      borderBottom: '1px solid #f1f3f4',
                      '&:hover': { bgcolor: selected ? '#e8f0fe' : '#f8f9fa' },
                    }}
                  >
                    {/* left tree column (spans indented one level) */}
                    <Box
                      sx={{
                        width: LEFT_W,
                        flexShrink: 0,
                        display: 'flex',
                        alignItems: 'center',
                        gap: 0.75,
                        pl: 4.5,
                        pr: 1,
                        height: ROW_H,
                        borderRight: '1px solid #dadce0',
                        minWidth: 0,
                      }}
                    >
                      <Icon sx={{ fontSize: 15, color: tint, flexShrink: 0 }} />
                      <Typography
                        variant="caption"
                        sx={{ color: tint, fontWeight: 500, flexShrink: 0 }}
                      >
                        {label}
                      </Typography>
                      <Tooltip title={s.destination} disableInteractive>
                        <Typography
                          variant="caption"
                          sx={{
                            color: 'text.secondary',
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            whiteSpace: 'nowrap',
                          }}
                        >
                          {s.destination}
                        </Typography>
                      </Tooltip>
                    </Box>

                    {/* timeline bar */}
                    <Box sx={{ flex: 1, position: 'relative' }}>
                      <GridLines ticks={ticks} />
                      <Tooltip
                        title={`${label} · ${dur}`}
                        disableInteractive
                        followCursor
                      >
                        <Box
                          sx={{
                            position: 'absolute',
                            top: '50%',
                            transform: 'translateY(-50%)',
                            left: `${left}%`,
                            width: `${width}%`,
                            minWidth: 3,
                            height: 12,
                            borderRadius: '3px',
                            bgcolor: color,
                            boxShadow: selected
                              ? `0 0 0 2px #fff, 0 0 0 3px ${color}`
                              : 'none',
                          }}
                        />
                      </Tooltip>
                      <Typography
                        variant="caption"
                        sx={{
                          position: 'absolute',
                          top: '50%',
                          transform: 'translateY(-50%)',
                          fontSize: 10,
                          whiteSpace: 'nowrap',
                          ...(labelInside
                            ? { right: `${100 - left}%`, pr: 0.5 }
                            : { left: `calc(${left + width}% + 6px)` }),
                        }}
                      >
                        {dur}
                      </Typography>
                    </Box>
                  </Box>
                );
              })}

            {ordered.length === 0 && (
              <Box sx={{ py: 5, textAlign: 'center' }}>
                <Typography variant="body2" color="text.secondary">
                  No spans captured yet…
                </Typography>
              </Box>
            )}
          </Box>
        </Box>
      </Box>
    </Paper>
  );
}

function GridLines({ ticks }: { ticks: number[] }) {
  return (
    <>
      {ticks.slice(1, -1).map((f) => (
        <Box
          key={f}
          sx={{
            position: 'absolute',
            left: `${f * 100}%`,
            top: 0,
            bottom: 0,
            width: '1px',
            bgcolor: '#f1f3f4',
          }}
        />
      ))}
    </>
  );
}

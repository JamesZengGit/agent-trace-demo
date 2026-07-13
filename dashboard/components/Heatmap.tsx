'use client';

// Custom time × latency density heatmap (CSS grid of divs).
// X = time buckets across the focus window, Y = fixed log latency bands.
// Only CLOSED traces are placed here (final latency known).

import { useMemo } from 'react';
import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import Chip from '@mui/material/Chip';
import Tooltip from '@mui/material/Tooltip';
import type { TraceSummary } from '@/lib/api';
import {
  latencyBand,
  timeCol,
  selectionCol,
  LATENCY_LABELS,
  N_BANDS,
  type GridWindow,
  type CellSelection,
} from '@/lib/buckets';
import { fmtTick, fmtTime } from '@/lib/format';
import { ERROR_RED, WARNING_AMBER, GOOGLE_BLUE } from '@/lib/theme';

interface Cell {
  count: number;
  errors: number;
  warnings: number;
}

// Sequential single-hue ramp: light blue → dark Google blue (magnitude job).
const RAMP_LO = [232, 240, 254]; // #e8f0fe
const RAMP_HI = [23, 78, 166]; // #174ea6

function densityColor(t: number): string {
  // sqrt spreads low counts perceptually; t in (0,1]
  const u = Math.sqrt(t);
  const c = RAMP_LO.map((lo, i) => Math.round(lo + (RAMP_HI[i] - lo) * u));
  return `rgb(${c[0]},${c[1]},${c[2]})`;
}

const ROW_H = 22;
const AXIS_W = 56;

export default function Heatmap({
  traces,
  grid,
  selection,
  onSelect,
}: {
  /** closed traces already filtered by the checkbox filters */
  traces: TraceSummary[];
  grid: GridWindow;
  selection: CellSelection | null;
  onSelect: (sel: CellSelection | null) => void;
}) {
  const { cells, max } = useMemo(() => {
    const cells: Cell[][] = Array.from({ length: N_BANDS }, () =>
      Array.from({ length: grid.cols }, () => ({
        count: 0,
        errors: 0,
        warnings: 0,
      })),
    );
    let max = 0;
    for (const t of traces) {
      if (t.status !== 'closed') continue;
      const col = timeCol(t.start_time, grid);
      if (col < 0) continue;
      const band = latencyBand(t.latency_ms);
      const cell = cells[band][col];
      cell.count += 1;
      if (t.error_count > 0) cell.errors += 1;
      if (t.warning_count > 0) cell.warnings += 1;
      if (cell.count > max) max = cell.count;
    }
    return { cells, max };
  }, [traces, grid]);

  const selCol = selection ? selectionCol(selection, grid) : -1;

  // X tick labels roughly every cols/6 columns.
  const tickEvery = Math.max(1, Math.round(grid.cols / 6));
  const windowMs = grid.toMs - grid.fromMs;

  const handleClick = (band: number, col: number) => {
    const bucketStartMs = grid.fromMs + col * grid.bucketMs;
    if (selection && selection.band === band && selCol === col) {
      onSelect(null); // click again clears
    } else {
      onSelect({ bucketStartMs, band });
    }
  };

  return (
    <Paper sx={{ p: 2 }}>
      <Box sx={{ display: 'flex', alignItems: 'center', mb: 1, gap: 1 }}>
        <Typography variant="subtitle2" sx={{ color: 'text.primary' }}>
          Latency heatmap
        </Typography>
        <Typography variant="caption">
          closed traces · density {max > 0 ? `(max ${max}/cell)` : ''}
        </Typography>
        <Box sx={{ flex: 1 }} />
        {selection && (
          <Chip
            size="small"
            label={`${fmtTime(new Date(selection.bucketStartMs))} · ${LATENCY_LABELS[selection.band]}`}
            onDelete={() => onSelect(null)}
            sx={{ bgcolor: '#e8f0fe', color: GOOGLE_BLUE }}
          />
        )}
      </Box>

      <Box sx={{ display: 'flex' }}>
        {/* Y axis labels (highest latency on top) */}
        <Box sx={{ width: AXIS_W, flexShrink: 0 }}>
          {Array.from({ length: N_BANDS }, (_, i) => N_BANDS - 1 - i).map(
            (band) => (
              <Box
                key={band}
                sx={{
                  height: ROW_H,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'flex-end',
                  pr: 1,
                }}
              >
                <Typography variant="caption" sx={{ fontSize: 10.5 }}>
                  {LATENCY_LABELS[band]}
                </Typography>
              </Box>
            ),
          )}
        </Box>

        {/* Grid */}
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: `repeat(${grid.cols}, 1fr)`,
              gridAutoRows: `${ROW_H}px`,
              gap: '1px',
              bgcolor: '#fff',
            }}
          >
            {Array.from({ length: N_BANDS }, (_, i) => N_BANDS - 1 - i).map(
              (band) =>
                cells[band].map((cell, col) => {
                  const selected = selection?.band === band && selCol === col;
                  const bg =
                    cell.count > 0 && max > 0
                      ? densityColor(cell.count / max)
                      : '#f8f9fa';
                  const bucketStart = grid.fromMs + col * grid.bucketMs;
                  const tip =
                    cell.count > 0
                      ? `${cell.count} trace${cell.count === 1 ? '' : 's'} · ${fmtTime(new Date(bucketStart))} · ${LATENCY_LABELS[band]}` +
                        (cell.errors > 0 ? ` · ${cell.errors} error` : '') +
                        (cell.warnings > 0 ? ` · ${cell.warnings} warning` : '')
                      : '';
                  const cellBox = (
                    <Box
                      key={`${band}-${col}`}
                      onClick={
                        cell.count > 0 || selected
                          ? () => handleClick(band, col)
                          : undefined
                      }
                      sx={{
                        position: 'relative',
                        bgcolor: bg,
                        cursor: cell.count > 0 ? 'pointer' : 'default',
                        overflow: 'hidden',
                        ...(selected && {
                          overflow: 'visible',
                          outline: '2px solid #ffffff',
                          boxShadow: `0 0 0 4px ${GOOGLE_BLUE}, 0 2px 8px rgba(60,64,67,0.4)`,
                          zIndex: 2,
                        }),
                        '&:hover':
                          cell.count > 0
                            ? { outline: `1px solid ${GOOGLE_BLUE}`, zIndex: 1 }
                            : undefined,
                      }}
                    >
                      {/* error mark: red top-right corner triangle */}
                      {cell.errors > 0 && (
                        <Box
                          sx={{
                            position: 'absolute',
                            top: 0,
                            right: 0,
                            width: 0,
                            height: 0,
                            borderTop: `7px solid ${ERROR_RED}`,
                            borderLeft: '7px solid transparent',
                          }}
                        />
                      )}
                      {/* warning mark: amber bottom-left corner triangle */}
                      {cell.warnings > 0 && (
                        <Box
                          sx={{
                            position: 'absolute',
                            bottom: 0,
                            left: 0,
                            width: 0,
                            height: 0,
                            borderBottom: `7px solid ${WARNING_AMBER}`,
                            borderRight: '7px solid transparent',
                          }}
                        />
                      )}
                    </Box>
                  );
                  return cell.count > 0 ? (
                    <Tooltip
                      key={`${band}-${col}`}
                      title={tip}
                      placement="top"
                      arrow
                      disableInteractive
                    >
                      {cellBox}
                    </Tooltip>
                  ) : (
                    cellBox
                  );
                }),
            )}
          </Box>

          {/* X axis ticks */}
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: `repeat(${grid.cols}, 1fr)`,
              mt: 0.5,
            }}
          >
            {Array.from({ length: grid.cols }, (_, col) => (
              <Box key={col} sx={{ overflow: 'visible', minWidth: 0 }}>
                {col % tickEvery === 0 && (
                  <Typography
                    variant="caption"
                    sx={{ fontSize: 10, whiteSpace: 'nowrap' }}
                  >
                    {fmtTick(
                      new Date(grid.fromMs + col * grid.bucketMs),
                      windowMs,
                    )}
                  </Typography>
                )}
              </Box>
            ))}
          </Box>
        </Box>
      </Box>

      {/* Legend: density ramp + status marks */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mt: 1.5 }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
          <Typography variant="caption" sx={{ fontSize: 10.5 }}>
            fewer
          </Typography>
          <Box
            sx={{
              width: 80,
              height: 8,
              borderRadius: 4,
              background: `linear-gradient(to right, #e8f0fe, ${GOOGLE_BLUE}, #174ea6)`,
            }}
          />
          <Typography variant="caption" sx={{ fontSize: 10.5 }}>
            more traces
          </Typography>
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <Box
            sx={{
              width: 0,
              height: 0,
              borderTop: `8px solid ${ERROR_RED}`,
              borderLeft: '8px solid transparent',
            }}
          />
          <Typography variant="caption" sx={{ fontSize: 10.5 }}>
            contains error
          </Typography>
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          <Box
            sx={{
              width: 0,
              height: 0,
              borderBottom: `8px solid ${WARNING_AMBER}`,
              borderRight: '8px solid transparent',
            }}
          />
          <Typography variant="caption" sx={{ fontSize: 10.5 }}>
            contains warning
          </Typography>
        </Box>
      </Box>
    </Paper>
  );
}

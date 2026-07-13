'use client';

import Paper from '@mui/material/Paper';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Skeleton from '@mui/material/Skeleton';
import type { Metrics } from '@/lib/api';
import { fmtDuration } from '@/lib/format';
import { ERROR_RED, WARNING_AMBER } from '@/lib/theme';

function StatTile({
  label,
  value,
  accent,
  loading,
}: {
  label: string;
  value: string;
  accent?: string;
  loading: boolean;
}) {
  return (
    <Paper sx={{ flex: 1, minWidth: 150, px: 2, py: 1.5 }}>
      <Typography
        variant="caption"
        sx={{ textTransform: 'uppercase', letterSpacing: 0.5, fontSize: 11 }}
      >
        {label}
      </Typography>
      {loading ? (
        <Skeleton width={72} height={36} />
      ) : (
        <Typography
          sx={{ fontSize: 26, fontWeight: 400, lineHeight: 1.3, color: accent }}
        >
          {value}
        </Typography>
      )}
    </Paper>
  );
}

export default function MetricsBar({
  metrics,
  loading,
}: {
  metrics: Metrics | null;
  loading: boolean;
}) {
  const m = metrics;
  return (
    <Box sx={{ display: 'flex', gap: 1.5, flexWrap: 'wrap' }}>
      <StatTile
        label="Trace count"
        value={m ? String(m.trace_count) : '—'}
        loading={loading}
      />
      <StatTile
        label="Average latency"
        value={m ? fmtDuration(m.avg_latency_ms) : '—'}
        loading={loading}
      />
      <StatTile
        label="Error count"
        value={m ? String(m.error_count) : '—'}
        accent={m && m.error_count > 0 ? ERROR_RED : undefined}
        loading={loading}
      />
      <StatTile
        label="Warning count"
        value={m ? String(m.warning_count) : '—'}
        accent={m && m.warning_count > 0 ? WARNING_AMBER : undefined}
        loading={loading}
      />
    </Box>
  );
}

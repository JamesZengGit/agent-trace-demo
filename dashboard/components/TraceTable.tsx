'use client';

import { useRouter } from 'next/navigation';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import Chip from '@mui/material/Chip';
import Box from '@mui/material/Box';
import Skeleton from '@mui/material/Skeleton';
import ErrorOutlineIcon from '@mui/icons-material/ErrorOutline';
import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import type { TraceSummary } from '@/lib/api';
import { fmtTime } from '@/lib/format';
import { ERROR_RED, WARNING_AMBER } from '@/lib/theme';

function CountBadge({
  count,
  kind,
}: {
  count: number;
  kind: 'error' | 'warning';
}) {
  if (count <= 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        —
      </Typography>
    );
  }
  const color = kind === 'error' ? ERROR_RED : WARNING_AMBER;
  const Icon = kind === 'error' ? ErrorOutlineIcon : WarningAmberIcon;
  return (
    <Box sx={{ display: 'inline-flex', alignItems: 'center', gap: 0.5 }}>
      <Icon sx={{ fontSize: 16, color }} />
      <Typography variant="body2" sx={{ color, fontWeight: 500 }}>
        {count}
      </Typography>
    </Box>
  );
}

export default function TraceTable({
  traces,
  loading,
  emptyHint,
}: {
  traces: TraceSummary[];
  loading: boolean;
  emptyHint: string;
}) {
  const router = useRouter();

  return (
    <Paper>
      <Box sx={{ px: 2, pt: 1.5, pb: 0.5 }}>
        <Typography variant="subtitle2" sx={{ color: 'text.primary' }}>
          Trace list
        </Typography>
      </Box>
      <TableContainer sx={{ maxHeight: 12 * 37 + 40 }}>
        <Table stickyHeader size="small">
          <TableHead>
            <TableRow>
              <TableCell>Agent ID</TableCell>
              <TableCell>Trace ID</TableCell>
              <TableCell>Start time</TableCell>
              <TableCell>End time</TableCell>
              <TableCell align="right">Warning</TableCell>
              <TableCell align="right">Error</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {loading &&
              Array.from({ length: 6 }, (_, i) => (
                <TableRow key={`sk-${i}`}>
                  {Array.from({ length: 6 }, (_, j) => (
                    <TableCell key={j}>
                      <Skeleton />
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            {!loading && traces.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} sx={{ py: 5, textAlign: 'center' }}>
                  <Typography variant="body2" color="text.secondary">
                    {emptyHint}
                  </Typography>
                </TableCell>
              </TableRow>
            )}
            {!loading &&
              traces.map((t) => (
                <TableRow
                  key={t.trace_id}
                  hover
                  onClick={() => router.push(`/trace/${t.trace_id}`)}
                  sx={{ cursor: 'pointer' }}
                >
                  <TableCell>
                    <Typography variant="body2">{t.agent_id}</Typography>
                  </TableCell>
                  <TableCell>
                    <Typography
                      variant="body2"
                      sx={{
                        fontFamily: 'Roboto Mono, monospace',
                        fontSize: 12.5,
                        color: 'primary.main',
                      }}
                    >
                      {t.trace_id}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    <Typography variant="body2">
                      {fmtTime(t.start_time)}
                    </Typography>
                  </TableCell>
                  <TableCell>
                    {t.status === 'running' || !t.end_time ? (
                      <Chip
                        size="small"
                        label="running"
                        sx={{
                          height: 20,
                          fontSize: 11,
                          bgcolor: '#e8f0fe',
                          color: 'primary.main',
                        }}
                      />
                    ) : (
                      <Typography variant="body2">
                        {fmtTime(t.end_time)}
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell align="right">
                    <CountBadge count={t.warning_count} kind="warning" />
                  </TableCell>
                  <TableCell align="right">
                    <CountBadge count={t.error_count} kind="error" />
                  </TableCell>
                </TableRow>
              ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Paper>
  );
}

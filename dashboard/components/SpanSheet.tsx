'use client';

// Right-side persistent sheet with three sections:
// Span Details (network facts) / Output (captured content) / Reasoning
// (warnings + infrastructure details).

import Drawer from '@mui/material/Drawer';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import IconButton from '@mui/material/IconButton';
import Chip from '@mui/material/Chip';
import Accordion from '@mui/material/Accordion';
import AccordionSummary from '@mui/material/AccordionSummary';
import AccordionDetails from '@mui/material/AccordionDetails';
import Divider from '@mui/material/Divider';
import CloseIcon from '@mui/icons-material/Close';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import type { Span } from '@/lib/api';
import { fmtDuration, fmtTime } from '@/lib/format';
import { ERROR_RED, WARNING_AMBER } from '@/lib/theme';
import { spanMeta } from './spanMeta';

export const SHEET_WIDTH = 420;

function Row({ label, value }: { label: string; value: React.ReactNode }) {
  if (value === undefined || value === null || value === '') return null;
  return (
    <Box sx={{ display: 'flex', py: 0.5 }}>
      <Typography
        variant="body2"
        color="text.secondary"
        sx={{ width: 130, flexShrink: 0 }}
      >
        {label}
      </Typography>
      <Typography
        variant="body2"
        component="div"
        sx={{ wordBreak: 'break-all', minWidth: 0 }}
      >
        {value}
      </Typography>
    </Box>
  );
}

function MonoBlock({ title, text }: { title: string; text?: string }) {
  return (
    <Box sx={{ mb: 1.5 }}>
      <Typography
        variant="caption"
        sx={{ textTransform: 'uppercase', letterSpacing: 0.5, fontSize: 10.5 }}
      >
        {title}
      </Typography>
      {text ? (
        <Box
          component="pre"
          sx={{
            m: 0,
            mt: 0.5,
            p: 1.25,
            bgcolor: '#f8f9fa',
            border: '1px solid #dadce0',
            borderRadius: 1,
            fontFamily: 'Roboto Mono, monospace',
            fontSize: 12,
            whiteSpace: 'pre-wrap',
            wordBreak: 'break-word',
            maxHeight: 320,
            overflow: 'auto',
          }}
        >
          {text}
        </Box>
      ) : (
        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
          — not captured —
        </Typography>
      )}
    </Box>
  );
}

export default function SpanSheet({
  span,
  onClose,
}: {
  span: Span | null;
  onClose: () => void;
}) {
  const open = span !== null;

  return (
    <Drawer
      variant="persistent"
      anchor="right"
      open={open}
      sx={{
        width: open ? SHEET_WIDTH : 0,
        flexShrink: 0,
        '& .MuiDrawer-paper': {
          width: SHEET_WIDTH,
          boxSizing: 'border-box',
          borderLeft: '1px solid #dadce0',
        },
      }}
    >
      {span && (
        <Box
          sx={{
            height: '100%',
            display: 'flex',
            flexDirection: 'column',
            overflow: 'hidden',
          }}
        >
          <Box
            sx={{
              px: 2,
              py: 1.5,
              display: 'flex',
              alignItems: 'center',
              gap: 1,
              borderBottom: '1px solid #dadce0',
            }}
          >
            {(() => {
              const { Icon, tint, label } = spanMeta(span.type);
              return (
                <Chip
                  size="small"
                  icon={<Icon sx={{ fontSize: 15 }} />}
                  label={label}
                  sx={{
                    bgcolor: `${tint}14`,
                    color: tint,
                    '& .MuiChip-icon': { color: tint },
                  }}
                />
              );
            })()}
            <Typography
              variant="body2"
              sx={{
                fontWeight: 500,
                minWidth: 0,
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
                flex: 1,
              }}
              title={span.destination}
            >
              {span.destination}
            </Typography>
            <IconButton size="small" onClick={onClose} aria-label="close">
              <CloseIcon fontSize="small" />
            </IconButton>
          </Box>

          <Box sx={{ flex: 1, overflowY: 'auto' }}>
            {/* 1 — network facts */}
            <Accordion defaultExpanded disableGutters elevation={0}>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography variant="subtitle2" sx={{ color: 'text.primary' }}>
                  Span Details
                </Typography>
              </AccordionSummary>
              <AccordionDetails sx={{ pt: 0 }}>
                <Row
                  label="Span ID"
                  value={
                    <span style={{ fontFamily: 'Roboto Mono, monospace' }}>
                      {span.span_id}
                    </span>
                  }
                />
                <Row label="Type" value={span.type} />
                <Row label="Method" value={span.method} />
                <Row label="Destination" value={span.destination} />
                <Row
                  label="Status code"
                  value={
                    span.status_code !== undefined ? (
                      <span
                        style={{
                          color:
                            span.status_code >= 400 ? ERROR_RED : undefined,
                          fontWeight: span.status_code >= 400 ? 500 : 400,
                        }}
                      >
                        {span.status_code}
                      </span>
                    ) : undefined
                  }
                />
                <Row label="Client IP" value={span.client_ip} />
                <Row label="Start" value={fmtTime(span.start_time)} />
                <Row label="End" value={fmtTime(span.end_time)} />
                <Row
                  label="Duration"
                  value={fmtDuration(
                    new Date(span.end_time).getTime() -
                      new Date(span.start_time).getTime(),
                  )}
                />
                {span.model && <Row label="Model" value={span.model} />}
                {span.prompt_tokens !== undefined && (
                  <Row
                    label="Tokens"
                    value={`${span.prompt_tokens} prompt / ${span.completion_tokens ?? 0} completion`}
                  />
                )}
              </AccordionDetails>
            </Accordion>
            <Divider />

            {/* 2 — captured content */}
            <Accordion defaultExpanded disableGutters elevation={0}>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography variant="subtitle2" sx={{ color: 'text.primary' }}>
                  Output
                </Typography>
              </AccordionSummary>
              <AccordionDetails sx={{ pt: 0 }}>
                {span.type === 'llm_call' ? (
                  <>
                    <MonoBlock title="System prompt" text={span.system_prompt} />
                    <MonoBlock title="User prompt" text={span.user_prompt} />
                    <MonoBlock title="Response" text={span.response} />
                  </>
                ) : (
                  <>
                    <MonoBlock title="Request body" text={span.request_body} />
                    <MonoBlock title="Response body" text={span.response_body} />
                  </>
                )}
              </AccordionDetails>
            </Accordion>
            <Divider />

            {/* 3 — warnings + infrastructure (not model text) */}
            <Accordion defaultExpanded disableGutters elevation={0}>
              <AccordionSummary expandIcon={<ExpandMoreIcon />}>
                <Typography variant="subtitle2" sx={{ color: 'text.primary' }}>
                  Reasoning
                </Typography>
              </AccordionSummary>
              <AccordionDetails sx={{ pt: 0 }}>
                {span.warnings && span.warnings.length > 0 ? (
                  span.warnings.map((w, i) => (
                    <Box
                      key={i}
                      sx={{
                        display: 'flex',
                        gap: 1,
                        mb: 1.25,
                        p: 1.25,
                        border: `1px solid ${WARNING_AMBER}55`,
                        borderRadius: 1,
                        bgcolor: '#fef7e0',
                      }}
                    >
                      <WarningAmberIcon
                        sx={{ fontSize: 18, color: WARNING_AMBER, mt: 0.25 }}
                      />
                      <Box sx={{ minWidth: 0 }}>
                        <Typography variant="body2" sx={{ fontWeight: 500 }}>
                          {w.rule}
                        </Typography>
                        <Typography variant="caption">
                          source: {w.source}
                        </Typography>
                        <Typography variant="body2" sx={{ mt: 0.25 }}>
                          {w.reason}
                        </Typography>
                      </Box>
                    </Box>
                  ))
                ) : (
                  <Typography
                    variant="body2"
                    color="text.secondary"
                    sx={{ mb: 1.5 }}
                  >
                    No warnings raised for this span.
                  </Typography>
                )}

                <Typography
                  variant="caption"
                  sx={{
                    display: 'block',
                    textTransform: 'uppercase',
                    letterSpacing: 0.5,
                    fontSize: 10.5,
                    mt: 1,
                  }}
                >
                  Infrastructure
                </Typography>
                {span.error && (
                  <Row
                    label="Error kind"
                    value={
                      <span style={{ color: ERROR_RED, fontWeight: 500 }}>
                        {span.error_kind ?? 'unknown'}
                      </span>
                    }
                  />
                )}
                <Row label="Dropped" value={span.dropped ? 'yes' : 'no'} />
                {span.seq !== undefined && (
                  <Row label="Sequence #" value={String(span.seq)} />
                )}
                <Row
                  label="Capture"
                  value="Recorded passively at the egress proxy; content reconstructed from the wire, not agent-reported."
                />
              </AccordionDetails>
            </Accordion>
          </Box>
        </Box>
      )}
    </Drawer>
  );
}

'use client';

// Decision Tree tab: mission root → behavior → sub-behavior → span leaves.
// Indented expandable tree; error/warning dot markers are pre-propagated to
// branches by the backend. Span leaves open the shared SpanSheet.

import { useState } from 'react';
import Box from '@mui/material/Box';
import Paper from '@mui/material/Paper';
import Typography from '@mui/material/Typography';
import IconButton from '@mui/material/IconButton';
import ExpandMoreIcon from '@mui/icons-material/ExpandMore';
import ChevronRightIcon from '@mui/icons-material/ChevronRight';
import AccountTreeIcon from '@mui/icons-material/AccountTree';
import type { BehaviorNode, Span } from '@/lib/api';
import { ERROR_RED, WARNING_AMBER } from '@/lib/theme';
import { spanMeta } from './spanMeta';

function FlagDots({ node }: { node: BehaviorNode }) {
  return (
    <Box sx={{ display: 'inline-flex', gap: 0.5, ml: 1, alignItems: 'center' }}>
      {node.error && (
        <Box
          sx={{
            width: 8,
            height: 8,
            borderRadius: '50%',
            bgcolor: ERROR_RED,
          }}
          title="contains error"
        />
      )}
      {node.warning && (
        <Box
          sx={{
            width: 8,
            height: 8,
            borderRadius: '50%',
            bgcolor: WARNING_AMBER,
          }}
          title="contains warning"
        />
      )}
    </Box>
  );
}

function TreeNode({
  node,
  depth,
  spansById,
  selectedSpanId,
  onSelectSpan,
}: {
  node: BehaviorNode;
  depth: number;
  spansById: Map<string, Span>;
  selectedSpanId: string | null;
  onSelectSpan: (span: Span) => void;
}) {
  const [open, setOpen] = useState(true);
  const children = node.children ?? [];
  const hasChildren = children.length > 0;
  const isSpan = node.kind === 'span';
  const span = isSpan && node.span_id ? spansById.get(node.span_id) : undefined;
  const selected = isSpan && !!node.span_id && node.span_id === selectedSpanId;

  const meta = span ? spanMeta(span.type) : null;

  return (
    <>
      <Box
        onClick={span ? () => onSelectSpan(span) : undefined}
        sx={{
          display: 'flex',
          alignItems: 'center',
          pl: 1 + depth * 3,
          pr: 1.5,
          minHeight: 32,
          cursor: span ? 'pointer' : 'default',
          bgcolor: selected ? '#e8f0fe' : 'transparent',
          borderBottom: '1px solid #f8f9fa',
          '&:hover': span
            ? { bgcolor: selected ? '#e8f0fe' : '#f8f9fa' }
            : undefined,
        }}
      >
        {hasChildren ? (
          <IconButton
            size="small"
            onClick={(e) => {
              e.stopPropagation();
              setOpen((o) => !o);
            }}
            aria-label={open ? 'collapse' : 'expand'}
            sx={{ mr: 0.25 }}
          >
            {open ? (
              <ExpandMoreIcon sx={{ fontSize: 18 }} />
            ) : (
              <ChevronRightIcon sx={{ fontSize: 18 }} />
            )}
          </IconButton>
        ) : (
          <Box sx={{ width: 30, flexShrink: 0 }} />
        )}

        {isSpan && meta ? (
          <>
            <meta.Icon sx={{ fontSize: 15, color: meta.tint, mr: 0.75 }} />
            <Typography
              variant="body2"
              sx={{
                color: meta.tint,
                fontWeight: 500,
                mr: 1,
                flexShrink: 0,
              }}
            >
              {meta.label}
            </Typography>
            <Typography
              variant="body2"
              color="text.secondary"
              sx={{
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {node.label}
            </Typography>
          </>
        ) : (
          <>
            {depth === 0 && (
              <AccountTreeIcon
                sx={{ fontSize: 16, color: 'primary.main', mr: 0.75 }}
              />
            )}
            <Typography
              variant="body2"
              sx={{
                fontWeight: node.kind === 'behavior' || depth === 0 ? 500 : 400,
                color:
                  node.kind === 'sub_behavior' ? 'text.secondary' : 'text.primary',
                fontStyle: node.kind === 'sub_behavior' ? 'italic' : 'normal',
              }}
            >
              {node.label}
            </Typography>
          </>
        )}
        <FlagDots node={node} />
      </Box>
      {open &&
        children.map((c, i) => (
          <TreeNode
            key={`${c.kind}-${c.span_id ?? c.label}-${i}`}
            node={c}
            depth={depth + 1}
            spansById={spansById}
            selectedSpanId={selectedSpanId}
            onSelectSpan={onSelectSpan}
          />
        ))}
    </>
  );
}

export default function BehaviorTree({
  root,
  spans,
  selectedSpanId,
  onSelectSpan,
}: {
  root: BehaviorNode | null;
  spans: Span[];
  selectedSpanId: string | null;
  onSelectSpan: (span: Span) => void;
}) {
  const spansById = new Map(spans.map((s) => [s.span_id, s]));

  return (
    <Paper sx={{ overflow: 'hidden' }}>
      <Box sx={{ maxHeight: '60vh', overflowY: 'auto', py: 0.5 }}>
        {root ? (
          <TreeNode
            node={root}
            depth={0}
            spansById={spansById}
            selectedSpanId={selectedSpanId}
            onSelectSpan={onSelectSpan}
          />
        ) : (
          <Box sx={{ py: 5, textAlign: 'center' }}>
            <Typography variant="body2" color="text.secondary">
              No behavior tree available for this trace.
            </Typography>
          </Box>
        )}
      </Box>
      <Box sx={{ px: 2, py: 1, borderTop: '1px solid #f1f3f4' }}>
        <Typography variant="caption">
          Behavior nodes assigned by a deterministic labeler stand-in;
          production used a small model.
        </Typography>
      </Box>
    </Paper>
  );
}

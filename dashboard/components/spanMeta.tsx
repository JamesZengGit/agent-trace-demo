'use client';

import type { SvgIconComponent } from '@mui/icons-material';
import PsychologyIcon from '@mui/icons-material/Psychology';
import BuildIcon from '@mui/icons-material/Build';
import StorageIcon from '@mui/icons-material/Storage';
import ChatBubbleOutlineIcon from '@mui/icons-material/ChatBubbleOutline';
import PublicIcon from '@mui/icons-material/Public';
import type { Span, SpanType } from '@/lib/api';
import { ERROR_RED, GOOGLE_BLUE, WARNING_AMBER } from '@/lib/theme';

export const SPAN_META: Record<
  SpanType,
  { label: string; Icon: SvgIconComponent; tint: string }
> = {
  llm_call: { label: 'llm_call', Icon: PsychologyIcon, tint: '#1a73e8' },
  tool_call: { label: 'tool_call', Icon: BuildIcon, tint: '#188038' },
  db_call: { label: 'db_call', Icon: StorageIcon, tint: '#8430ce' },
  output: { label: 'output', Icon: ChatBubbleOutlineIcon, tint: '#e37400' },
  external: { label: 'external', Icon: PublicIcon, tint: '#5f6368' },
};

export function spanMeta(type: SpanType) {
  return SPAN_META[type] ?? SPAN_META.external;
}

/** Bar/status color: error red > warning amber > Google blue. */
export function spanStatusColor(span: Span): string {
  if (span.error) return ERROR_RED;
  if (span.warnings && span.warnings.length > 0) return WARNING_AMBER;
  return GOOGLE_BLUE;
}

// AgentTrace API client — types and fetchers matching docs/CONTRACT.md exactly.

export const API_URL: string =
  process.env.NEXT_PUBLIC_API_URL || 'http://localhost:7000';

/** Derive the live WebSocket URL from the HTTP API URL. */
export function liveWsUrl(): string {
  return API_URL.replace(/^http/, 'ws').replace(/\/$/, '') + '/api/live';
}

// ---------------------------------------------------------------------------
// Data shapes (CONTRACT.md)
// ---------------------------------------------------------------------------

export type TraceStatus = 'running' | 'closed';

export type SpanType = 'llm_call' | 'tool_call' | 'db_call' | 'output' | 'external';

export interface TraceSummary {
  trace_id: string;
  agent_id: string;
  status: TraceStatus;
  start_time: string; // RFC3339
  end_time?: string; // absent while running
  latency_ms: number;
  span_count: number;
  error_count: number;
  warning_count: number;
  total_cost_usd: number;
  purpose: string;
}

export interface SpanWarning {
  source: 'model_checker' | 'policy_engine';
  rule: string;
  reason: string;
}

export type ErrorKind = 'http_status' | 'no_answer' | 'body_failure';

export interface Span {
  span_id: string;
  trace_id: string;
  agent_id: string;
  type: SpanType;
  start_time: string;
  end_time: string;
  client_ip?: string;
  destination: string;
  method?: string;
  status_code?: number;
  dropped?: boolean;
  model?: string;
  system_prompt?: string;
  user_prompt?: string;
  response?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  request_body?: string;
  response_body?: string;
  error?: boolean;
  error_kind?: ErrorKind;
  warnings?: SpanWarning[];
  behavior?: string;
  sub_behavior?: string;
  /** sequence number, when the collector provides one */
  seq?: number;
}

export interface BehaviorNode {
  label: string;
  kind: 'behavior' | 'sub_behavior' | 'span';
  span_id?: string; // only on kind=span
  error?: boolean;
  warning?: boolean;
  children?: BehaviorNode[];
}

export interface Metrics {
  trace_count: number;
  total_trace_count: number;
  avg_latency_ms: number;
  error_count: number;
  warning_count: number;
}

export interface TraceDetail {
  summary: TraceSummary;
  spans: Span[];
  behavior_tree: BehaviorNode;
}

export type LiveEvent =
  | { type: 'trace_upsert'; summary: TraceSummary }
  | { type: 'span'; span: Span };

export interface TopologyAgent {
  id: string;
  calls: number;
}

export interface TopologyBackend {
  id: string;
  label: string;
  kind: SpanType;
}

export interface TopologyEdge {
  agent: string;
  backend: string;
  calls: number;
  errors: number;
  warnings: number;
}

export interface Topology {
  agents: TopologyAgent[];
  backends: TopologyBackend[];
  edges: TopologyEdge[];
}

// ---------------------------------------------------------------------------
// Fetchers
// ---------------------------------------------------------------------------

async function getJson<T>(path: string): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, { cache: 'no-store' });
  if (!res.ok) {
    throw new Error(`API ${res.status} ${res.statusText} for ${path}`);
  }
  return (await res.json()) as T;
}

/** POST helper mirroring getJson. Surfaces the server's {error} message when present. */
async function postJson<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${API_URL}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    cache: 'no-store',
  });
  if (!res.ok) {
    let detail = `${res.status} ${res.statusText}`;
    try {
      const j = (await res.json()) as { error?: string };
      if (j.error) detail = j.error;
    } catch {
      /* non-JSON error body */
    }
    const err = new Error(detail) as Error & { status?: number };
    err.status = res.status;
    throw err;
  }
  return (await res.json()) as T;
}

export function fetchMetrics(from: Date, to: Date): Promise<Metrics> {
  const q = new URLSearchParams({
    from: from.toISOString(),
    to: to.toISOString(),
  });
  return getJson<Metrics>(`/api/metrics?${q}`);
}

export async function fetchTraces(
  from: Date,
  to: Date,
  limit = 1000,
): Promise<TraceSummary[]> {
  const q = new URLSearchParams({
    from: from.toISOString(),
    to: to.toISOString(),
    limit: String(limit),
  });
  const body = await getJson<{ traces: TraceSummary[] | null }>(
    `/api/traces?${q}`,
  );
  return body.traces ?? [];
}

export function fetchTrace(traceId: string): Promise<TraceDetail> {
  return getJson<TraceDetail>(`/api/traces/${encodeURIComponent(traceId)}`);
}

/** Fleet network map (default window: last 24h server-side). */
export function fetchTopology(limitAgents = 0): Promise<Topology> {
  const q = limitAgents > 0 ? `?limit_agents=${limitAgents}` : '';
  return getJson<Topology>(`/api/topology${q}`);
}

// ---------------------------------------------------------------------------
// Chat
// ---------------------------------------------------------------------------

export interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
}

/**
 * Ask the assistant a question about the trace data. The backend runs a
 * tool-calling loop against the real store, so answers are grounded. Send the
 * full turn history (user/assistant); the server prepends its own system
 * prompt. Throws with the server's error message (e.g. chat not configured).
 */
export async function sendChat(messages: ChatMessage[]): Promise<string> {
  const body = await postJson<{ reply: string }>('/api/chat', { messages });
  return body.reply;
}

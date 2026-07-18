# AgentTrace — Component Contract

Single source of truth for ports, HTTP API, and WebSocket protocol.

## Ports (local + docker-compose)

| Service   | Port | Role |
|-----------|------|------|
| proxy     | 8080 | capture proxy — agents' HTTP egress |
| collector | 7100 | span ingest (WebSocket `/ingest` from proxies) |
| api       | 7000 | dashboard backend: HTTP queries + live WS + OTel ingest |
| mocksvc   | 9100 | mock LLM (`/v1/chat/completions`), tools (`/tools/*`), db (`/db/*`), user channel (`/user/deliver`), external sink (`/external/*`) |
| dashboard | 3000 | Next.js UI |
| postgres  | 5432 | storage (db `agenttrace`) |
| nats      | 4222 | transport broker (JetStream) |

## API service (port 7000) — consumed by the dashboard

All responses `application/json`; CORS `*`.

### GET /api/metrics?from=RFC3339&to=RFC3339
`trace_count` is within the window; `total_trace_count` is all-time.
```json
{ "trace_count": 128, "total_trace_count": 4213, "avg_latency_ms": 2144.5,
  "error_count": 7, "warning_count": 3 }
```

### GET /api/traces?from=RFC3339&to=RFC3339&limit=1000
Newest first. Trace summaries in the window (start_time within [from,to]).
```json
{ "traces": [ {
  "trace_id": "tr_...", "agent_id": "support-triage",
  "status": "running" | "closed",
  "start_time": "RFC3339", "end_time": "RFC3339 or absent while running",
  "latency_ms": 3120.4, "span_count": 6,
  "error_count": 0, "warning_count": 1,
  "total_cost_usd": 0.00312, "purpose": "Mission: ..." } ] }
```
Error/warning checkbox filtering happens client-side (demo scale).

### GET /api/traces/{trace_id}
```json
{
  "summary": { ...TraceSummary as above },
  "spans": [ {
    "span_id": "sp_...", "trace_id": "tr_...", "agent_id": "...",
    "type": "llm_call" | "tool_call" | "db_call" | "output" | "external",
    "start_time": "RFC3339", "end_time": "RFC3339",
    "client_ip": "...", "destination": "mocksvc:9100/v1/chat/completions",
    "method": "POST", "status_code": 200,
    "dropped": false,
    "model": "mock-large-1", "system_prompt": "...", "user_prompt": "...",
    "response": "...", "prompt_tokens": 812, "completion_tokens": 214,
    "request_body": "...", "response_body": "...",
    "error": false, "error_kind": "http_status" | "no_answer" | "body_failure",
    "warnings": [ { "source": "model_checker" | "policy_engine",
                    "rule": "instruction_override", "reason": "..." } ],
    "behavior": "Reasoning", "sub_behavior": "LLM consultation"
  } ],
  "behavior_tree": {
    "label": "Mission: ...", "kind": "behavior" | "sub_behavior" | "span",
    "span_id": "only on kind=span", "error": true, "warning": false,
    "children": [ ...recursive ]
  }
}
```

### GET /api/topology?from=RFC3339&to=RFC3339&limit_agents=N
Fleet network map, derived automatically from captured traffic — every agent
observed in the window and every backend it called. Nothing is configured;
the topology is what the proxy saw. Defaults: last 24 h, all agents
(`limit_agents` > 0 keeps only the busiest N).
```json
{
  "agents":   [ { "id": "support-triage", "calls": 142 } ],
  "backends": [ { "id": "llm", "label": "LLM API", "kind": "llm_call" },
                { "id": "tool:web_search", "label": "Tool: web_search", "kind": "tool_call" },
                { "id": "ext:mocksvc:9100", "label": "External: mocksvc:9100", "kind": "external" } ],
  "edges":    [ { "agent": "support-triage", "backend": "llm",
                  "calls": 142, "errors": 3, "warnings": 0 } ]
}
```
An edge means "this agent has called this backend" (static connectivity);
counts power tooltips and error/warning marks. Backends are grouped: one
`llm` node, one `db` node, one `user` node, one node per tool, one per
external host.

### WS /api/live
Server pushes JSON `LiveEvent` frames; client sends nothing (pings ok).
```json
{ "type": "trace_upsert", "summary": { ...TraceSummary } }
{ "type": "span", "span": { ...Span } }
```
`trace_upsert` fires on every span (running trace) and on close (`status:
"closed"`, end_time set). Dashboard rule: running traces appear in the trace
table only; a trace enters the heatmap when it closes and its final latency
is known.

### POST /otel/v1/traces
OTLP/HTTP JSON ingest adapter (labeled improvement, optional inlet).

### POST /api/chat
Natural-language questions about the trace data. The backend runs a
tool-calling loop against an OpenAI-compatible model: the model calls read
tools (`get_metrics`, `list_traces`, `get_trace`, `get_topology`,
`search_spans`) that map to the store, so every answer is grounded in a real
query — nothing is invented. Send the full turn history; the server prepends
its own system prompt.
```json
// request
{ "messages": [ { "role": "user" | "assistant", "content": "..." } ] }
// response
{ "reply": "inventory-sync had 5 warnings; on trace tr_… it followed a prompt injection…" }
```
Returns **503** `{"error":"chat is not configured…"}` when `OPENAI_API_KEY` is
unset, and **502** if the model call fails. Enabled by three env vars on the
api service:
- `OPENAI_API_KEY` — required to enable chat (server-side only; never sent to the browser).
- `OPENAI_BASE_URL` — default `https://api.openai.com/v1`. Point at a
  self-hosted vLLM server (`http://vllm:8000/v1`) to switch providers with no
  code change — same chat-completions + tools protocol.
- `OPENAI_MODEL` — default `gpt-4o-mini`.

## Dashboard realtime model (confirmed)

Hybrid: HTTP loads the focus window (re-query on every window/filter change);
the WebSocket carries only the live edge. Heatmap window and trace table
always change together. In live mode the X axis tracks "now"; a focused past
window does not move when data arrives.

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"agenttrace/internal/model"
	"agenttrace/internal/store"
)

// tools is the fixed menu handed to the model on every call. Each maps to a
// read method on the store. Parameters are described so the model fills them
// correctly; the "since" convention is shared across tools.
var tools = []toolDef{
	{Type: "function", Function: functionDef{
		Name:        "get_metrics",
		Description: "Aggregate counts over a time window: number of traces in the window, all-time total traces, average latency (ms), and how many traces had errors or warnings.",
		Parameters: schema(map[string]any{
			"since": sinceProp,
		}, nil),
	}},
	{Type: "function", Function: functionDef{
		Name:        "list_traces",
		Description: "List trace summaries (newest first) in a time window, optionally filtered. Use this to find which agents ran, which had errors/warnings, and latency/cost per trace. Returns trace_id, agent_id, status, times, latency_ms, span_count, error_count, warning_count, cost, purpose.",
		Parameters: schema(map[string]any{
			"since":         sinceProp,
			"only_errors":   map[string]any{"type": "boolean", "description": "keep only traces with at least one error"},
			"only_warnings": map[string]any{"type": "boolean", "description": "keep only traces with at least one warning (prompt injection, data leakage, or policy violation)"},
			"agent_id":      map[string]any{"type": "string", "description": "restrict to one agent by exact id"},
			"limit":         map[string]any{"type": "integer", "description": "max rows (default 50, max 200)"},
		}, nil),
	}},
	{Type: "function", Function: functionDef{
		Name:        "get_trace",
		Description: "Fetch one full trace by id: its summary plus every captured span in order. Span payloads include the captured LLM system_prompt, user_prompt, and response — use this to see exactly what an agent did or what a prompt contained.",
		Parameters: schema(map[string]any{
			"trace_id": map[string]any{"type": "string", "description": "the trace id, e.g. tr_20260717T210650-5b95fbead04c"},
		}, []string{"trace_id"}),
	}},
	{Type: "function", Function: functionDef{
		Name:        "get_topology",
		Description: "The fleet network map derived from captured traffic: which agents call which backends (LLM, database, tools, external hosts), with per-edge call/error/warning counts. Use this for 'who talks to what' questions.",
		Parameters: schema(map[string]any{
			"since":        sinceProp,
			"limit_agents": map[string]any{"type": "integer", "description": "keep only the busiest N agents"},
		}, nil),
	}},
	{Type: "function", Function: functionDef{
		Name:        "search_spans",
		Description: "Keyword search across captured span content (system/user prompts, responses, request/response bodies, destinations) in a time window. Use this to find traces whose content mentions a word or phrase, e.g. 'refund', 'exfil', a hostname, or an instruction.",
		Parameters: schema(map[string]any{
			"keyword": map[string]any{"type": "string", "description": "text to search for (case-insensitive substring)"},
			"since":   sinceProp,
			"limit":   map[string]any{"type": "integer", "description": "max matches (default 20, max 100)"},
		}, []string{"keyword"}),
	}},
}

var sinceProp = map[string]any{
	"type":        "string",
	"description": "relative time window ending now, e.g. '15m', '1h', '24h', '7d'. Omit for the default 24h.",
}

// schema builds a minimal JSON Schema object for a tool's parameters.
func schema(props map[string]any, required []string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

// dispatch runs one tool call and returns a JSON string for the tool message.
// Errors are returned as JSON too, so the model can read and recover from them
// rather than the whole request failing.
func (e *Engine) dispatch(ctx context.Context, name, arguments string) string {
	var args map[string]any
	if arguments != "" {
		if err := json.Unmarshal([]byte(arguments), &args); err != nil {
			return toolError("could not parse arguments: %v", err)
		}
	}

	switch name {
	case "get_metrics":
		from, to := window(args)
		m, err := e.src.QueryMetrics(ctx, from, to)
		if err != nil {
			return toolError("query failed: %v", err)
		}
		return toJSON(m)

	case "list_traces":
		from, to := window(args)
		f := store.TraceFilter{
			From:         from,
			To:           to,
			OnlyErrors:   boolArg(args, "only_errors"),
			OnlyWarnings: boolArg(args, "only_warnings"),
			AgentID:      strArg(args, "agent_id"),
			Limit:        clampInt(intArg(args, "limit", 50), 1, 200),
		}
		rows, err := e.src.QueryTraces(ctx, f)
		if err != nil {
			return toolError("query failed: %v", err)
		}
		return toJSON(map[string]any{"count": len(rows), "traces": rows})

	case "get_trace":
		id := strArg(args, "trace_id")
		if id == "" {
			return toolError("trace_id is required")
		}
		summary, spans, tree, err := e.src.GetTrace(ctx, id)
		if err != nil {
			return toolError("trace not found: %v", err)
		}
		return toJSON(map[string]any{
			"summary":       summary,
			"spans":         trimSpans(spans),
			"behavior_tree": tree,
		})

	case "get_topology":
		from, to := window(args)
		topo, err := e.src.QueryTopology(ctx, from, to, intArg(args, "limit_agents", 0))
		if err != nil {
			return toolError("query failed: %v", err)
		}
		return toJSON(topo)

	case "search_spans":
		kw := strArg(args, "keyword")
		if kw == "" {
			return toolError("keyword is required")
		}
		from, to := window(args)
		matches, err := e.src.SearchSpans(ctx, kw, from, to, clampInt(intArg(args, "limit", 20), 1, 100))
		if err != nil {
			return toolError("search failed: %v", err)
		}
		return toJSON(map[string]any{"count": len(matches), "matches": matches})

	default:
		return toolError("unknown tool %q", name)
	}
}

// ---- argument helpers (the model sends loosely-typed JSON) ----

func strArg(args map[string]any, key string) string {
	if v, ok := args[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func boolArg(args map[string]any, key string) bool {
	switch v := args[key].(type) {
	case bool:
		return v
	case string:
		return v == "true"
	}
	return false
}

func intArg(args map[string]any, key string, def int) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func clampInt(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// window turns a "since" string into a [from, to=now] range. Supports Go
// durations (m, h) plus a "d" (days) suffix. Defaults to 24h.
func window(args map[string]any) (time.Time, time.Time) {
	to := time.Now()
	from := to.Add(-parseSince(strArg(args, "since")))
	return from, to
}

func parseSince(s string) time.Duration {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 24 * time.Hour
	}
	if strings.HasSuffix(s, "d") {
		if n, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil && n > 0 {
			return time.Duration(n) * 24 * time.Hour
		}
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return d
	}
	return 24 * time.Hour
}

// trimSpans caps captured payload fields so a big trace doesn't blow the token
// budget. The interesting content (prompts, responses) stays, just bounded.
func trimSpans(spans []*model.Span) []*model.Span {
	const limit = 800
	out := make([]*model.Span, len(spans))
	for i, s := range spans {
		c := *s
		c.SystemPrompt = truncate(c.SystemPrompt, limit)
		c.UserPrompt = truncate(c.UserPrompt, limit)
		c.Response = truncate(c.Response, limit)
		c.RequestBody = truncate(c.RequestBody, limit)
		c.ResponseBody = truncate(c.ResponseBody, limit)
		out[i] = &c
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…[truncated]"
	}
	return s
}

func toJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError("could not encode result: %v", err)
	}
	return string(b)
}

func toolError(format string, args ...any) string {
	return fmt.Sprintf(`{"error":%q}`, fmt.Sprintf(format, args...))
}

// Package store is the PostgreSQL persistence layer. Two normalized tables,
// designed for access patterns rather than data shape:
//
//   - trace_summary — composite PK (trace_id, agent_id). Serves the dashboard
//     and heatmap queries. Agents are an activity context, not a queryable
//     unit, so the agent rides in the key rather than owning a table.
//   - trace_detail — EAV rows (detail_name / detail_value per span). Serves
//     full drilldowns; new detail types need no migration.
//
// The original JSONB approach was abandoned: nested-field queries were slow
// and storage costs scaled faster than servers could be provisioned.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"agenttrace/internal/model"
)

type Store struct {
	db *sql.DB
}

func Open(dsn string) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(16)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Migrate creates the two tables. Idempotent; the advisory lock serializes
// services that boot at the same time (processor and api both migrate).
func (s *Store) Migrate(ctx context.Context) error {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock(752026)`); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, `SELECT pg_advisory_unlock_all()`)
	_, err = conn.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS trace_summary (
    trace_id      TEXT        NOT NULL,
    agent_id      TEXT        NOT NULL,
    status        TEXT        NOT NULL,
    start_time    TIMESTAMPTZ NOT NULL,
    end_time      TIMESTAMPTZ,
    latency_ms    DOUBLE PRECISION NOT NULL DEFAULT 0,
    span_count    INT         NOT NULL DEFAULT 0,
    error_count   INT         NOT NULL DEFAULT 0,
    warning_count INT         NOT NULL DEFAULT 0,
    total_cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
    purpose       TEXT        NOT NULL DEFAULT '',
    PRIMARY KEY (trace_id, agent_id)
);
CREATE INDEX IF NOT EXISTS idx_summary_start ON trace_summary (start_time);
CREATE INDEX IF NOT EXISTS idx_summary_agent ON trace_summary (agent_id, start_time);

CREATE TABLE IF NOT EXISTS trace_detail (
    id           BIGSERIAL PRIMARY KEY,
    trace_id     TEXT NOT NULL,
    agent_id     TEXT NOT NULL,
    span_id      TEXT NOT NULL,
    detail_name  TEXT NOT NULL,
    detail_value TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_detail_trace ON trace_detail (trace_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_detail_unique ON trace_detail (span_id, detail_name);
`)
	return err
}

// UpsertSummary writes a summary row (running traces update in place).
func (s *Store) UpsertSummary(ctx context.Context, ts *model.TraceSummary) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO trace_summary
  (trace_id, agent_id, status, start_time, end_time, latency_ms,
   span_count, error_count, warning_count, total_cost_usd, purpose)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
ON CONFLICT (trace_id, agent_id) DO UPDATE SET
  status=$3, end_time=$5, latency_ms=$6, span_count=$7,
  error_count=$8, warning_count=$9, total_cost_usd=$10, purpose=$11`,
		ts.TraceID, ts.AgentID, ts.Status, ts.StartTime, ts.EndTime, ts.LatencyMS,
		ts.SpanCount, ts.ErrorCount, ts.WarningCount, ts.TotalCostUSD, ts.Purpose)
	return err
}

// spanDetails flattens a span into EAV rows. Payload fields become their own
// details; adding a new capture field later needs no schema change.
func spanDetails(sp *model.Span) map[string]string {
	d := map[string]string{
		"type":        string(sp.Type),
		"sequence":    strconv.FormatUint(sp.Sequence, 10),
		"start_time":  sp.StartTime.Format(time.RFC3339Nano),
		"end_time":    sp.EndTime.Format(time.RFC3339Nano),
		"client_ip":   sp.ClientIP,
		"destination": sp.Destination,
		"method":      sp.Method,
		"status_code": strconv.Itoa(sp.StatusCode),
		"error":       strconv.FormatBool(sp.Error),
	}
	set := func(k, v string) {
		if v != "" {
			d[k] = v
		}
	}
	set("error_kind", string(sp.ErrorKind))
	set("model", sp.Model)
	set("system_prompt", sp.SystemPrompt)
	set("user_prompt", sp.UserPrompt)
	set("response", sp.Response)
	set("request_body", sp.RequestBody)
	set("response_body", sp.ResponseBody)
	set("behavior", sp.Behavior)
	set("sub_behavior", sp.SubBehavior)
	if sp.Dropped {
		d["dropped"] = "true"
	}
	if sp.PromptTokens > 0 {
		d["prompt_tokens"] = strconv.Itoa(sp.PromptTokens)
		d["completion_tokens"] = strconv.Itoa(sp.CompletionTokens)
	}
	if len(sp.Warnings) > 0 {
		b, _ := json.Marshal(sp.Warnings)
		d["warnings"] = string(b)
	}
	return d
}

// InsertSpan writes one span as EAV detail rows in a single batch statement.
func (s *Store) InsertSpan(ctx context.Context, sp *model.Span) error {
	details := spanDetails(sp)
	names := make([]string, 0, len(details))
	values := make([]string, 0, len(details))
	for k, v := range details {
		names = append(names, k)
		values = append(values, v)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO trace_detail (trace_id, agent_id, span_id, detail_name, detail_value)
SELECT $1, $2, $3, n, v FROM unnest($4::text[], $5::text[]) AS t(n, v)
ON CONFLICT (span_id, detail_name) DO UPDATE SET detail_value = EXCLUDED.detail_value`,
		sp.TraceID, sp.AgentID, sp.SpanID, names, values)
	return err
}

// UpdateSpanBehavior writes behavior labels assigned at trace close.
func (s *Store) UpdateSpanBehavior(ctx context.Context, sp *model.Span) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO trace_detail (trace_id, agent_id, span_id, detail_name, detail_value)
VALUES ($1,$2,$3,'behavior',$4), ($1,$2,$3,'sub_behavior',$5)
ON CONFLICT (span_id, detail_name) DO UPDATE SET detail_value = EXCLUDED.detail_value`,
		sp.TraceID, sp.AgentID, sp.SpanID, sp.Behavior, sp.SubBehavior)
	return err
}

// SaveBehaviorTree stores the decision tree once, keyed to the trace.
func (s *Store) SaveBehaviorTree(ctx context.Context, traceID, agentID string, tree *model.BehaviorNode) error {
	b, err := json.Marshal(tree)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO trace_detail (trace_id, agent_id, span_id, detail_name, detail_value)
VALUES ($1,$2,$3,'behavior_tree',$4)
ON CONFLICT (span_id, detail_name) DO UPDATE SET detail_value = EXCLUDED.detail_value`,
		traceID, agentID, "trace:"+traceID, string(b))
	return err
}

// TraceFilter narrows summary queries. Zero values mean "no constraint".
type TraceFilter struct {
	From, To     time.Time
	OnlyErrors   bool
	OnlyWarnings bool
	AgentID      string
	Limit        int
}

// QueryTraces returns summary rows for the dashboard window, newest first.
func (s *Store) QueryTraces(ctx context.Context, f TraceFilter) ([]*model.TraceSummary, error) {
	q := `SELECT trace_id, agent_id, status, start_time, end_time, latency_ms,
       span_count, error_count, warning_count, total_cost_usd, purpose
       FROM trace_summary WHERE start_time >= $1 AND start_time <= $2`
	args := []any{f.From, f.To}
	if f.OnlyErrors && f.OnlyWarnings {
		q += ` AND (error_count > 0 OR warning_count > 0)`
	} else if f.OnlyErrors {
		q += ` AND error_count > 0`
	} else if f.OnlyWarnings {
		q += ` AND warning_count > 0`
	}
	if f.AgentID != "" {
		args = append(args, f.AgentID)
		q += fmt.Sprintf(` AND agent_id = $%d`, len(args))
	}
	limit := f.Limit
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	args = append(args, limit)
	q += fmt.Sprintf(` ORDER BY start_time DESC LIMIT $%d`, len(args))

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.TraceSummary
	for rows.Next() {
		var t model.TraceSummary
		var end sql.NullTime
		if err := rows.Scan(&t.TraceID, &t.AgentID, &t.Status, &t.StartTime, &end,
			&t.LatencyMS, &t.SpanCount, &t.ErrorCount, &t.WarningCount,
			&t.TotalCostUSD, &t.Purpose); err != nil {
			return nil, err
		}
		if end.Valid {
			t.EndTime = &end.Time
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// Metrics is the heatmap page's top bar: trace counts (focus window and
// all-time), average latency, errors.
type Metrics struct {
	TraceCount      int     `json:"trace_count"` // within the queried window
	TotalTraceCount int     `json:"total_trace_count"`
	AvgLatencyMS    float64 `json:"avg_latency_ms"`
	ErrorCount      int     `json:"error_count"`
	WarningCount    int     `json:"warning_count"`
}

func (s *Store) QueryMetrics(ctx context.Context, from, to time.Time) (*Metrics, error) {
	var m Metrics
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*),
       COALESCE(AVG(latency_ms) FILTER (WHERE status = 'closed'), 0),
       COUNT(*) FILTER (WHERE error_count > 0),
       COUNT(*) FILTER (WHERE warning_count > 0),
       (SELECT COUNT(*) FROM trace_summary)
FROM trace_summary WHERE start_time >= $1 AND start_time <= $2`,
		from, to).Scan(&m.TraceCount, &m.AvgLatencyMS, &m.ErrorCount, &m.WarningCount,
		&m.TotalTraceCount)
	return &m, err
}

// GetTrace reconstructs one full trace from the EAV detail table: summary,
// ordered spans, and the stored behavior tree.
func (s *Store) GetTrace(ctx context.Context, traceID string) (*model.TraceSummary, []*model.Span, *model.BehaviorNode, error) {
	var t model.TraceSummary
	var end sql.NullTime
	err := s.db.QueryRowContext(ctx, `
SELECT trace_id, agent_id, status, start_time, end_time, latency_ms,
       span_count, error_count, warning_count, total_cost_usd, purpose
FROM trace_summary WHERE trace_id = $1`, traceID).
		Scan(&t.TraceID, &t.AgentID, &t.Status, &t.StartTime, &end, &t.LatencyMS,
			&t.SpanCount, &t.ErrorCount, &t.WarningCount, &t.TotalCostUSD, &t.Purpose)
	if err != nil {
		return nil, nil, nil, err
	}
	if end.Valid {
		t.EndTime = &end.Time
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT span_id, detail_name, detail_value FROM trace_detail
WHERE trace_id = $1 ORDER BY id`, traceID)
	if err != nil {
		return nil, nil, nil, err
	}
	defer rows.Close()

	spanKV := map[string]map[string]string{}
	var tree *model.BehaviorNode
	for rows.Next() {
		var spanID, name, value string
		if err := rows.Scan(&spanID, &name, &value); err != nil {
			return nil, nil, nil, err
		}
		if name == "behavior_tree" {
			tree = &model.BehaviorNode{}
			_ = json.Unmarshal([]byte(value), tree)
			continue
		}
		if spanKV[spanID] == nil {
			spanKV[spanID] = map[string]string{}
		}
		spanKV[spanID][name] = value
	}
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}

	spans := make([]*model.Span, 0, len(spanKV))
	for id, kv := range spanKV {
		spans = append(spans, spanFromDetails(&t, id, kv))
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].StartTime.Before(spans[j].StartTime) })
	return &t, spans, tree, nil
}

func spanFromDetails(t *model.TraceSummary, spanID string, kv map[string]string) *model.Span {
	sp := &model.Span{SpanID: spanID, TraceID: t.TraceID, AgentID: t.AgentID}
	sp.Type = model.SpanType(kv["type"])
	sp.StartTime, _ = time.Parse(time.RFC3339Nano, kv["start_time"])
	sp.EndTime, _ = time.Parse(time.RFC3339Nano, kv["end_time"])
	sp.ClientIP = kv["client_ip"]
	sp.Destination = kv["destination"]
	sp.Method = kv["method"]
	sp.StatusCode, _ = strconv.Atoi(kv["status_code"])
	sp.Error = kv["error"] == "true"
	sp.ErrorKind = model.ErrorKind(kv["error_kind"])
	sp.Dropped = kv["dropped"] == "true"
	sp.Model = kv["model"]
	sp.SystemPrompt = kv["system_prompt"]
	sp.UserPrompt = kv["user_prompt"]
	sp.Response = kv["response"]
	sp.RequestBody = kv["request_body"]
	sp.ResponseBody = kv["response_body"]
	sp.Behavior = kv["behavior"]
	sp.SubBehavior = kv["sub_behavior"]
	sp.PromptTokens, _ = strconv.Atoi(kv["prompt_tokens"])
	sp.CompletionTokens, _ = strconv.Atoi(kv["completion_tokens"])
	sp.Sequence, _ = strconv.ParseUint(kv["sequence"], 10, 64)
	if w := kv["warnings"]; w != "" {
		_ = json.Unmarshal([]byte(w), &sp.Warnings)
	}
	return sp
}

// CountSpans reports stored span rows — used by bench and chaos scripts.
func (s *Store) CountSpans(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT span_id) FROM trace_detail WHERE span_id NOT LIKE 'trace:%'`).Scan(&n)
	return n, err
}

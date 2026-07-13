// Package model defines the span and trace-mission data model shared by all
// AgentTrace components. Spans are the leaves and branches; the trace mission
// is the trunk.
package model

import (
	"time"
)

// SpanType distinguishes calls from outputs (confirmed span taxonomy).
type SpanType string

const (
	SpanLLMCall  SpanType = "llm_call"  // agent -> LLM
	SpanToolCall SpanType = "tool_call" // agent/LLM -> tool
	SpanDBCall   SpanType = "db_call"   // agent/LLM -> database
	SpanOutput   SpanType = "output"    // agent -> user (terminal span of a trace)
	SpanExternal SpanType = "external"  // anything leaving the known destination set
)

// WarningSource identifies which system raised a warning.
type WarningSource string

const (
	WarnModelChecker WarningSource = "model_checker" // prompt injection / data leakage
	WarnPolicyEngine WarningSource = "policy_engine" // access restriction violation
)

// Warning is attached to a span by the model checker or the policy engine.
type Warning struct {
	Source WarningSource `json:"source"`
	Rule   string        `json:"rule"`
	Reason string        `json:"reason"`
}

// ErrorKind classifies why a span is labeled error. Rate limits, slow
// responses and retries are intentionally NOT errors.
type ErrorKind string

const (
	ErrHTTPStatus  ErrorKind = "http_status"  // 4xx/5xx from the called service
	ErrNoAnswer    ErrorKind = "no_answer"    // timeout, refused, dropped mid-response
	ErrBodyFailure ErrorKind = "body_failure" // 200 but body says failure (refusal, overload, cut-off)
)

// Span is the unit of capture. It is emitted by the proxy and enriched by the
// processor (error labels, checker warnings, behavior grouping).
type Span struct {
	SpanID   string   `json:"span_id"`
	AgentID  string   `json:"agent_id"`
	TraceID  string   `json:"trace_id,omitempty"` // assigned by the processor (timestamp correlation)
	Type     SpanType `json:"type"`
	Sequence uint64   `json:"sequence"` // proxy-assigned, for at-least-once dedup

	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	// Network-layer facts, recorded as they crossed the wire.
	ClientIP    string `json:"client_ip"`
	Destination string `json:"destination"` // host+path the agent called
	Method      string `json:"method"`
	StatusCode  int    `json:"status_code"`
	Dropped     bool   `json:"dropped,omitempty"` // no answer: timeout / refused / cut connection

	// LLM payload capture (present on llm_call spans).
	Model            string `json:"model,omitempty"`
	SystemPrompt     string `json:"system_prompt,omitempty"`
	UserPrompt       string `json:"user_prompt,omitempty"`
	Response         string `json:"response,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`

	// Non-LLM payload capture (tools, db, outputs).
	RequestBody  string `json:"request_body,omitempty"`
	ResponseBody string `json:"response_body,omitempty"`

	// Labels. Error labeling happens in the processor; policy warnings are
	// attached at capture time, checker warnings in the processor.
	Error     bool      `json:"error"`
	ErrorKind ErrorKind `json:"error_kind,omitempty"`
	Warnings  []Warning `json:"warnings,omitempty"`

	// Behavior label assigned by the behavior labeler when the trace closes.
	Behavior    string `json:"behavior,omitempty"`
	SubBehavior string `json:"sub_behavior,omitempty"`
}

// DurationMS is the span's wall-clock duration in milliseconds.
func (s *Span) DurationMS() float64 {
	return float64(s.EndTime.Sub(s.StartTime).Microseconds()) / 1000.0
}

// TraceStatus is the lifecycle of a trace mission.
type TraceStatus string

const (
	TraceRunning TraceStatus = "running"
	TraceClosed  TraceStatus = "closed"
)

// TraceSummary is one row of the summary table (composite PK trace_id +
// agent_id). It serves the dashboard and heatmap queries; agents are an
// activity context, not a queryable unit.
type TraceSummary struct {
	TraceID      string      `json:"trace_id"`
	AgentID      string      `json:"agent_id"`
	Status       TraceStatus `json:"status"`
	StartTime    time.Time   `json:"start_time"`
	EndTime      *time.Time  `json:"end_time,omitempty"` // nil while running
	LatencyMS    float64     `json:"latency_ms"`         // end-start once closed
	SpanCount    int         `json:"span_count"`
	ErrorCount   int         `json:"error_count"`
	WarningCount int         `json:"warning_count"`
	TotalCostUSD float64     `json:"total_cost_usd"` // from captured LLM usage logs
	Purpose      string      `json:"purpose"`        // mission purpose (top behavior label)
}

// BehaviorNode is one node of the decision tree: behavior -> sub-behavior ->
// span leaves. A decision tree represents a mission; a mission represents an
// agent behavior.
type BehaviorNode struct {
	Label    string          `json:"label"`
	Kind     string          `json:"kind"` // "behavior" | "sub_behavior" | "span"
	SpanID   string          `json:"span_id,omitempty"`
	Error    bool            `json:"error,omitempty"`
	Warning  bool            `json:"warning,omitempty"`
	Children []*BehaviorNode `json:"children,omitempty"`
}

// Envelope is the wire format between proxy -> collector -> transport ->
// processor. Sequence + ProxyID give the collector at-least-once dedup and
// power the reattachment logic.
type Envelope struct {
	ProxyID string `json:"proxy_id"`
	Span    Span   `json:"span"`
}

// Ack is the collector's acknowledgement to the proxy over the WebSocket.
// The proxy trims its resend buffer up to Sequence.
type Ack struct {
	Sequence uint64 `json:"sequence"`
}

// LiveEvent is what the API service pushes to dashboard WebSocket clients.
type LiveEvent struct {
	Type    string        `json:"type"` // "trace_upsert" | "span"
	Summary *TraceSummary `json:"summary,omitempty"`
	Span    *Span         `json:"span,omitempty"`
}

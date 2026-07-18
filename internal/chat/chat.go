// Package chat answers natural-language questions about captured trace data.
//
// It drives an OpenAI-compatible chat-completions endpoint with tool calling:
// the model decides which query to run, the Go side executes it against the
// real store, and the model answers from the returned rows. Nothing is
// hallucinated from a stuffed context — every number the model reports came
// from a query it asked for.
//
// The endpoint is configured by base URL, so the same code targets OpenAI
// today (https://api.openai.com/v1) and a self-hosted vLLM server tomorrow
// (http://vllm:8000/v1) with no code change — vLLM speaks the same protocol.
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"agenttrace/internal/model"
	"agenttrace/internal/store"
)

// DataSource is the slice of the store the chat tools read. *store.Store
// satisfies it; keeping it an interface makes the tool boundary explicit and
// the package testable.
type DataSource interface {
	QueryTraces(ctx context.Context, f store.TraceFilter) ([]*model.TraceSummary, error)
	QueryMetrics(ctx context.Context, from, to time.Time) (*store.Metrics, error)
	GetTrace(ctx context.Context, traceID string) (*model.TraceSummary, []*model.Span, *model.BehaviorNode, error)
	QueryTopology(ctx context.Context, from, to time.Time, limitAgents int) (*store.Topology, error)
	SearchSpans(ctx context.Context, keyword string, from, to time.Time, limit int) ([]store.SpanMatch, error)
}

type Config struct {
	BaseURL string // e.g. https://api.openai.com/v1  or  http://vllm:8000/v1
	APIKey  string
	Model   string
}

type Engine struct {
	cfg    Config
	src    DataSource
	client *http.Client
}

func New(cfg Config, src DataSource) *Engine {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com/v1"
	}
	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "/")
	if cfg.Model == "" {
		cfg.Model = "gpt-4o-mini"
	}
	return &Engine{
		cfg: cfg,
		src: src,
		// Long-lived client with an explicit timeout, matching the proxy/fleet
		// convention. A tool loop can take a few seconds; 60s is generous.
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// ---- OpenAI-compatible wire types (chat completions + tools) ----

// Message is one turn. Roles: system | user | assistant | tool.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type toolDef struct {
	Type     string      `json:"type"`
	Function functionDef `json:"function"`
}

type functionDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Tools       []toolDef `json:"tools,omitempty"`
	Temperature float64   `json:"temperature"`
}

type chatResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

const maxToolRounds = 6 // guard against a model that loops on tool calls

// Answer runs the tool-calling loop and returns the assistant's final text.
// history is the prior conversation (user/assistant turns) — the caller does
// not include the system prompt; Answer prepends it.
func (e *Engine) Answer(ctx context.Context, history []Message) (string, error) {
	messages := make([]Message, 0, len(history)+1)
	messages = append(messages, Message{Role: "system", Content: e.systemPrompt()})
	messages = append(messages, history...)

	for round := 0; round < maxToolRounds; round++ {
		resp, err := e.complete(ctx, messages)
		if err != nil {
			return "", err
		}
		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("model returned no choices")
		}
		msg := resp.Choices[0].Message

		if len(msg.ToolCalls) == 0 {
			return strings.TrimSpace(msg.Content), nil
		}

		// Append the assistant's tool-call message, then one tool result per call.
		messages = append(messages, msg)
		for _, tc := range msg.ToolCalls {
			result := e.dispatch(ctx, tc.Function.Name, tc.Function.Arguments)
			messages = append(messages, Message{
				Role:       "tool",
				ToolCallID: tc.ID,
				Name:       tc.Function.Name,
				Content:    result,
			})
		}
	}
	return "", fmt.Errorf("chat exceeded %d tool rounds without a final answer", maxToolRounds)
}

// complete makes one chat-completions call with the tool definitions attached.
func (e *Engine) complete(ctx context.Context, messages []Message) (*chatResponse, error) {
	body, _ := json.Marshal(chatRequest{
		Model:       e.cfg.Model,
		Messages:    messages,
		Tools:       tools,
		Temperature: 0.1, // factual, low variance
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.cfg.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.cfg.APIKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llm request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	var out chatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("llm returned unparseable response (status %d): %s",
			resp.StatusCode, snippet(raw))
	}
	if out.Error != nil {
		return nil, fmt.Errorf("llm error: %s", out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llm status %d: %s", resp.StatusCode, snippet(raw))
	}
	return &out, nil
}

func (e *Engine) systemPrompt() string {
	now := time.Now().UTC().Format(time.RFC3339)
	return `You are the AgentTrace assistant. You answer questions about AI-agent
network activity captured by AgentTrace — a network-layer observability system.

Data model you can reason about:
- A trace (a "mission") is one agent's burst of activity, assembled by timestamp.
  It has: agent_id, status (running|closed), start/end time, latency_ms,
  span_count, error_count, warning_count, total_cost_usd, and a purpose label.
- A span is one captured network call. Types: llm_call, tool_call, db_call,
  output, external. Spans carry destination, status_code, timing, and — for
  llm_call spans — the captured system_prompt, user_prompt, and response.
- Errors are labeled from observed traffic: http_status (4xx/5xx), no_answer
  (timeout/refused/dropped), body_failure (200 but the body signals failure).
  Rate limits, slowness, and retries are NOT errors.
- Warnings come from two sources: model_checker (prompt injection / data
  leakage detected in LLM messages) and policy_engine (an agent called a
  destination it is not allowed to).
- Topology is the fleet map of which agents call which backends, derived
  entirely from captured traffic.

Use the tools to fetch real data before answering — never invent numbers,
agent names, trace IDs, or timestamps. If a tool returns nothing, say so
plainly. Current time is ` + now + ` (UTC). When the user gives a relative
time ("today", "last hour"), pass the matching "since" value to the tools.
Keep answers concise and concrete; cite specific agent IDs, counts, and trace
IDs from the tool results. If asked something the data cannot answer, say what
is and isn't captured.`
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

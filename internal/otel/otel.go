// Package otel is the OpenTelemetry ingest adapter — a labeled 2026
// improvement, not part of the original system. It is an optional on-ramp
// for teams already emitting OTLP; the native path (proxy capture, zero code
// changes) stays the headline. OTLP/HTTP JSON in, native spans out.
package otel

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"agenttrace/internal/model"
)

// Minimal OTLP/HTTP JSON shapes (trace signal only, string-keyed attributes).
type otlpPayload struct {
	ResourceSpans []struct {
		Resource struct {
			Attributes []kv `json:"attributes"`
		} `json:"resource"`
		ScopeSpans []struct {
			Spans []otlpSpan `json:"spans"`
		} `json:"scopeSpans"`
	} `json:"resourceSpans"`
}

type kv struct {
	Key   string `json:"key"`
	Value struct {
		StringValue string  `json:"stringValue,omitempty"`
		IntValue    string  `json:"intValue,omitempty"`
		DoubleValue float64 `json:"doubleValue,omitempty"`
	} `json:"value"`
}

type otlpSpan struct {
	SpanID            string `json:"spanId"`
	Name              string `json:"name"`
	StartTimeUnixNano string `json:"startTimeUnixNano"`
	EndTimeUnixNano   string `json:"endTimeUnixNano"`
	Attributes        []kv   `json:"attributes"`
	Status            struct {
		Code int `json:"code"` // 2 = ERROR
	} `json:"status"`
}

func attrs(list []kv) map[string]string {
	m := map[string]string{}
	for _, a := range list {
		switch {
		case a.Value.StringValue != "":
			m[a.Key] = a.Value.StringValue
		case a.Value.IntValue != "":
			m[a.Key] = a.Value.IntValue
		default:
			m[a.Key] = strconv.FormatFloat(a.Value.DoubleValue, 'f', -1, 64)
		}
	}
	return m
}

func unixNano(s string) time.Time {
	n, _ := strconv.ParseInt(s, 10, 64)
	return time.Unix(0, n)
}

// Convert maps an OTLP export request onto native spans. GenAI semantic
// conventions (gen_ai.*) are honored where present; the service.name resource
// attribute becomes the agent ID.
func Convert(r io.Reader) ([]*model.Span, error) {
	var p otlpPayload
	if err := json.NewDecoder(io.LimitReader(r, 8<<20)).Decode(&p); err != nil {
		return nil, fmt.Errorf("invalid OTLP JSON: %w", err)
	}
	var out []*model.Span
	for _, rs := range p.ResourceSpans {
		res := attrs(rs.Resource.Attributes)
		agent := res["service.name"]
		if agent == "" {
			agent = "otel-unattributed"
		}
		for _, ss := range rs.ScopeSpans {
			for _, os := range ss.Spans {
				a := attrs(os.Attributes)
				sp := &model.Span{
					SpanID:      "otel_" + os.SpanID,
					AgentID:     agent,
					StartTime:   unixNano(os.StartTimeUnixNano),
					EndTime:     unixNano(os.EndTimeUnixNano),
					Method:      orDefault(a["http.request.method"], "POST"),
					Destination: orDefault(a["server.address"]+a["url.path"], os.Name),
					Model:       a["gen_ai.request.model"],
					UserPrompt:  a["gen_ai.prompt"],
					Response:    a["gen_ai.completion"],
				}
				if v, err := strconv.Atoi(a["http.response.status_code"]); err == nil {
					sp.StatusCode = v
				} else if os.Status.Code != 2 {
					sp.StatusCode = 200
				}
				if v, err := strconv.Atoi(a["gen_ai.usage.input_tokens"]); err == nil {
					sp.PromptTokens = v
				}
				if v, err := strconv.Atoi(a["gen_ai.usage.output_tokens"]); err == nil {
					sp.CompletionTokens = v
				}
				if sp.Model != "" {
					sp.Type = model.SpanLLMCall
				} else {
					sp.Type = model.SpanToolCall
				}
				if os.Status.Code == 2 && sp.StatusCode < 400 {
					sp.StatusCode = 500
				}
				out = append(out, sp)
			}
		}
	}
	return out, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

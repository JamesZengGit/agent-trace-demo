// proxy is the capture layer: a forward HTTP proxy the agents' egress traffic
// flows through (standard HTTP_PROXY configuration — zero agent code
// changes). It records what actually crossed the wire, injects trace-context
// headers into requests as they pass through, evaluates the policy engine on
// every destination, and ships spans to the collector over a WebSocket with
// sequence-numbered acknowledgements and reattachment on reconnect.
//
// REPLICA NOTICE: production used Rust + Envoy with transparent TLS
// interception. This replica implements the same capture logic in Go over
// plain HTTP — the mechanism (interception, header injection, timing, body
// capture) is the point, not the TLS plumbing.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"agenttrace/internal/model"
	"agenttrace/internal/policy"
)

const maxCapturedBody = 8 << 10 // capture cap per payload field

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

// ---------- span shipping: WS to collector, ack-trimmed resend buffer ----------

// shipper maintains the WebSocket to the collector. Every envelope carries a
// sequence number; the collector acks what it has accepted, and the shipper
// keeps everything unacked in a resend buffer. On reconnect the buffer is
// replayed — this is the fix for the orphan incident. AT_REATTACH=off
// reproduces the original bug: the buffer is dropped when the connection dies.
type shipper struct {
	proxyID      string
	collectorURL string
	reattach     bool

	mu      sync.Mutex
	pending []*model.Envelope // unacked, oldest first
	notify  chan struct{}

	connected atomic.Bool
	seq       atomic.Uint64
	sent      atomic.Uint64
	dropped   atomic.Uint64
}

func newShipper(collectorURL, proxyID string, reattach bool) *shipper {
	return &shipper{
		proxyID:      proxyID,
		collectorURL: collectorURL,
		reattach:     reattach,
		notify:       make(chan struct{}, 1),
	}
}

func (sh *shipper) enqueue(sp model.Span) {
	// Original-bug mode: the proxy does not detect the dead destination —
	// spans emitted while the connection is down go into the dead pipe and
	// are gone, exactly like records shipped to a reassigned pod:port.
	if !sh.reattach && !sh.connected.Load() {
		sh.dropped.Add(1)
		return
	}
	sh.mu.Lock()
	// Sequence assignment and append happen under one lock so the pending
	// buffer is always sorted by sequence. Assigning the sequence outside
	// the lock lets two concurrent requests append out of order — the writer
	// skips the lower one (cursor already passed it) and the ack for the
	// higher one trims it unwritten. Found by `make bench` accounting: ~0.1%
	// of spans vanished at high concurrency.
	sp.Sequence = sh.seq.Add(1)
	sh.pending = append(sh.pending, &model.Envelope{ProxyID: sh.proxyID, Span: sp})
	if len(sh.pending) > 100000 { // hard cap so a dead collector can't OOM the proxy
		sh.pending = sh.pending[len(sh.pending)-100000:]
	}
	sh.mu.Unlock()
	select {
	case sh.notify <- struct{}{}:
	default:
	}
}

func (sh *shipper) run(ctx context.Context) {
	for ctx.Err() == nil {
		conn, _, err := websocket.DefaultDialer.DialContext(ctx, sh.collectorURL, nil)
		if err != nil {
			log.Printf("shipper: collector dial failed: %v (retrying)", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		log.Printf("shipper: connected to collector")
		sh.connected.Store(true)
		sh.session(ctx, conn)
		sh.connected.Store(false)
		conn.Close()
		if !sh.reattach {
			// Original-bug mode: the in-flight buffer is stranded with the
			// dead connection, exactly like agents pinned to a pod:port.
			sh.mu.Lock()
			n := len(sh.pending)
			sh.pending = nil
			sh.mu.Unlock()
			sh.dropped.Add(uint64(n))
			if n > 0 {
				log.Printf("shipper: REATTACH OFF — stranded %d unacked spans", n)
			}
		} else {
			sh.mu.Lock()
			n := len(sh.pending)
			sh.mu.Unlock()
			if n > 0 {
				log.Printf("shipper: connection lost, %d unacked spans held for reattach", n)
			}
		}
	}
}

// session pumps pending envelopes out and acks in, until the socket dies.
func (sh *shipper) session(ctx context.Context, conn *websocket.Conn) {
	done := make(chan struct{})
	// ack reader: trim everything up to the acked sequence.
	go func() {
		defer close(done)
		for {
			var ack model.Ack
			if err := conn.ReadJSON(&ack); err != nil {
				return
			}
			sh.mu.Lock()
			i := 0
			for i < len(sh.pending) && sh.pending[i].Span.Sequence <= ack.Sequence {
				i++
			}
			sh.pending = sh.pending[i:]
			sh.mu.Unlock()
		}
	}()

	cursor := uint64(0) // highest sequence written this session
	for {
		sh.mu.Lock()
		var batch []*model.Envelope
		for _, e := range sh.pending {
			if e.Span.Sequence > cursor {
				batch = append(batch, e)
			}
		}
		sh.mu.Unlock()
		for _, e := range batch {
			if err := conn.WriteJSON(e); err != nil {
				return
			}
			cursor = e.Span.Sequence
			sh.sent.Add(1)
		}
		select {
		case <-ctx.Done():
			return
		case <-done: // read side died -> reconnect (resend from trimmed buffer)
			return
		case <-sh.notify:
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// ---------- the forward proxy ----------

type captureProxy struct {
	ship    *shipper
	pol     *policy.Engine
	client  *http.Client
	served  atomic.Uint64
	baseMap map[string]string // path-prefix routing for agents using base-URL mode
}

// classify maps a destination to the span taxonomy: calls and outputs.
func classify(dest string) model.SpanType {
	switch {
	case strings.Contains(dest, "/v1/chat/completions"), strings.Contains(dest, "/v1/messages"):
		return model.SpanLLMCall
	case strings.Contains(dest, "/tools/"):
		return model.SpanToolCall
	case strings.Contains(dest, "/db/"):
		return model.SpanDBCall
	case strings.Contains(dest, "/user/"):
		return model.SpanOutput
	default:
		return model.SpanExternal
	}
}

func (p *captureProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" && !r.URL.IsAbs() {
		writeJSON(w, map[string]any{"ok": true, "served": p.served.Load(), "shipped": p.sentStats()})
		return
	}
	if r.Method == http.MethodConnect {
		http.Error(w, "CONNECT (TLS interception) not implemented in replica; production Envoy terminated TLS", http.StatusMethodNotAllowed)
		return
	}

	// Absolute-form URL = standard HTTP_PROXY egress. Otherwise fall back to
	// base-URL routing (agents pointed straight at the proxy).
	outURL := r.URL.String()
	if !r.URL.IsAbs() {
		upstream := os.Getenv("AT_DEFAULT_UPSTREAM")
		if upstream == "" {
			upstream = "http://localhost:9100"
		}
		outURL = strings.TrimSuffix(upstream, "/") + r.URL.String()
	}

	agentID := r.Header.Get("X-Agent-ID")
	if agentID == "" {
		agentID = "unattributed"
	}

	spanID := newID("sp")
	start := time.Now()

	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	r.Body.Close()

	req, err := http.NewRequestWithContext(r.Context(), r.Method, outURL, bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	for k, vv := range r.Header {
		if k == "Proxy-Connection" || k == "Connection" {
			continue
		}
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	// Header injection: trace context rides the request as it passes through.
	req.Header.Set("X-AgentTrace-Span-ID", spanID)
	req.Header.Set("X-AgentTrace-Agent-ID", agentID)

	dest := req.URL.Host + req.URL.Path
	sp := model.Span{
		SpanID:      spanID,
		AgentID:     agentID,
		Type:        classify(dest),
		StartTime:   start,
		ClientIP:    clientIP(r),
		Destination: dest,
		Method:      r.Method,
	}

	// Policy engine: one check in the capture path. Violations warn, not block.
	if warn := p.pol.Check(agentID, dest); warn != nil {
		sp.Warnings = append(sp.Warnings, *warn)
	}

	captureRequest(&sp, body)

	resp, err := p.client.Do(req)
	if err != nil {
		// No answer: timeout, refused, dropped mid-response.
		sp.EndTime = time.Now()
		sp.Dropped = true
		sp.StatusCode = 0
		p.finish(w, &sp, nil, err)
		return
	}
	respBody, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	sp.EndTime = time.Now()
	sp.StatusCode = resp.StatusCode
	if readErr != nil {
		sp.Dropped = true
	}
	captureResponse(&sp, respBody)

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
	p.finish(nil, &sp, respBody, nil)
}

func (p *captureProxy) finish(w http.ResponseWriter, sp *model.Span, _ []byte, err error) {
	if w != nil && err != nil {
		http.Error(w, "upstream unreachable: "+err.Error(), http.StatusBadGateway)
	}
	p.served.Add(1)
	p.ship.enqueue(*sp)
}

func (p *captureProxy) sentStats() map[string]uint64 {
	return map[string]uint64{"sent": p.ship.sent.Load(), "stranded": p.ship.dropped.Load()}
}

// captureRequest parses LLM chat bodies into prompt fields; other payloads
// are captured raw (truncated).
func captureRequest(sp *model.Span, body []byte) {
	if sp.Type == model.SpanLLMCall {
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal(body, &req) == nil {
			sp.Model = req.Model
			for _, m := range req.Messages {
				switch m.Role {
				case "system":
					sp.SystemPrompt = truncate(m.Content)
				case "user":
					sp.UserPrompt = truncate(m.Content)
				}
			}
			return
		}
	}
	sp.RequestBody = truncate(string(body))
}

func captureResponse(sp *model.Span, body []byte) {
	if sp.Type == model.SpanLLMCall && sp.StatusCode == 200 {
		var resp struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(body, &resp) == nil && len(resp.Choices) > 0 {
			sp.Response = truncate(resp.Choices[0].Message.Content)
			sp.PromptTokens = resp.Usage.PromptTokens
			sp.CompletionTokens = resp.Usage.CompletionTokens
			return
		}
	}
	sp.ResponseBody = truncate(string(body))
}

func truncate(s string) string {
	if len(s) > maxCapturedBody {
		return s[:maxCapturedBody] + "…[truncated]"
	}
	return s
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := env("PROXY_ADDR", ":8080")
	collectorURL := env("COLLECTOR_URL", "ws://localhost:7100/ingest")
	policyFile := env("POLICY_FILE", "configs/policy.yaml")
	reattach := env("AT_REATTACH", "on") != "off"

	pol, err := policy.Load(policyFile)
	if err != nil {
		log.Fatalf("policy: %v", err)
	}

	ship := newShipper(collectorURL, newID("proxy"), reattach)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ship.run(ctx)

	p := &captureProxy{
		ship: ship,
		pol:  pol,
		client: &http.Client{
			Timeout: 15 * time.Second,
			// The proxy must not proxy itself.
			Transport: &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 256},
		},
	}
	if !reattach {
		log.Printf("proxy: AT_REATTACH=off — reproducing the orphan incident")
	}
	log.Printf("capture proxy listening on %s -> collector %s", addr, collectorURL)
	srv := &http.Server{Addr: addr, Handler: p}
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// processor consumes raw spans from the transport, assembles them into trace
// missions, labels errors, runs the model checker, redacts payloads, writes
// the two-table schema, and publishes live events for the dashboard.
//
// Correlation is by timestamp: agents are not instrumented, so a trace is an
// agent's burst of activity — a new span opens a trace, activity keeps it
// open, and it closes on a terminal output span or after an idle gap.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agenttrace/internal/behavior"
	"agenttrace/internal/checker"
	"agenttrace/internal/model"
	"agenttrace/internal/redact"
	"agenttrace/internal/store"
	"agenttrace/internal/transport"
)

// idleGap closes a trace when an agent goes quiet (article: idle timeout).
const idleGap = 10 * time.Second

// modelPrices is USD per million tokens (input, output) — cost is computed
// from the captured LLM usage logs, never estimated from text length.
var modelPrices = map[string][2]float64{
	"mock-large-1": {3.00, 15.00},
	"mock-small-1": {0.25, 1.25},
}

// bodyFailure detects "200 but the body says failure": refusals, overload
// messages, cut-off/malformed responses. Rate limits and slowness are NOT
// errors by definition.
var (
	refusalRe  = regexp.MustCompile(`(?i)^(i can'?t|i cannot|i won'?t|i'?m (unable|not able))\b.{0,80}(comply|help|assist|request|guidelines)`)
	overloadRe = regexp.MustCompile(`(?i)\b(currently overloaded|over capacity|please retry your request later)\b`)
)

func bodyFailure(sp *model.Span) bool {
	if sp.Type != model.SpanLLMCall || sp.StatusCode != 200 {
		return false
	}
	if refusalRe.MatchString(sp.Response) || overloadRe.MatchString(sp.Response) {
		return true
	}
	// Cut off mid-thought: non-empty response with no terminal punctuation.
	r := strings.TrimSpace(sp.Response)
	if r != "" && !strings.ContainsAny(r[len(r)-1:], ".!?)]\"'”’") {
		return true
	}
	return false
}

// labelError applies the confirmed error taxonomy.
func labelError(sp *model.Span) {
	switch {
	case sp.Dropped || (sp.StatusCode == 0):
		sp.Error, sp.ErrorKind = true, model.ErrNoAnswer
	case sp.StatusCode >= 400:
		sp.Error, sp.ErrorKind = true, model.ErrHTTPStatus
	case bodyFailure(sp):
		sp.Error, sp.ErrorKind = true, model.ErrBodyFailure
	}
}

func spanCost(sp *model.Span) float64 {
	p, ok := modelPrices[sp.Model]
	if !ok {
		return 0
	}
	return float64(sp.PromptTokens)/1e6*p[0] + float64(sp.CompletionTokens)/1e6*p[1]
}

// openTrace is the processor's in-flight assembly state for one agent.
type openTrace struct {
	summary      model.TraceSummary
	spans        []*model.Span
	lastActivity time.Time
}

type processor struct {
	st      *store.Store
	tr      transport.Transport
	labeler behavior.Labeler

	mu     sync.Mutex
	open   map[string]*openTrace // agentID -> open trace
	seen   map[string]bool       // span dedup (at-least-once transport)
	seenQ  []string              // FIFO for bounded dedup memory
	closed uint64
	spans  uint64

	storageDropped atomic.Uint64
	retained       atomic.Uint64 // traces removed by the retention sweep
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "tr_" + time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b)
}

func (p *processor) handleSpan(ctx context.Context, data []byte) error {
	var sp model.Span
	if err := json.Unmarshal(data, &sp); err != nil {
		log.Printf("processor: bad span payload: %v", err)
		return nil // poison message: ack, don't redeliver forever
	}

	p.mu.Lock()
	if p.seen[sp.SpanID] {
		p.mu.Unlock()
		return nil // duplicate from at-least-once delivery
	}
	p.seen[sp.SpanID] = true
	p.seenQ = append(p.seenQ, sp.SpanID)
	if len(p.seenQ) > 200000 {
		delete(p.seen, p.seenQ[0])
		p.seenQ = p.seenQ[1:]
	}

	// --- trace assembly by timestamp ---
	ot := p.open[sp.AgentID]
	if ot == nil || sp.StartTime.Sub(ot.lastActivity) > idleGap {
		if ot != nil {
			p.closeLocked(ctx, sp.AgentID, ot)
		}
		ot = &openTrace{summary: model.TraceSummary{
			TraceID:   newID(),
			AgentID:   sp.AgentID,
			Status:    model.TraceRunning,
			StartTime: sp.StartTime,
		}}
		p.open[sp.AgentID] = ot
	}
	sp.TraceID = ot.summary.TraceID

	// --- enrichment: error taxonomy, model checker, redaction ---
	labelError(&sp)
	sp.Warnings = append(sp.Warnings, checker.Scan(&sp)...) // checker sees raw text
	redact.Span(&sp)                                        // then payloads are masked before storage

	ot.spans = append(ot.spans, &sp)
	if sp.EndTime.After(ot.lastActivity) {
		ot.lastActivity = sp.EndTime
	}
	s := &ot.summary
	s.SpanCount++
	if sp.Error {
		s.ErrorCount++
	}
	if len(sp.Warnings) > 0 {
		s.WarningCount++
	}
	s.TotalCostUSD += spanCost(&sp)

	terminal := sp.Type == model.SpanOutput
	if terminal {
		p.closeLocked(ctx, sp.AgentID, ot)
		delete(p.open, sp.AgentID)
	}
	summary := *s
	p.mu.Unlock()

	// --- persistence + live edge ---
	// Marking the span seen (above) was the commit point for assembly state,
	// so a transport redelivery would be dropped as a duplicate. Storage
	// failures are therefore retried HERE, and anything that still fails is
	// counted loudly rather than lost silently — the observability layer
	// monitors itself.
	if err := p.retryStore(ctx, func() error { return p.st.InsertSpan(ctx, &sp) }); err != nil {
		log.Printf("processor: span %s DROPPED after storage retries: %v", sp.SpanID, err)
		p.storageDropped.Add(1)
		return nil
	}
	if !terminal {
		if err := p.retryStore(ctx, func() error { return p.st.UpsertSummary(ctx, &summary) }); err != nil {
			log.Printf("processor: summary %s upsert failed: %v", summary.TraceID, err)
		}
		p.publishLive(ctx, model.LiveEvent{Type: "trace_upsert", Summary: &summary})
	}
	p.publishLive(ctx, model.LiveEvent{Type: "span", Span: &sp})
	p.mu.Lock()
	p.spans++
	p.mu.Unlock()
	return nil
}

// retryStore retries a storage write with backoff — Postgres blips (restarts,
// pool contention) must not turn into data loss.
func (p *processor) retryStore(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < 6; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Duration(attempt+1) * 250 * time.Millisecond):
		}
	}
	return err
}

// closeLocked finalizes a trace: latency from wall-clock bounds, behavior
// tree from the labeler, summary flipped to closed. Caller holds p.mu.
func (p *processor) closeLocked(ctx context.Context, agentID string, ot *openTrace) {
	s := ot.summary
	sort.Slice(ot.spans, func(i, j int) bool { return ot.spans[i].StartTime.Before(ot.spans[j].StartTime) })
	end := ot.lastActivity
	s.EndTime = &end
	s.Status = model.TraceClosed
	s.LatencyMS = float64(end.Sub(s.StartTime).Microseconds()) / 1000.0

	tree := p.labeler.Label(ot.spans)
	if tree != nil {
		s.Purpose = tree.Label
	}
	spans := ot.spans
	p.closed++

	// Storage work outside the lock's hot path would be nicer; at demo scale
	// synchronous is simpler and safe (called with lock held only briefly).
	go func() {
		for _, sp := range spans {
			if err := p.st.UpdateSpanBehavior(ctx, sp); err != nil {
				log.Printf("processor: behavior update: %v", err)
			}
		}
		if tree != nil {
			if err := p.st.SaveBehaviorTree(ctx, s.TraceID, agentID, tree); err != nil {
				log.Printf("processor: save tree: %v", err)
			}
		}
		if err := p.st.UpsertSummary(ctx, &s); err != nil {
			log.Printf("processor: close summary: %v", err)
			return
		}
		p.publishLive(ctx, model.LiveEvent{Type: "trace_upsert", Summary: &s})
	}()
}

// retention ages out traces older than the retention window (AT_RETENTION,
// default 24h; 0 disables). Runs at startup and every 10 minutes — without
// it the detail table grows ~0.5 GB/day at demo pace and never shrinks.
func (p *processor) retention(ctx context.Context, keep time.Duration) {
	if keep <= 0 {
		log.Printf("processor: retention disabled")
		return
	}
	run := func() {
		details, traces, err := p.st.DeleteTracesBefore(ctx, time.Now().Add(-keep))
		if err != nil {
			log.Printf("processor: retention sweep failed: %v", err)
			return
		}
		if traces > 0 {
			log.Printf("processor: retention swept %d traces (%d detail rows) older than %s",
				traces, details, keep)
		}
		p.retained.Add(uint64(traces))
	}
	run()
	t := time.NewTicker(10 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// sweep closes idle traces (no terminal span seen).
func (p *processor) sweep(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.mu.Lock()
			now := time.Now()
			for agentID, ot := range p.open {
				if now.Sub(ot.lastActivity) > idleGap {
					p.closeLocked(ctx, agentID, ot)
					delete(p.open, agentID)
				}
			}
			p.mu.Unlock()
		}
	}
}

func (p *processor) publishLive(ctx context.Context, ev model.LiveEvent) {
	data, _ := json.Marshal(ev)
	if err := p.tr.Publish(ctx, transport.SubjectSpansProcessed, data); err != nil {
		log.Printf("processor: live publish: %v", err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	st, err := store.Open(env("POSTGRES_DSN", "postgres://agenttrace:agenttrace@localhost:5432/agenttrace"))
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	tr, err := transport.New(env("AT_TRANSPORT", "nats"), env("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		log.Fatalf("transport: %v", err)
	}
	defer tr.Close()

	p := &processor{
		st: st, tr: tr,
		labeler: behavior.Deterministic{},
		open:    map[string]*openTrace{},
		seen:    map[string]bool{},
	}
	if err := tr.Subscribe(ctx, transport.SubjectSpansRaw, "processor", p.handleSpan); err != nil {
		log.Fatalf("subscribe: %v", err)
	}
	go p.sweep(ctx)

	retention := 24 * time.Hour
	if v := os.Getenv("AT_RETENTION"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			log.Fatalf("invalid AT_RETENTION %q: %v", v, err)
		}
		retention = d
	}
	go p.retention(ctx, retention)

	addr := env("PROCESSOR_ADDR", ":7200")
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		p.mu.Lock()
		defer p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "open_traces": len(p.open), "closed_traces": p.closed, "spans": p.spans,
			"storage_dropped": p.storageDropped.Load(),
			"retention_swept": p.retained.Load(),
		})
	})
	log.Printf("processor running; health on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

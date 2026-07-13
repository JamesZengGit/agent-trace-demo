// api is the dashboard backend: HTTP queries over the summary/detail tables,
// a live WebSocket that streams the processed edge to browsers, and the
// OpenTelemetry ingest adapter (labeled improvement — an optional inlet for
// teams already emitting OTLP; never required).
//
// Hybrid realtime model (confirmed design): HTTP serves closed traces — they
// load with the window and stay put; the WebSocket carries only the live
// edge — running traces and traces closing right now.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"agenttrace/internal/model"
	"agenttrace/internal/otel"
	"agenttrace/internal/store"
	"agenttrace/internal/transport"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

// hub fans processed live events out to every connected dashboard.
type hub struct {
	mu    sync.Mutex
	conns map[*websocket.Conn]chan []byte
}

func newHub() *hub { return &hub{conns: map[*websocket.Conn]chan []byte{}} }

func (h *hub) broadcast(data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for conn, ch := range h.conns {
		select {
		case ch <- data:
		default: // slow browser: drop frame rather than stall the hub
			_ = conn
		}
	}
}

func (h *hub) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	ch := make(chan []byte, 256)
	h.mu.Lock()
	h.conns[conn] = ch
	n := len(h.conns)
	h.mu.Unlock()
	log.Printf("api: dashboard connected (%d live)", n)

	defer func() {
		h.mu.Lock()
		delete(h.conns, conn)
		h.mu.Unlock()
		conn.Close()
	}()

	// Reader: discard client frames, detect close.
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				conn.Close()
				return
			}
		}
	}()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case data := <-ch:
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		case <-ping.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

type api struct {
	st  *store.Store
	tr  transport.Transport
	hub *hub
}

func (a *api) parseWindow(r *http.Request) (time.Time, time.Time) {
	q := r.URL.Query()
	to := time.Now()
	from := to.Add(-15 * time.Minute)
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = t
		}
	}
	return from, to
}

func (a *api) metrics(w http.ResponseWriter, r *http.Request) {
	from, to := a.parseWindow(r)
	m, err := a.st.QueryMetrics(r.Context(), from, to)
	if err != nil {
		httpErr(w, err)
		return
	}
	writeJSON(w, m)
}

func (a *api) traces(w http.ResponseWriter, r *http.Request) {
	from, to := a.parseWindow(r)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	list, err := a.st.QueryTraces(r.Context(), store.TraceFilter{From: from, To: to, Limit: limit})
	if err != nil {
		httpErr(w, err)
		return
	}
	if list == nil {
		list = []*model.TraceSummary{}
	}
	writeJSON(w, map[string]any{"traces": list})
}

func (a *api) trace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	summary, spans, tree, err := a.st.GetTrace(r.Context(), id)
	if err != nil {
		http.Error(w, `{"error":"trace not found"}`, 404)
		return
	}
	writeJSON(w, map[string]any{"summary": summary, "spans": spans, "behavior_tree": tree})
}

// otelIngest converts OTLP/HTTP JSON into native spans and feeds them into
// the same raw pipeline the collector uses.
func (a *api) otelIngest(w http.ResponseWriter, r *http.Request) {
	spans, err := otel.Convert(r.Body)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 400)
		return
	}
	for _, sp := range spans {
		data, _ := json.Marshal(sp)
		if err := a.tr.Publish(r.Context(), transport.SubjectSpansRaw, data); err != nil {
			httpErr(w, err)
			return
		}
	}
	writeJSON(w, map[string]any{"accepted_spans": len(spans)})
}

func httpErr(w http.ResponseWriter, err error) {
	log.Printf("api: %v", err)
	http.Error(w, `{"error":"internal"}`, 500)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func instanceID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
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

	a := &api{st: st, tr: tr, hub: newHub()}
	// Unique queue per instance: every api node sees every live event (fanout,
	// not work-sharing).
	err = tr.Subscribe(ctx, transport.SubjectSpansProcessed, "api-"+instanceID(),
		func(_ context.Context, data []byte) error {
			a.hub.broadcast(data)
			return nil
		})
	if err != nil {
		log.Fatalf("subscribe: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/metrics", a.metrics)
	mux.HandleFunc("GET /api/traces", a.traces)
	mux.HandleFunc("GET /api/traces/{id}", a.trace)
	mux.HandleFunc("/api/live", a.hub.serve)
	mux.HandleFunc("POST /otel/v1/traces", a.otelIngest)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		a.hub.mu.Lock()
		n := len(a.hub.conns)
		a.hub.mu.Unlock()
		writeJSON(w, map[string]any{"ok": true, "dashboards": n})
	})

	addr := env("API_ADDR", ":7000")
	log.Printf("api listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, cors(mux)))
}

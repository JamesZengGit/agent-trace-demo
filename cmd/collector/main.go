// collector is the ingest tier: it holds live WebSocket connections from
// capture proxies, validates incoming spans, acknowledges accepted sequences,
// and publishes to the transport. Acks are what make proxy-side reattachment
// safe: a span is acked only after the transport accepted it, so anything the
// collector dies holding is resent by the proxy on reconnect.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"agenttrace/internal/model"
	"agenttrace/internal/transport"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  64 << 10,
	WriteBufferSize: 16 << 10,
	CheckOrigin:     func(*http.Request) bool { return true },
}

type collector struct {
	tr       transport.Transport
	accepted atomic.Uint64
	rejected atomic.Uint64
}

func validate(e *model.Envelope) error {
	s := &e.Span
	switch {
	case s.SpanID == "":
		return fmt.Errorf("missing span_id")
	case s.AgentID == "":
		return fmt.Errorf("missing agent_id")
	case s.StartTime.IsZero() || s.EndTime.IsZero():
		return fmt.Errorf("missing timestamps")
	case s.EndTime.Before(s.StartTime):
		return fmt.Errorf("end before start")
	case s.Destination == "":
		return fmt.Errorf("missing destination")
	}
	return nil
}

func (c *collector) ingest(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	log.Printf("collector: proxy connected from %s", r.RemoteAddr)

	for {
		var env model.Envelope
		if err := conn.ReadJSON(&env); err != nil {
			log.Printf("collector: connection closed: %v", err)
			return
		}
		if err := validate(&env); err != nil {
			c.rejected.Add(1)
			log.Printf("collector: rejected span: %v", err)
			// Ack anyway: an invalid span will never become valid on resend.
			_ = conn.WriteJSON(model.Ack{Sequence: env.Span.Sequence})
			continue
		}
		data, _ := json.Marshal(&env.Span)
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		err := c.tr.Publish(ctx, transport.SubjectSpansRaw, data)
		cancel()
		if err != nil {
			// Not acked: the proxy keeps it and resends. At-least-once, end to end.
			log.Printf("collector: publish failed (span held at proxy): %v", err)
			continue
		}
		c.accepted.Add(1)
		if err := conn.WriteJSON(model.Ack{Sequence: env.Span.Sequence}); err != nil {
			return
		}
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	addr := env("COLLECTOR_ADDR", ":7100")
	tr, err := transport.New(env("AT_TRANSPORT", "nats"), env("NATS_URL", "nats://localhost:4222"))
	if err != nil {
		log.Fatalf("transport: %v", err)
	}
	defer tr.Close()

	c := &collector{tr: tr}
	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", c.ingest)
	// The observability layer is the last thing anyone monitors — so it
	// monitors itself. Explicit health with real counters, not just a 200.
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true, "accepted": c.accepted.Load(), "rejected": c.rejected.Load(),
		})
	})
	log.Printf("collector listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

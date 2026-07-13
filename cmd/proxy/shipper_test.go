package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"agenttrace/internal/model"
)

// TestShipperConcurrentEnqueueNoLoss is the regression test for a bug found
// by `make bench` accounting: sequence numbers assigned outside the pending-
// buffer lock let concurrent requests append out of order, and ~0.1% of
// spans were trimmed unwritten. Every enqueued span must reach the collector
// exactly at-least-once.
func TestShipperConcurrentEnqueueNoLoss(t *testing.T) {
	var mu sync.Mutex
	received := map[string]bool{}

	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var env model.Envelope
			if err := conn.ReadJSON(&env); err != nil {
				return
			}
			mu.Lock()
			received[env.Span.SpanID] = true
			mu.Unlock()
			_ = conn.WriteJSON(model.Ack{Sequence: env.Span.Sequence})
		}
	}))
	defer srv.Close()

	sh := newShipper("ws"+strings.TrimPrefix(srv.URL, "http")+"/", "test-proxy", true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go sh.run(ctx)

	const workers, perWorker = 64, 200
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := range perWorker {
				sh.enqueue(model.Span{
					SpanID:    fmt.Sprintf("sp-%d-%d", w, i),
					AgentID:   "test",
					StartTime: time.Now(),
					EndTime:   time.Now(),
				})
			}
		}(w)
	}
	wg.Wait()

	want := workers * perWorker
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("lost spans: received %d of %d", len(received), want)
}

package transport

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestMemoryDropsUnderBurst documents why the in-memory transport was
// abandoned (article iteration 1): a slow consumer + a burst = silent loss.
// Run with `go test ./internal/transport -v`.
func TestMemoryDropsUnderBurst(t *testing.T) {
	m := NewMemory(64)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var handled atomic.Uint64
	err := m.Subscribe(ctx, "t", "", func(context.Context, []byte) error {
		time.Sleep(time.Millisecond) // slow consumer
		handled.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	const burst = 5000
	for range burst {
		_ = m.Publish(ctx, "t", []byte("span"))
	}
	time.Sleep(300 * time.Millisecond)

	if m.Dropped() == 0 {
		t.Fatalf("expected drops under burst; handled=%d", handled.Load())
	}
	t.Logf("burst=%d handled=%d dropped=%d — the failure mode that motivated a durable transport",
		burst, handled.Load(), m.Dropped())
}

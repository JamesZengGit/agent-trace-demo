package transport

import (
	"context"
	"sync"
)

// Memory is the article's first transport iteration: an in-process buffer.
// It works locally and collapses under load — messages are dropped when the
// buffer fills, and everything in flight is lost on restart. Kept as a
// benchmarkable exhibit, not as a production path.
type Memory struct {
	mu      sync.RWMutex
	subs    map[string][]chan []byte
	dropped uint64
	bufSize int
}

func NewMemory(bufSize int) *Memory {
	if bufSize <= 0 {
		bufSize = 4096
	}
	return &Memory{subs: map[string][]chan []byte{}, bufSize: bufSize}
}

func (m *Memory) Publish(_ context.Context, subject string, data []byte) error {
	m.mu.RLock()
	chans := m.subs[subject]
	m.mu.RUnlock()
	for _, ch := range chans {
		select {
		case ch <- data:
		default: // buffer full: silently dropped — the failure mode that motivated iteration 2
			m.mu.Lock()
			m.dropped++
			m.mu.Unlock()
		}
	}
	return nil
}

func (m *Memory) Subscribe(ctx context.Context, subject, _ string, h Handler) error {
	ch := make(chan []byte, m.bufSize)
	m.mu.Lock()
	m.subs[subject] = append(m.subs[subject], ch)
	m.mu.Unlock()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case data := <-ch:
				_ = h(ctx, data) // no redelivery: failed messages are gone
			}
		}
	}()
	return nil
}

// Dropped reports how many messages were lost to full buffers.
func (m *Memory) Dropped() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.dropped
}

func (m *Memory) Close() error { return nil }

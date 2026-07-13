// Package transport abstracts the event bus between collector and processor.
// Production used GCP Pub/Sub; the local stand-in is NATS JetStream behind the
// same interface, so the broker is swappable without touching either side.
// This mirrors the article's transport evolution: in-memory buffer -> FIFO
// queue -> managed pub/sub. The in-memory implementation is kept on purpose —
// `make bench TRANSPORT=memory` reproduces why it was abandoned.
package transport

import "context"

// Handler processes one raw message. Returning an error means "do not ack" —
// the message is redelivered (at-least-once delivery).
type Handler func(ctx context.Context, data []byte) error

// Transport is the Pub/Sub-shaped seam. Subjects are dot-separated topics.
type Transport interface {
	Publish(ctx context.Context, subject string, data []byte) error
	// Subscribe delivers messages for subject to h. queue names a consumer
	// group: members share the stream, each message goes to one member.
	Subscribe(ctx context.Context, subject, queue string, h Handler) error
	Close() error
}

// Subjects used across the pipeline.
const (
	SubjectSpansRaw       = "agenttrace.spans.raw"       // collector -> processor
	SubjectSpansProcessed = "agenttrace.spans.processed" // processor -> api (live edge)
)

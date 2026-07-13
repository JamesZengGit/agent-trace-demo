package transport

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NATS implements Transport on NATS JetStream: durable stream, at-least-once
// delivery, redelivery on nack — the same guarantees the production system got
// from GCP Pub/Sub, running as one local container.
type NATS struct {
	nc *nats.Conn
	js jetstream.JetStream
}

func NewNATS(url string) (*NATS, error) {
	nc, err := nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(500*time.Millisecond),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream: %w", err)
	}
	t := &NATS{nc: nc, js: js}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "AGENTTRACE",
		Subjects:  []string{SubjectSpansRaw},
		Retention: jetstream.LimitsPolicy,
		MaxAge:    24 * time.Hour,
		Storage:   jetstream.FileStorage,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("create stream: %w", err)
	}
	return t, nil
}

func (t *NATS) Publish(ctx context.Context, subject string, data []byte) error {
	// The live edge (processor -> api -> browsers) is ephemeral by design:
	// dashboards refetch on reconnect, so durability would only add a
	// stream-ack round trip per event. Core NATS for that subject; JetStream
	// (durable, acked) for everything that must survive a crash.
	if subject == SubjectSpansProcessed {
		return t.nc.Publish(subject, data)
	}
	_, err := t.js.Publish(ctx, subject, data)
	return err
}

func (t *NATS) Subscribe(ctx context.Context, subject, queue string, h Handler) error {
	if subject == SubjectSpansProcessed {
		sub, err := t.nc.Subscribe(subject, func(m *nats.Msg) { _ = h(ctx, m.Data) })
		if err != nil {
			return err
		}
		go func() {
			<-ctx.Done()
			_ = sub.Unsubscribe()
		}()
		return nil
	}
	durable := queue + "-" + strings.ReplaceAll(subject, ".", "-")
	cons, err := t.js.CreateOrUpdateConsumer(ctx, "AGENTTRACE", jetstream.ConsumerConfig{
		Durable:       durable,
		FilterSubject: subject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       30 * time.Second,
		MaxDeliver:    5, // then parked: JetStream's stand-in for a dead-letter queue
	})
	if err != nil {
		return fmt.Errorf("create consumer: %w", err)
	}
	cc, err := cons.Consume(func(msg jetstream.Msg) {
		if err := h(ctx, msg.Data()); err != nil {
			_ = msg.Nak() // redeliver — at-least-once
			return
		}
		_ = msg.Ack()
	})
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	go func() {
		<-ctx.Done()
		cc.Stop()
	}()
	return nil
}

func (t *NATS) Close() error {
	t.nc.Close()
	return nil
}

// New picks a transport from configuration: "nats" (default) or "memory".
func New(kind, natsURL string) (Transport, error) {
	switch kind {
	case "", "nats":
		return NewNATS(natsURL)
	case "memory":
		return NewMemory(4096), nil
	default:
		return nil, fmt.Errorf("unknown transport %q", kind)
	}
}

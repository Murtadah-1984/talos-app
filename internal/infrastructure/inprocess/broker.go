// Package inprocess provides in-memory Broker/EventBus implementations used
// in unit tests and single-process local development, so the workflow
// engine's logic can be exercised without a running RabbitMQ instance.
// Production deployments use internal/infrastructure/rabbitmq instead.
package inprocess

import (
	"context"
	"sync"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

// Broker is a minimal, non-durable ports.Broker: each Publish call is
// dispatched synchronously to every currently-subscribed handler for that
// topic. It intentionally does not persist messages across restarts.
type Broker struct {
	mu       sync.RWMutex
	handlers map[string][]func(context.Context, []byte) error
}

func NewBroker() *Broker {
	return &Broker{handlers: make(map[string][]func(context.Context, []byte) error)}
}

func (b *Broker) Publish(ctx context.Context, topic string, payload []byte) error {
	b.mu.RLock()
	hs := append([]func(context.Context, []byte) error{}, b.handlers[topic]...)
	b.mu.RUnlock()

	for _, h := range hs {
		if err := h(ctx, payload); err != nil {
			return err
		}
	}
	return nil
}

func (b *Broker) Subscribe(_ context.Context, topic string, handler func(context.Context, []byte) error) (func(), error) {
	b.mu.Lock()
	b.handlers[topic] = append(b.handlers[topic], handler)
	idx := len(b.handlers[topic]) - 1
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		hs := b.handlers[topic]
		if idx < len(hs) {
			hs[idx] = nil
		}
	}
	return unsubscribe, nil
}

var _ ports.Broker = (*Broker)(nil)

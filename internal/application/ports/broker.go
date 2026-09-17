package ports

import "context"

// Broker is the durable async messaging port backing the workflow engine
// (ADR-0005). RabbitMQ is the production implementation; an in-process
// implementation backs local development/tests.
type Broker interface {
	Publish(ctx context.Context, topic string, payload []byte) error
	Subscribe(ctx context.Context, topic string, handler func(ctx context.Context, payload []byte) error) (unsubscribe func(), err error)
}

// EventBus fans out platform events (audit/observability/workflow progress)
// to real-time subscribers (the WebSocket/SSE layer, §28). Distinct from
// Broker: EventBus is fire-and-forget/best-effort UI fan-out, not durable
// work dispatch.
type EventBus interface {
	Publish(ctx context.Context, channel string, event any) error
	Subscribe(ctx context.Context, channel string) (<-chan []byte, func(), error)
}

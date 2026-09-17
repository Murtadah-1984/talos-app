// Package rabbitmq implements the ports.Broker port (ADR-0005) on top of
// RabbitMQ, giving the workflow engine durable, at-least-once delivery of
// step-dispatch messages across platform-worker replicas.
package rabbitmq

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/talos-platform/talos-platform/internal/application/ports"
)

// Broker is a topic-per-queue RabbitMQ implementation of ports.Broker: each
// topic gets its own durable queue bound to a single fanout-free exchange,
// which is sufficient for the workflow engine's single-consumer-group model.
type Broker struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

func Dial(url string) (*Broker, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("dialing rabbitmq: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("opening rabbitmq channel: %w", err)
	}
	return &Broker{conn: conn, ch: ch}, nil
}

func (b *Broker) Close() error {
	if err := b.ch.Close(); err != nil {
		return err
	}
	return b.conn.Close()
}

func (b *Broker) declareQueue(topic string) error {
	_, err := b.ch.QueueDeclare(topic, true /*durable*/, false, false, false, nil)
	return err
}

func (b *Broker) Publish(ctx context.Context, topic string, payload []byte) error {
	if err := b.declareQueue(topic); err != nil {
		return fmt.Errorf("declaring queue %s: %w", topic, err)
	}
	return b.ch.PublishWithContext(ctx, "", topic, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Body:         payload,
	})
}

func (b *Broker) Subscribe(ctx context.Context, topic string, handler func(context.Context, []byte) error) (func(), error) {
	if err := b.declareQueue(topic); err != nil {
		return nil, fmt.Errorf("declaring queue %s: %w", topic, err)
	}
	// A dedicated channel per subscription keeps Ack/Nack isolated from Publish.
	ch, err := b.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("opening consumer channel: %w", err)
	}
	msgs, err := ch.Consume(topic, "", false /*autoAck*/, false, false, false, nil)
	if err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("consuming queue %s: %w", topic, err)
	}

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case msg, ok := <-msgs:
				if !ok {
					return
				}
				if err := handler(ctx, msg.Body); err != nil {
					_ = msg.Nack(false, true) // requeue on handler failure (ADR-0005 retry)
					continue
				}
				_ = msg.Ack(false)
			}
		}
	}()

	unsubscribe := func() {
		close(done)
		_ = ch.Close()
	}
	return unsubscribe, nil
}

var _ ports.Broker = (*Broker)(nil)

package event

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/streadway/amqp"

	"github.com/VadimVikt/banner-rotation/internal/model"
)

const (
	defaultExchange     = "banner_events"
	defaultExchangeType = "topic"
	defaultQueue        = "banner_events_queue"
)

// RabbitMQPublisher publishes domain events to a RabbitMQ broker.
type RabbitMQPublisher struct {
	url      string
	conn     *amqp.Connection
	ch       *amqp.Channel
	exchange string
	queue    string
}

// NewRabbitMQPublisher creates a publisher connected to the RabbitMQ broker at the given URL.
// It auto-declares an exchange and a queue on initialization.
func NewRabbitMQPublisher(url string) (*RabbitMQPublisher, error) {
	if url == "" {
		url = "amqp://guest:guest@localhost:5672/"
	}

	p := &RabbitMQPublisher{
		url:      url,
		exchange: defaultExchange,
		queue:    defaultQueue,
	}

	if err := p.connect(); err != nil {
		return nil, fmt.Errorf("connect to rabbitmq: %w", err)
	}

	if err := p.declare(); err != nil {
		p.Close()
		return nil, fmt.Errorf("declare exchange/queue: %w", err)
	}

	return p, nil
}

// connect (or reconnect) to the RabbitMQ broker.
func (p *RabbitMQPublisher) connect() error {
	var err error
	p.conn, err = amqp.Dial(p.url)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	p.ch, err = p.conn.Channel()
	if err != nil {
		p.conn.Close()
		return fmt.Errorf("open channel: %w", err)
	}

	return nil
}

// declare creates the exchange and queue, and binds them.
func (p *RabbitMQPublisher) declare() error {
	if err := p.ch.ExchangeDeclare(
		p.exchange,
		defaultExchangeType,
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,   // arguments
	); err != nil {
		return fmt.Errorf("declare exchange: %w", err)
	}

	_, err := p.ch.QueueDeclare(
		p.queue,
		true,  // durable
		false, // auto-deleted
		false, // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("declare queue: %w", err)
	}

	if err := p.ch.QueueBind(
		p.queue,
		"", // routing key
		p.exchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("bind queue: %w", err)
	}

	return nil
}

// Publish serializes the event to JSON and publishes it to the RabbitMQ exchange.
func (p *RabbitMQPublisher) Publish(_ context.Context, ev model.Event) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// Attempt to reconnect on nil channel/connection.
	if p.ch == nil || p.conn.IsClosed() {
		if reconnectErr := p.connect(); reconnectErr != nil {
			return reconnectErr
		}
		if declareErr := p.declare(); declareErr != nil {
			return declareErr
		}
	}

	err = p.ch.Publish(
		p.exchange,
		string(ev.Type), // routing key
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	return nil
}

// Close closes the AMQP channel and connection.
func (p *RabbitMQPublisher) Close() {
	if p.ch != nil {
		p.ch.Close()
	}
	if p.conn != nil {
		p.conn.Close()
	}
}

// Ensure RabbitMQPublisher implements Publisher at compile time.
var _ Publisher = (*RabbitMQPublisher)(nil)

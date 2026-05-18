package rabbitmq

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

type NotificationEvent struct {
	EventID  string            `json:"event_id"`
	UserID   string            `json:"user_id"`
	UserRole string            `json:"user_role"`
	Type     string            `json:"type"`
	Title    string            `json:"title"`
	Content  string            `json:"content"`
	Priority string            `json:"priority"`
	Category string            `json:"category"`
	Channels []string          `json:"channels"`
	Metadata map[string]string `json:"metadata"`
}

type Publisher struct {
	conn     *amqp.Connection
	ch       *amqp.Channel
	exchange string
}

func NewPublisher(url, exchange string) (*Publisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

	err = ch.ExchangeDeclare(
		exchange, // name
		"topic",   // type
		true,     // durable
		false,    // auto-deleted
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
	if err != nil {
		return nil, err
	}

	return &Publisher{
		conn:     conn,
		ch:       ch,
		exchange: exchange,
	}, nil
}

func (p *Publisher) SendNotification(ctx context.Context, event NotificationEvent) error {
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}
	if len(event.Channels) == 0 {
		event.Channels = []string{"push", "email", "sms"}
	}

	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	routingKey := "notification.wallet." + event.Type

	return p.ch.PublishWithContext(ctx,
		p.exchange, // exchange
		routingKey, // routing key
		false,      // mandatory
		false,      // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
}

func (p *Publisher) Close() {
	if p.ch != nil {
		p.ch.Close()
	}
	if p.conn != nil {
		p.conn.Close()
	}
}

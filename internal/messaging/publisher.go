package messaging

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

func exchangeFromEnv(explicit string, keys []string, fallback string) string {
	if s := strings.TrimSpace(explicit); s != "" {
		return s
	}
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return fallback
}

type Publisher struct {
	conn                 *amqp.Connection
	ch                   *amqp.Channel
	analyticsExchange    string
	notificationExchange string
}

func NewPublisher(amqpURL, analyticsExchange, notificationExchange string) (*Publisher, error) {
	if strings.TrimSpace(amqpURL) == "" {
		return &Publisher{}, nil
	}
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	p := &Publisher{
		conn: conn,
		ch:   ch,
		analyticsExchange: exchangeFromEnv(analyticsExchange,
			[]string{"ANALYTICS_EXCHANGE", "RABBITMQ_EXCHANGE_ANALYTICS"},
			"analytics_exchange",
		),
		notificationExchange: exchangeFromEnv(notificationExchange,
			[]string{"NOTIFICATION_EXCHANGE", "RABBITMQ_EXCHANGE_NOTIFICATION"},
			"notification_exchange",
		),
	}
	if p.analyticsExchange != "" {
		if err := ch.ExchangeDeclare(
			p.analyticsExchange,
			"topic",
			true,
			false,
			false,
			false,
			nil,
		); err != nil {
			_ = p.Close()
			return nil, err
		}
	}
	if p.notificationExchange != "" {
		if err := ch.ExchangeDeclare(
			p.notificationExchange,
			"topic",
			true,
			false,
			false,
			false,
			nil,
		); err != nil {
			_ = p.Close()
			return nil, err
		}
	}
	return p, nil
}

func (p *Publisher) Close() error {
	if p.ch != nil {
		_ = p.ch.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

func (p *Publisher) publishJSON(exchange, routingKey string, payload map[string]any) error {
	if p.ch == nil || exchange == "" {
		return nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.ch.PublishWithContext(
		context.Background(),
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			Body:         b,
			Timestamp:    time.Now().UTC(),
		},
	)
}

func (p *Publisher) PublishAnalytics(_ context.Context, routingKey string, fields map[string]any) error {
	payload := map[string]any{
		"id":         uuid.NewString(),
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	for k, v := range fields {
		payload[k] = v
	}
	return p.publishJSON(p.analyticsExchange, routingKey, payload)
}

func (p *Publisher) PublishNotification(_ context.Context, routingKey string, fields map[string]any) error {
	return p.publishJSON(p.notificationExchange, routingKey, fields)
}

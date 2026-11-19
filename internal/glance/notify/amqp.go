package notify

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// AMQP publishes like oslo.messaging's rabbit driver for notifications: topic
// exchange "glance" (control_exchange) and queue "<topic>.<priority>" bound by
// the same routing key, all non-durable and not auto-deleted (oslo defaults
// amqp_durable_queues/amqp_auto_delete=False); persistent JSON messages.
type AMQP struct {
	uri      string
	exchange string
	timeout  time.Duration

	mu       sync.Mutex
	conn     *amqp.Connection
	ch       *amqp.Channel
	declared map[string]bool
}

// ParseTransportURL accepts oslo's single-host form rabbit://user:pass@host:port/vhost
// (amqp:// and amqps:// too) and returns the AMQP URI. Errors never echo it.
func ParseTransportURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	_, rest, _ := strings.Cut(raw, "://")
	authority, _, _ := strings.Cut(rest, "/")
	if strings.Contains(authority, ",") || strings.Count(authority, "@") > 1 {
		return "", errors.New("transport URL must name exactly one broker host")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("transport URL must be rabbit://user:password@host:port/[vhost] with one host")
	}
	switch u.Scheme {
	case "rabbit":
		u.Scheme = "amqp"
	case "amqp", "amqps":
	default:
		return "", errors.New("transport URL scheme must be rabbit, amqp or amqps")
	}
	if _, err = amqp.ParseURI(u.String()); err != nil {
		return "", errors.New("transport URL is not a valid AMQP address")
	}
	return u.String(), nil
}

func NewAMQP(uri, exchange string, timeout time.Duration) *AMQP {
	if exchange == "" {
		exchange = "glance"
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &AMQP{uri: uri, exchange: exchange, timeout: timeout, declared: map[string]bool{}}
}

func (a *AMQP) Publish(ctx context.Context, topic, priority string, body []byte) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	if err := a.connectLocked(); err != nil {
		return err
	}
	key := topic + "." + strings.ToLower(priority)
	if !a.declared[key] {
		if _, err := a.ch.QueueDeclare(key, false, false, false, false, nil); err != nil {
			a.resetLocked()
			return err
		}
		if err := a.ch.QueueBind(key, key, a.exchange, false, nil); err != nil {
			a.resetLocked()
			return err
		}
		a.declared[key] = true
	}
	err := a.ch.PublishWithContext(ctx, a.exchange, key, false, false, amqp.Publishing{
		ContentType: "application/json", ContentEncoding: "utf-8", DeliveryMode: amqp.Persistent, Body: body,
	})
	if err != nil {
		a.resetLocked()
	}
	return err
}

func (a *AMQP) connectLocked() error {
	if a.ch != nil && !a.ch.IsClosed() && a.conn != nil && !a.conn.IsClosed() {
		return nil
	}
	a.resetLocked()
	conn, err := amqp.DialConfig(a.uri, amqp.Config{
		Dial: amqp.DefaultDial(a.timeout), Heartbeat: 10 * time.Second,
		Properties: amqp.Table{"connection_name": "glance-go notifier"},
	})
	if err != nil {
		return errors.New("cannot connect to the notification broker")
	}
	ch, err := conn.Channel()
	if err == nil {
		err = ch.ExchangeDeclare(a.exchange, "topic", false, false, false, false, nil)
	}
	if err != nil {
		_ = conn.Close()
		return errors.New("cannot prepare the notification exchange")
	}
	a.conn, a.ch = conn, ch
	return nil
}

func (a *AMQP) resetLocked() {
	if a.ch != nil {
		_ = a.ch.Close()
	}
	if a.conn != nil {
		_ = a.conn.Close()
	}
	a.conn, a.ch = nil, nil
	a.declared = map[string]bool{}
}

func (a *AMQP) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.resetLocked()
	return nil
}

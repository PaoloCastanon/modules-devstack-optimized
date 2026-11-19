package notify

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Transport publishes one encoded notification to its topic and priority. It
// is implemented over AMQP (amqpTransport) and by test doubles.
type Transport interface {
	Publish(ctx context.Context, topic, priority string, body []byte) error
	Close() error
}

// MessagingOptions configure the "messagingv2" driver.
type MessagingOptions struct {
	PublisherID string   // default_publisher_id; "image.localhost" by default
	Topics      []string // [oslo_messaging_notifications] topics; "notifications"
	QueueSize   int      // bounded buffer; when full, new events are dropped
	Retry       time.Duration
	Now         func() time.Time
	Logger      *slog.Logger
}

type pending struct {
	body     []byte
	priority string
}

// Messaging is the messagingv2 driver. Notify only encodes and enqueues; one
// worker publishes in order. Unlike official Glance, a slow or absent broker
// never delays or fails an API request: while it is unreachable the worker
// retries the oldest message and new ones wait in the bounded queue, then are
// dropped (counted and logged once per outage).
type Messaging struct {
	transport Transport
	opts      MessagingOptions
	queue     chan pending
	done      chan struct{}
	stop      context.CancelFunc
	ctx       context.Context

	mu      sync.Mutex
	closed  bool
	dropped int64
	warned  bool
}

func NewMessaging(t Transport, opts MessagingOptions) *Messaging {
	if opts.PublisherID == "" {
		opts.PublisherID = "image.localhost"
	}
	if len(opts.Topics) == 0 {
		opts.Topics = []string{"notifications"}
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = 1024
	}
	if opts.Retry <= 0 {
		opts.Retry = time.Second
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Logger == nil {
		opts.Logger = slog.New(slog.DiscardHandler)
	}
	ctx, stop := context.WithCancel(context.Background())
	m := &Messaging{transport: t, opts: opts, queue: make(chan pending, opts.QueueSize), done: make(chan struct{}), stop: stop, ctx: ctx}
	go m.run()
	return m
}

func (m *Messaging) Notify(eventType, priority string, payload any) {
	body, err := Envelope(m.opts.PublisherID, eventType, priority, payload, m.opts.Now())
	if err != nil {
		m.opts.Logger.Warn("notification not encoded", "event_type", eventType)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	select {
	case m.queue <- pending{body: body, priority: priority}:
	default:
		m.dropped++
		if !m.warned {
			m.warned = true
			m.opts.Logger.Warn("notification queue full; dropping notifications", "queue_size", m.opts.QueueSize)
		}
	}
}

// Dropped reports how many notifications the full queue rejected.
func (m *Messaging) Dropped() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dropped
}

func (m *Messaging) run() {
	defer close(m.done)
	for p := range m.queue {
		for _, topic := range m.opts.Topics {
			for {
				err := m.transport.Publish(m.ctx, topic, p.priority, p.body)
				if err == nil {
					m.mu.Lock()
					if m.warned {
						m.warned = false
						m.opts.Logger.Info("notifications delivered again", "dropped_total", m.dropped)
					}
					m.mu.Unlock()
					break
				}
				if m.ctx.Err() != nil {
					return
				}
				m.mu.Lock()
				if !m.warned {
					m.warned = true
					m.opts.Logger.Warn("notification broker unavailable; retrying", "error_kind", errorKind(err))
				}
				m.mu.Unlock()
				select {
				case <-m.ctx.Done():
					return
				case <-time.After(m.opts.Retry):
				}
			}
		}
	}
}

// Close stops accepting events, delivers what is queued until ctx ends, then
// releases the transport. Messages still queued at the deadline are lost.
func (m *Messaging) Close(ctx context.Context) error {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		close(m.queue)
	}
	m.mu.Unlock()
	var err error
	select {
	case <-m.done:
	case <-ctx.Done():
		err = errors.New("notification queue not drained before shutdown")
	}
	m.stop()
	<-m.done
	if cerr := m.transport.Close(); err == nil {
		err = cerr
	}
	return err
}

// errorKind never includes broker addresses or credentials.
func errorKind(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case strings.Contains(strings.ToLower(err.Error()), "refused"):
		return "connection_refused"
	default:
		return "publish_failed"
	}
}

package main

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/config"
	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/notify"
)

// notifierFor builds [oslo_messaging_notifications]: noop by default, or
// messagingv2 to the "glance" exchange (official control_exchange). The
// returned close drains queued notifications after HTTP has stopped.
func notifierFor(n config.Notifications, logger *slog.Logger) (notify.Notifier, func(context.Context) error, error) {
	if n.Driver != "messagingv2" {
		return notify.Noop{}, func(context.Context) error { return nil }, nil
	}
	uri, err := notify.ParseTransportURL(n.TransportURL)
	if err != nil {
		return nil, nil, err
	}
	m := notify.NewMessaging(notify.NewAMQP(uri, "glance", 5*time.Second),
		notify.MessagingOptions{PublisherID: n.PublisherID, Topics: n.Topics, Logger: logger})
	var out notify.Notifier = m
	if len(n.Disabled) > 0 {
		out = notify.Filtered{Next: m, Disabled: n.Disabled}
	}
	logger.Info("notifications enabled", "driver", n.Driver, "topics", strings.Join(n.Topics, ","), "publisher_id", n.PublisherID)
	return out, m.Close, nil
}

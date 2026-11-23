package config

import (
	"regexp"
)

// Notifications is [oslo_messaging_notifications] plus Glance's
// default_publisher_id and disabled_notifications. TransportURL carries
// credentials: it only comes from a private file and is never logged.
type Notifications struct {
	Driver       string // "noop" (default) or "messagingv2"
	TransportURL string
	Topics       []string
	PublisherID  string
	Disabled     map[string]bool
}

var notificationName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

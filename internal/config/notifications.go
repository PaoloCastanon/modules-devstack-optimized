package config

import (
	"errors"
	"regexp"
	"strings"
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

func loadNotifications(c *Config, env func(string) string) error {
	n := Notifications{Driver: "noop", Topics: []string{"notifications"}, PublisherID: "image.localhost", Disabled: map[string]bool{}}
	switch driver := env("GLANCE_GO_NOTIFICATION_DRIVER"); driver {
	case "", "noop":
	case "messagingv2":
		n.Driver = driver
		secret, err := ReadSecret(env("GLANCE_GO_TRANSPORT_URL_FILE"))
		if err != nil {
			return errors.New("GLANCE_GO_TRANSPORT_URL_FILE: " + err.Error())
		}
		n.TransportURL = strings.TrimSpace(string(secret))
		if n.TransportURL == "" {
			return errors.New("GLANCE_GO_TRANSPORT_URL_FILE is empty")
		}
	default:
		return errors.New("GLANCE_GO_NOTIFICATION_DRIVER must be noop or messagingv2")
	}
	list := func(key string) ([]string, error) {
		var out []string
		for _, item := range strings.Split(env(key), ",") {
			if item = strings.TrimSpace(item); item == "" {
				continue
			}
			if !notificationName.MatchString(item) {
				return nil, errors.New(key + " entries must be names like image.send or notifications")
			}
			out = append(out, item)
		}
		return out, nil
	}
	topics, err := list("GLANCE_GO_NOTIFICATION_TOPICS")
	if err != nil {
		return err
	}
	if len(topics) > 0 {
		n.Topics = topics
	}
	if id := env("GLANCE_GO_DEFAULT_PUBLISHER_ID"); id != "" {
		if !notificationName.MatchString(id) {
			return errors.New("GLANCE_GO_DEFAULT_PUBLISHER_ID must be like image.host1")
		}
		n.PublisherID = id
	}
	disabled, err := list("GLANCE_GO_DISABLED_NOTIFICATIONS")
	if err != nil {
		return err
	}
	for _, d := range disabled {
		n.Disabled[d] = true
	}
	c.Notifications = n
	return nil
}

package config

import (
	"strings"
	"testing"
)

// Notifications come from the original's [oslo_messaging_notifications] and
// [DEFAULT] (transport_url fallback, default_publisher_id,
// disabled_notifications), as oslo.config reads them.
func TestNotificationsConfiguration(t *testing.T) {
	load := func(text string) (Config, error) {
		e := map[string]string{"GLANCE_GO_CONFIG_FILE": secretFile(t, text)}
		return Load(func(k string) string { return e[k] }, "serve")
	}
	c, err := load(glanceConf(""))
	if err != nil || c.Notifications.Driver != "noop" || c.Notifications.PublisherID != "image.localhost" ||
		strings.Join(c.Notifications.Topics, ",") != "notifications" || len(c.Notifications.Disabled) != 0 {
		t.Fatal("noop with official names expected", c.Notifications, err)
	}
	on := strings.Replace(glanceConf("default_publisher_id = image.host1\ndisabled_notifications = image.send,task\n"),
		"driver = noop", "driver = messagingv2\ntopics = notifications, audit", 1)
	c, err = load(on)
	if err != nil || c.Notifications.Driver != "messagingv2" || c.Notifications.TransportURL != "rabbit://stackrabbit:s3cret@127.0.0.1:5672/" ||
		strings.Join(c.Notifications.Topics, ",") != "notifications,audit" || c.Notifications.PublisherID != "image.host1" ||
		!c.Notifications.Disabled["image.send"] || !c.Notifications.Disabled["task"] {
		t.Fatal("messagingv2 settings", c.Notifications, err)
	}
	for _, bad := range []string{
		strings.Replace(on, "topics = notifications, audit", "topics = bad topic", 1),
		strings.Replace(on, "image.host1", "image/x", 1),
		strings.Replace(on, "image.send,task", "image.send;rm", 1),
		strings.Replace(strings.Replace(glanceConf(""), "driver = noop", "driver = messagingv2", 1), "transport_url = rabbit://stackrabbit:s3cret@127.0.0.1:5672/\n", "", 1),
	} {
		if _, err = load(bad); err == nil || strings.Contains(err.Error(), "s3cret") {
			t.Fatal("accepted or echoed unsafe notification config", err)
		}
	}
	e := environment(t)
	e["GLANCE_GO_NOTIFICATION_DRIVER"] = "messagingv2"
	if _, err = Load(func(k string) string { return e[k] }, "serve"); err == nil {
		t.Fatal("an environment override competed with glance-api.conf")
	}
}

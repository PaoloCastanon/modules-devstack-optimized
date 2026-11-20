package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotificationsConfiguration(t *testing.T) {
	load := func(extra map[string]string) (Config, error) {
		e := environment(t)
		for k, v := range extra {
			e[k] = v
		}
		return Load(func(k string) string { return e[k] }, "serve")
	}
	c, err := load(nil)
	if err != nil || c.Notifications.Driver != "noop" || c.Notifications.PublisherID != "image.localhost" ||
		strings.Join(c.Notifications.Topics, ",") != "notifications" || len(c.Notifications.Disabled) != 0 {
		t.Fatal("defaults must be noop with official names", c.Notifications, err)
	}
	dir := t.TempDir()
	secret := filepath.Join(dir, "transport")
	if err = os.WriteFile(secret, []byte("rabbit://stackrabbit:s3cret@192.168.122.10:5672/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err = load(map[string]string{"GLANCE_GO_NOTIFICATION_DRIVER": "messagingv2", "GLANCE_GO_TRANSPORT_URL_FILE": secret,
		"GLANCE_GO_NOTIFICATION_TOPICS": "notifications, audit", "GLANCE_GO_DEFAULT_PUBLISHER_ID": "image.host1",
		"GLANCE_GO_DISABLED_NOTIFICATIONS": "image.send,task"})
	if err != nil || c.Notifications.TransportURL != "rabbit://stackrabbit:s3cret@192.168.122.10:5672/" ||
		strings.Join(c.Notifications.Topics, ",") != "notifications,audit" || c.Notifications.PublisherID != "image.host1" ||
		!c.Notifications.Disabled["image.send"] || !c.Notifications.Disabled["task"] {
		t.Fatal("messagingv2 settings", err)
	}
	loose := filepath.Join(dir, "loose")
	_ = os.WriteFile(loose, []byte("rabbit://u:s3cret@h:5672/"), 0o644)
	for _, bad := range []map[string]string{
		{"GLANCE_GO_NOTIFICATION_DRIVER": "messaging"},
		{"GLANCE_GO_NOTIFICATION_DRIVER": "messagingv2"},
		{"GLANCE_GO_NOTIFICATION_DRIVER": "messagingv2", "GLANCE_GO_TRANSPORT_URL_FILE": loose},
		{"GLANCE_GO_NOTIFICATION_TOPICS": "bad topic"},
		{"GLANCE_GO_DEFAULT_PUBLISHER_ID": "image/x"},
		{"GLANCE_GO_DISABLED_NOTIFICATIONS": "image.send;rm"},
	} {
		if _, err = load(bad); err == nil || strings.Contains(err.Error(), "s3cret") {
			t.Fatal("accepted or echoed unsafe notification config", bad, err)
		}
	}
}

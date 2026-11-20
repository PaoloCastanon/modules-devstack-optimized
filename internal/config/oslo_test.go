package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const devstackConf = `[DEFAULT]
public_endpoint = http://192.168.122.10/image
transport_url = rabbit://stackrabbit:secret@192.168.122.10:5672/
; comment
[database]
connection = mysql+pymysql://root:p%40ss@127.0.0.1/glance?charset=utf8&plugin=dbcounter
[keystone_authtoken]
auth_url = http://192.168.122.10/identity
auth_type = password
username = glance
password = svc
project_name = service
user_domain_name = Default
project_domain_name = Default
[oslo_messaging_notifications]
driver = messagingv2
[glance_store]
filesystem_store_datadir = /opt/stack/data/glance/images/
`

func originalConf(t *testing.T, text string, mode os.FileMode) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "glance-api.conf")
	if err := os.WriteFile(p, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOriginalGlanceAPIConfIsReadLikeTheOriginal(t *testing.T) {
	env := map[string]string{"GLANCE_GO_CONFIG_FILE": originalConf(t, devstackConf, 0644)}
	c, err := Load(func(k string) string { return env[k] }, "serve")
	if err != nil {
		t.Fatal(err)
	}
	if !c.OriginalAccount || c.OfficialCatalog != "glance" || c.Binary.OfficialStoreDir != "/opt/stack/data/glance/images" {
		t.Fatal("catalog", c.OriginalAccount, c.OfficialCatalog, c.Binary.OfficialStoreDir)
	}
	if !strings.HasPrefix(c.DatabaseDSN, "root:p@ss@tcp(127.0.0.1:3306)/glance_go_shared") {
		t.Fatal("same account on the shared database expected, got", c.DatabaseDSN)
	}
	if c.KeystoneURL != "http://192.168.122.10/identity/v3" || c.Credential.Username != "glance" || c.Credential.ProjectName != "service" || c.Credential.ID != "" {
		t.Fatal("keystone", c.KeystoneURL, c.Credential.Username)
	}
	if c.Notifications.Driver != "messagingv2" || c.Notifications.TransportURL != "rabbit://stackrabbit:secret@192.168.122.10:5672/" || c.Notifications.Topics[0] != "notifications" {
		t.Fatal("notifications", c.Notifications)
	}
	if c.PublicURL != "http://192.168.122.10/image" || c.TokenCacheTime != 300*time.Second {
		t.Fatal(c.PublicURL, c.TokenCacheTime)
	}
}

func TestOriginalGlanceAPIConfRefusesAmbiguityAndUnsafeFiles(t *testing.T) {
	path := originalConf(t, devstackConf, 0644)
	for _, extra := range []map[string]string{
		{"GLANCE_GO_DATABASE_DSN_FILE": "/x"},
		{"GLANCE_GO_KEYSTONE_AUTH_URL": "http://k/v3"},
		{"GLANCE_GO_NOTIFICATION_DRIVER": "noop"},
		{"GLANCE_GO_CATALOG": "own"},
	} {
		extra["GLANCE_GO_CONFIG_FILE"] = path
		if _, err := Load(func(k string) string { return extra[k] }, "serve"); err == nil {
			t.Fatalf("accepted %v", extra)
		}
	}
	writable := originalConf(t, devstackConf, 0666)
	if _, err := Load(func(k string) string { return map[string]string{"GLANCE_GO_CONFIG_FILE": writable}[k] }, "serve"); err == nil {
		t.Fatal("world-writable glance-api.conf accepted")
	}
	noCatalog := originalConf(t, strings.Replace(devstackConf, "/glance?", "/nova?", 1), 0644)
	if _, err := Load(func(k string) string { return map[string]string{"GLANCE_GO_CONFIG_FILE": noCatalog}[k] }, "serve"); err == nil {
		t.Fatal("a non-glance database accepted")
	}
}

func TestOsloParserMergesSectionsAndLastValueWins(t *testing.T) {
	f, err := parseOslo("[DEFAULT]\nA = 1\n[x]\nb = 2\n[x]\nB = 3\n")
	if err != nil || f.get("x", "b") != "3" || f.get("x", "a") != "1" {
		t.Fatal(f, err)
	}
	if _, err = parseOslo("key = value\n"); err == nil {
		t.Fatal("option outside a section accepted")
	}
}

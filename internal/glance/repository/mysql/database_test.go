package mysql

import (
	"testing"
)

func TestDSNGuards(t *testing.T) {
	for _, dsn := range []string{
		"root:x@tcp(127.0.0.1:3306)/glance", "root:x@tcp(127.0.0.1:3306)/placement",
		":x@tcp(127.0.0.1:3306)/glance_go_shared",
		"glance_go_app:x@tcp(localhost:3306)/glance_go_shared?multiStatements=true",
		"glance_go_app:x@tcp(localhost:3306)/glance_go_shared?tls=skip-verify",
		"glance_go_app:x@tcp(localhost:3306)/glance_go_shared?allowAllFiles=true",
	} {
		if _, err := Open(dsn); err == nil {
			t.Fatalf("accepted unsafe DSN %s", dsn)
		}
	}
	db, err := Open("glance_go_app:fixture@tcp(127.0.0.1:1)/glance_go_shared")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	c, err := parseDSN("glance_go_app:fixture@tcp(127.0.0.1:1)/glance_go_shared")
	if err != nil || !c.ParseTime || c.InterpolateParams || c.ReadTimeout <= 0 || !c.ClientFoundRows {
		t.Fatal("unsafe defaults")
	}
	if err := ValidateTestDSN("glance_go_test:fixture@tcp(127.0.0.1:3306)/glance_go_shared"); err == nil {
		t.Fatal("tests accepted development schema")
	}
	if err := ValidateTestDSN("glance_go_test:fixture@tcp(192.168.122.10:3306)/glance_go_shared_test_a"); err == nil {
		t.Fatal("tests accepted thesis VM")
	}
	if err := ValidateTestDSN("glance_go_test:fixture@tcp(127.0.0.1:3306)/glance_go_shared_test_a"); err != nil {
		t.Fatal(err)
	}
}

func TestDSNTransportAndAutocommit(t *testing.T) {
	const base = "glance_go_app:fixture@tcp(127.0.0.1:3306)/glance_go_shared"
	for _, tc := range []struct {
		name, dsn string
		valid     bool
	}{
		{"loopback", base, true},
		{"socket", "glance_go_app:fixture@unix(/tmp/isolated-mysql.sock)/glance_go_shared", true},
		{"verified TLS remote", "glance_go_app:fixture@tcp(db.example.invalid:3306)/glance_go_shared?tls=true", true},
		{"plaintext remote", "glance_go_app:fixture@tcp(db.example.invalid:3306)/glance_go_shared", false},
		{"unresolved localhost", "glance_go_app:fixture@tcp(localhost:3306)/glance_go_shared", false},
		{"fallback", base + "?tls=true&allowFallbackToPlaintext=true", false},
		{"preferred", base + "?tls=preferred", false},
		{"skip verify", base + "?tls=skip-verify", false},
		{"unknown TLS registration", base + "?tls=custom", false},
		{"old password", base + "?allowOldPasswords=true", false},
		{"cleartext password", base + "?allowCleartextPasswords=true", false},
		{"disabled autocommit", base + "?autocommit=false", false},
		{"zero autocommit", base + "?autocommit=0", false},
		{"ambiguous autocommit", base + "?AUTOCOMMIT=1", false},
		{"enabled autocommit", base + "?autocommit=true", true},
		{"numeric autocommit", base + "?autocommit=1", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := parseDSN(tc.dsn)
			if (err == nil) != tc.valid {
				t.Fatalf("accepted=%v want=%v", err == nil, tc.valid)
			}
			if err == nil && c.Params["autocommit"] != "1" {
				t.Fatal("autocommit not normalized")
			}
		})
	}
}

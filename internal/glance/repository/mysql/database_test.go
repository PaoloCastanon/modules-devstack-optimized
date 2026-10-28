package mysql

import (
	"testing"
)

func TestDSNGuards(t *testing.T) {
	for _, dsn := range []string{
		"root:x@tcp(localhost:3306)/glance_go", "glance:x@tcp(localhost:3306)/glance_go",
		"glance_go_app:x@tcp(localhost:3306)/glance", "glance_go_app:x@tcp(localhost:3306)/placement",
		"glance_go_app:x@tcp(localhost:3306)/glance_go?multiStatements=true",
		"glance_go_app:x@tcp(localhost:3306)/glance_go?tls=skip-verify",
		"glance_go_app:x@tcp(localhost:3306)/glance_go?allowAllFiles=true",
	} {
		if _, err := Open(dsn); err == nil {
			t.Fatalf("accepted unsafe DSN %s", dsn)
		}
	}
	db, err := Open("glance_go_app:fixture@tcp(127.0.0.1:1)/glance_go")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	c, err := parseDSN("glance_go_app:fixture@tcp(127.0.0.1:1)/glance_go")
	if err != nil || !c.ParseTime || c.InterpolateParams || c.ReadTimeout <= 0 {
		t.Fatal("unsafe defaults")
	}
	if err := ValidateTestDSN("glance_go_test:fixture@tcp(127.0.0.1:3306)/glance_go"); err == nil {
		t.Fatal("tests accepted development schema")
	}
	if err := ValidateTestDSN("glance_go_test:fixture@tcp(192.168.122.10:3306)/glance_go_test_a"); err == nil {
		t.Fatal("tests accepted thesis VM")
	}
	if err := ValidateTestDSN("glance_go_test:fixture@tcp(127.0.0.1:3306)/glance_go_test_a"); err != nil {
		t.Fatal(err)
	}
}

func TestGrantGuard(t *testing.T) {
	for _, tc := range []struct {
		grant, mode string
		allowed     bool
	}{
		{"GRANT USAGE ON *.* TO `fixture`@`localhost`", "serve", true},
		{"GRANT SELECT, INSERT ON `glance_go`.* TO `fixture`@`localhost`", "serve", true},
		{"GRANT SELECT, INSERT, UPDATE, CREATE, ALTER, INDEX, REFERENCES ON `glance_go`.* TO `fixture`@`localhost`", "migrate", true},
		{"GRANT CREATE ON `glance_go`.* TO `fixture`@`localhost`", "serve", false},
		{"GRANT ALL PRIVILEGES ON *.* TO `fixture`@`localhost`", "migrate", false},
		{"GRANT SELECT ON `glance`.* TO `fixture`@`localhost`", "serve", false},
		{"GRANT SELECT ON `glance_go`.* TO `fixture`@`localhost` WITH GRANT OPTION", "serve", false},
		{"GRANT `powerful_role` TO `fixture`@`localhost`", "migrate", false},
	} {
		if got := allowedGrant(tc.grant, "glance_go", tc.mode, true); got != tc.allowed {
			t.Fatalf("guard %q = %v", tc.grant, got)
		}
	}
	if allowedGrant("GRANT SELECT ON `glance_go`.* TO `fixture`@`localhost`", "glance_go", "serve", false) {
		t.Fatal("underscore wildcard grant accepted as literal")
	}
	if !allowedGrant("GRANT SELECT ON `glance\\_go`.* TO `fixture`@`localhost`", "glance_go", "serve", false) {
		t.Fatal("escaped literal schema grant rejected")
	}
}

func TestDSNTransportAndAutocommit(t *testing.T) {
	const base = "glance_go_app:fixture@tcp(127.0.0.1:3306)/glance_go"
	for _, tc := range []struct {
		name, dsn string
		valid     bool
	}{
		{"loopback", base, true},
		{"socket", "glance_go_app:fixture@unix(/tmp/isolated-mysql.sock)/glance_go", true},
		{"verified TLS remote", "glance_go_app:fixture@tcp(db.example.invalid:3306)/glance_go?tls=true", true},
		{"plaintext remote", "glance_go_app:fixture@tcp(db.example.invalid:3306)/glance_go", false},
		{"unresolved localhost", "glance_go_app:fixture@tcp(localhost:3306)/glance_go", false},
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

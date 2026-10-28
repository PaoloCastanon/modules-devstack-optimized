package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
)

type identityConnector struct {
	server, name string
	fail         bool
	t            *testing.T
}

func (c identityConnector) Connect(context.Context) (driver.Conn, error) { return identityConn{c}, nil }
func (c identityConnector) Driver() driver.Driver                        { return identityDriver{} }

type identityDriver struct{}

func (identityDriver) Open(string) (driver.Conn, error) { return nil, errors.New("connector only") }

type identityConn struct{ identityConnector }

func (identityConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unexpected SQL") }
func (identityConn) Close() error                        { return nil }
func (identityConn) Begin() (driver.Tx, error)           { return nil, errors.New("unexpected transaction") }
func (c identityConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	if q != "SELECT @@GLOBAL.server_uuid, DATABASE()" {
		c.t.Fatal("unexpected query", q)
	}
	if c.fail {
		return nil, errors.New("SECRET")
	}
	return &identityRows{values: []driver.Value{c.server, c.name}}, nil
}

type identityRows struct{ values []driver.Value }

func (*identityRows) Columns() []string { return []string{"server", "schema"} }
func (*identityRows) Close() error      { return nil }
func (r *identityRows) Next(p []driver.Value) error {
	if r.values == nil {
		return io.EOF
	}
	copy(p, r.values)
	r.values = nil
	return nil
}

func TestDatabaseScopeBindsServerAndSchema(t *testing.T) {
	server := "00000000-0000-4000-8000-000000000001"
	get := func(s, n string, fail bool) (string, error) {
		db := sql.OpenDB(identityConnector{s, n, fail, t})
		defer db.Close()
		return databaseScope(context.Background(), db)
	}
	a, err := get(server, "glance_go", false)
	if err != nil {
		t.Fatal(err)
	}
	b, err := get(server, "glance_go", false)
	if err != nil || a != b {
		t.Fatal("scope not stable")
	}
	b, err = get(server, "glance_go_test_other", false)
	if err != nil || a == b {
		t.Fatal("schema not bound")
	}
	b, err = get("00000000-0000-4000-8000-000000000002", "glance_go", false)
	if err != nil || a == b {
		t.Fatal("server not bound")
	}
	for _, c := range []identityConnector{{server: "bad", name: "glance_go"}, {server: server, name: "glance"}, {server: server, name: "glance_go", fail: true}, {server: "00000000-0000-0000-0000-000000000000", name: "glance_go"}} {
		if _, err = get(c.server, c.name, c.fail); err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatal("unverified binding accepted", err)
		}
	}
}

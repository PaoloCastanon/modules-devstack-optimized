package main

import (
	"context"
	"database/sql"
	"errors"

	"github.com/PaoloCastanon/modules-devstack-optimized/internal/glance/repository/mysql"
	"github.com/google/uuid"
)

type scopeReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Read-only binding to a DIRECT authoritative MySQL primary and its schema.
// It is an identity check, not a backup generation or a replication protocol.
// Recreating a schema with the same name/server UUID requires a NEW empty root;
// restoring/cloning must preserve the SQL+store pair, never mix generations.
func databaseScope(ctx context.Context, db scopeReader) (string, error) {
	var server, name string
	if db.QueryRowContext(ctx, "SELECT @@GLOBAL.server_uuid, DATABASE()").Scan(&server, &name) != nil {
		return "", errors.New("cannot verify SQL/store identity")
	}
	id, err := uuid.Parse(server)
	if err != nil || id == uuid.Nil || id.String() != server || !mysql.AllowedDatabase(name) {
		return "", errors.New("invalid SQL/store identity")
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("glance-go-store-v1\x00"+server+"\x00"+name)).String(), nil
}

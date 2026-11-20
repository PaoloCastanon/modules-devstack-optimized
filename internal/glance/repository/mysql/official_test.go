package mysql

import (
	"strings"
	"testing"
)

func officialRepo(t *testing.T) *Repository {
	t.Helper()
	r := New(nil, 0)
	if err := r.UseOfficialCatalog("glance", "/opt/stack/data/glance/images"); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOwnCatalogSQLIsUnchanged(t *testing.T) {
	r := New(nil, 0)
	for _, q := range []string{readImageQuery, oneImagePropsQuery, tombstoneDataSQL, readDataOperation} {
		if r.sql(q) != q {
			t.Fatalf("own catalog rewrote %q", q)
		}
	}
}

func TestOfficialCatalogQualifiesTablesAndSkipsDeletedChildren(t *testing.T) {
	r := officialRepo(t)
	cases := map[string]string{
		readImageQuery:     "FROM `glance`.images WHERE id=? AND deleted=FALSE",
		oneImagePropsQuery: "FROM `glance`.image_properties WHERE deleted=0 AND image_id IN (?)",
		oneImageTagsQuery:  "FROM `glance`.image_tags WHERE deleted=0 AND image_id IN (?)",
		tombstoneDataSQL:   "UPDATE `glance`.images SET deleted=TRUE",
		"SELECT 1 FROM images i WHERE EXISTS (SELECT 1 FROM image_properties p WHERE p.image_id=i.id) AND EXISTS (SELECT 1 FROM image_tags t WHERE t.image_id=i.id)": "image_properties p WHERE p.deleted=0 AND p.image_id=i.id",
	}
	for q, want := range cases {
		if got := r.sql(q); !strings.Contains(got, want) {
			t.Fatalf("%q\n-> %q\nwant %q", q, got, want)
		}
	}
	// Transfer bookkeeping stays in the paired shared database.
	if got := r.sql(readDataOperation); got != readDataOperation {
		t.Fatalf("data operations must not move: %q", got)
	}
	// Hard deletes of children before re-insert must not skip soft-deleted
	// rows (the original's unique (image_id, name) would collide).
	if got := r.sql("DELETE FROM image_properties WHERE image_id=?"); got != "DELETE FROM `glance`.image_properties WHERE image_id=?" {
		t.Fatalf("delete rewritten as %q", got)
	}
	if r.location("x") != "file:///opt/stack/data/glance/images/x" {
		t.Fatal(r.location("x"))
	}
}

func TestOfficialCatalogConfigurationIsValidated(t *testing.T) {
	for _, c := range [][2]string{{"nova", "/d"}, {"glance", "relative"}, {"glance", "/d/../e"}} {
		if New(nil, 0).UseOfficialCatalog(c[0], c[1]) == nil {
			t.Fatalf("accepted %v", c)
		}
	}
	if OfficialAttempt("84acf0aa-128f-4d38-a3fc-ba5a1eac1740") != OfficialAttempt("84acf0aa-128f-4d38-a3fc-ba5a1eac1740") {
		t.Fatal("adopted attempt must be deterministic")
	}
}

func TestSharedAccountGrants(t *testing.T) {
	ok := []string{
		"GRANT SELECT, INSERT, UPDATE, DELETE ON `glance`.* TO `glance_go_app`@`127.0.0.1`",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON `glance\\_go\\_shared`.* TO `glance_go_app`@`127.0.0.1`",
	}
	for _, g := range ok {
		if !allowedGrant(g, SharedDatabase, "serve", false) {
			t.Fatalf("refused %q", g)
		}
	}
	bad := []string{
		"GRANT ALTER ON `glance`.* TO `x`@`127.0.0.1`",
		"GRANT SELECT ON `nova`.* TO `x`@`127.0.0.1`",
	}
	for _, g := range bad {
		if allowedGrant(g, SharedDatabase, "serve", false) {
			t.Fatalf("accepted %q", g)
		}
	}
	if allowedGrant(ok[0], "glance_go", "serve", false) {
		t.Fatal("own database must not accept official grants")
	}
	if allowedGrant(ok[0], SharedDatabase, "migrate", false) {
		t.Fatal("migration account must not touch the official catalog")
	}
}

func TestSharedMigrationDropsOnlyTheForeignKey(t *testing.T) {
	m := sharedMigrations()
	if len(m) != 1 || strings.Contains(m[0].sql, "REFERENCES images") || !strings.Contains(m[0].sql, "CREATE TABLE image_data_operations") {
		t.Fatal("unexpected shared registry")
	}
	if validateRegistry(m) != nil || m[0].checksum() == migrations()[1].checksum() {
		t.Fatal("shared registry must be valid and distinct")
	}
}

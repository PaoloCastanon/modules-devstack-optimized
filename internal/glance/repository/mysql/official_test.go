package mysql

import (
	"strings"
	"testing"
)

func officialRepo(t *testing.T) *Repository { t.Helper(); return testRepo(nil, 0) }

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
	// The one-round-trip image read also skips deleted children.
	full := r.sql(fullImageQuery)
	for _, want := range []string{"FROM `glance`.images i WHERE i.id=?", "FROM `glance`.image_properties WHERE deleted=0 AND image_id=i.id", "FROM `glance`.image_tags WHERE deleted=0 AND image_id=i.id"} {
		if !strings.Contains(full, want) {
			t.Fatalf("%q missing in %q", want, full)
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
	for _, c := range [][2]string{{"nova", "/d"}, {"glance_go_shared", "/d"}, {"glance", "relative"}, {"glance", "/d/../e"}} {
		if _, err := New(nil, 0, c[0], c[1]); err == nil {
			t.Fatalf("accepted %v", c)
		}
	}
	if _, err := New(nil, 0, "glance_official_test_a", "/srv/images"); err != nil {
		t.Fatal(err)
	}
	if OfficialAttempt("84acf0aa-128f-4d38-a3fc-ba5a1eac1740") != OfficialAttempt("84acf0aa-128f-4d38-a3fc-ba5a1eac1740") {
		t.Fatal("adopted attempt must be deterministic")
	}
}

// The only own table has no foreign key: images lives in the original's database.
func TestOnlyMigrationIsBookkeepingWithoutForeignKey(t *testing.T) {
	m := migrations()
	if len(m) != 1 || strings.Contains(m[0].sql, "REFERENCES") || !strings.Contains(m[0].sql, "CREATE TABLE image_data_operations") || validateRegistry(m) != nil {
		t.Fatal("unexpected registry")
	}
}

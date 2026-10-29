package mysql

import "testing"

func TestMigrationHistoryMustBeContiguousAndUnchanged(t *testing.T) {
	all := migrations()
	if len(all) != 4 {
		t.Fatal("unexpected migration chain")
	}
	for n, step := range all {
		if !historyMatches(n, step.version, step.checksum(), false) {
			t.Fatal("known migration rejected")
		}
	}
	for _, tc := range []struct {
		index   int
		version int64
		hash    string
		dirty   bool
	}{
		{0, 2, all[1].checksum(), false},
		{1, 1, all[0].checksum(), false},
		{0, 1, "changed", false},
		{0, 1, all[0].checksum(), true},
		{4, 5, "future", false},
	} {
		if historyMatches(tc.index, tc.version, tc.hash, tc.dirty) {
			t.Fatal("invalid history accepted", tc)
		}
	}
	if all[1].checksum() != "7eafb891cac6d430240affbaab34007eced9b117801b98e0dc22b134c3da1516" {
		t.Fatal("v2 was changed")
	}
	if all[2].checksum() != "597fcba06ca63ee48bc83297ebf8cd93170186c7ec311e4550b30d33ccc01008" {
		t.Fatal("v3 was changed")
	}
	// v1 was already applied in B07. Its SQL/checksum must not be rewritten.
	if all[0].checksum() != "6608a960a668840dcd3bc94fa536edb5cb0fe83167b06a1cdc3100ebef28455d" {
		t.Fatal("original migration was changed")
	}
}

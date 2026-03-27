package common

import "testing"

func TestAllowedTableAcceptsKnownTables(t *testing.T) {
	t.Parallel()
	for _, table := range objectTables {
		if !allowedTable(table) {
			t.Fatalf("expected %q to be allowed", table)
		}
	}
}

func TestAllowedTableAcceptsObjectTypeToTableValues(t *testing.T) {
	t.Parallel()
	for typ, table := range objectTypeToTable {
		if !allowedTable(table) {
			t.Fatalf("expected %q (type %q) to be allowed", table, typ)
		}
	}
}

func TestAllowedTableRejectsUnknownNames(t *testing.T) {
	t.Parallel()
	badNames := []string{
		"users",
		"DROP TABLE users--",
		"",
		"ff_repository; DROP TABLE users",
		"unknown_table",
	}
	for _, name := range badNames {
		if allowedTable(name) {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
}

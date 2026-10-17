package proxyendpoint

import (
	"context"
	"database/sql"
	"reflect"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// Execute the aggregate against a real SQL catalog fixture, rather than a
// recorder that derives its answer from the requested parameters. Only Dolt's
// two scalar functions are replaced with literals; all aggregation stays SQL.
func TestPostOpenCatalogAggregateMatchesPointCounts(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "mixed schemas", true: "empty schema"}[empty], func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			db.SetMaxOpenConns(1)
			for _, q := range []string{
				"ATTACH DATABASE ':memory:' AS information_schema",
				"CREATE TABLE information_schema.tables (table_schema TEXT, table_name TEXT)",
				"CREATE TABLE information_schema.columns (table_schema TEXT, table_name TEXT, column_name TEXT)",
			} {
				if _, err := db.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			for i, table := range postOpenTables() {
				if _, err := db.Exec("INSERT INTO information_schema.tables VALUES ('foreign', ?)", table); err != nil {
					t.Fatal(err)
				}
				if !empty && i%2 == 0 {
					if _, err := db.Exec("INSERT INTO information_schema.tables VALUES ('current', ?)", table); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := db.Exec("INSERT INTO information_schema.columns VALUES ('foreign', 'leases', 'granted_node')"); err != nil {
				t.Fatal(err)
			}
			if !empty {
				if _, err := db.Exec("INSERT INTO information_schema.columns VALUES ('current', 'leases', 'granted_node')"); err != nil {
					t.Fatal(err)
				}
			}
			query, args := postOpenQuery()
			old := postOpenHeadQuery
			for range postOpenTables() {
				old += ", (SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?)"
			}
			old += ", (SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?)"
			read := func(q string) []any {
				q = strings.ReplaceAll(strings.ReplaceAll(q, "DATABASE()", "'current'"), "DOLT_HASHOF('HEAD')", "'head'")
				values, dest := make([]any, len(postOpenTables())+2), make([]any, len(postOpenTables())+2)
				for i := range values {
					dest[i] = &values[i]
				}
				if err := db.QueryRowContext(context.Background(), q, args...).Scan(dest...); err != nil {
					t.Fatal(err)
				}
				return values
			}
			if got, want := read(query), read(old); !reflect.DeepEqual(got, want) {
				t.Fatalf("aggregate = %v, point counts = %v", got, want)
			}
		})
	}
}

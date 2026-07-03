package mediaindex

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// TestFTS5Available is the blocking prerequisite: modernc.org/sqlite must ship
// with SQLITE_ENABLE_FTS5 compiled in for the whole index design to hold. If
// this fails on any target arch, the FTS path is unavailable and Search() must
// fall back to LIKE. Run under CGO_ENABLED=0 on amd64 AND arm64.
func TestFTS5Available(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE VIRTUAL TABLE t USING fts5(body)`); err != nil {
		t.Fatalf("fts5 create (FTS5 not compiled in?): %v", err)
	}
	if _, err := db.Exec(`INSERT INTO t(body) VALUES ('hello world'), ('the quick brown fox')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM t WHERE t MATCH ?`, `quick*`).Scan(&n); err != nil {
		t.Fatalf("match query: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 match for 'quick*', got %d", n)
	}
}

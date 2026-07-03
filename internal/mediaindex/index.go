// Package mediaindex is a pure-Go SQLite cache of the media folder that makes
// the library searchable (§6). It walks MEDIA_DIR incrementally, extracts audio
// tags (reusing internal/audio), and stores rows in an FTS5-backed SQLite file
// under DataDir. It implements api.Media, so it drops into the API's media seam
// in place of the stateless filesystem lister, adding a Search method.
//
// The DB is a derived cache, never a source of truth: it can be deleted and
// rebuilt from the (possibly read-only) media tree at any time.
package mediaindex

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" database/sql driver
)

// mediaExts mirrors internal/api/media.go (kept local to decouple the packages).
var mediaExts = map[string]bool{".wav": true, ".mp3": true, ".flac": true}

const (
	defaultInterval = 5 * time.Minute
	batchSize       = 500 // files per write transaction, to hold the lock briefly
	maxQueryTokens  = 16
	schemaVersion   = 1
)

// Options configures Open.
type Options struct {
	MediaDir string
	DBPath   string
	Interval time.Duration // rescan cadence; <=0 uses defaultInterval
	Log      *slog.Logger
}

// Index is a SQLite-backed, searchable view of the media folder.
type Index struct {
	mediaDir string
	dbPath   string
	interval time.Duration
	log      *slog.Logger

	wdb *sql.DB // writer: single connection (SQLite is single-writer)
	rdb *sql.DB // reader: read-only pool (WAL → never blocks the writer)
	fts bool    // FTS5 available (else Search falls back to LIKE)

	ready atomic.Bool // true once the first full scan has completed
	done  chan struct{}
	wg    sync.WaitGroup
}

// Open opens (creating/migrating) the index DB and probes FTS5. On a corrupt DB
// it rebuilds once. It does not scan; call Start. A returned error means the
// caller should fall back to the plain filesystem lister.
func Open(opts Options) (*Index, error) {
	if opts.Log == nil {
		opts.Log = slog.Default()
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultInterval
	}
	ix := &Index{
		mediaDir: opts.MediaDir,
		dbPath:   opts.DBPath,
		interval: interval,
		log:      opts.Log.With("comp", "mediaindex"),
		done:     make(chan struct{}),
	}
	if err := ix.open(); err != nil {
		// One rebuild attempt: the DB is a throwaway cache.
		ix.log.Warn("media index unusable, rebuilding", "err", err)
		ix.closeDBs()
		if rmErr := ix.removeDBFiles(); rmErr != nil {
			return nil, fmt.Errorf("open media index: %w (rebuild: %v)", err, rmErr)
		}
		if err2 := ix.open(); err2 != nil {
			return nil, fmt.Errorf("open media index after rebuild: %w", err2)
		}
	}
	return ix, nil
}

func (ix *Index) open() error {
	// Writer: exactly one connection so writes never contend; WAL makes the DB
	// header persist WAL mode for the read-only pool opened next.
	wdb, err := sql.Open("sqlite", ix.dsn(false))
	if err != nil {
		return err
	}
	wdb.SetMaxOpenConns(1)
	if err := wdb.Ping(); err != nil {
		wdb.Close()
		return err
	}
	if err := integrityOK(wdb); err != nil {
		wdb.Close()
		return err
	}
	ix.wdb = wdb
	if err := ix.migrate(); err != nil {
		return err
	}
	ix.fts = ix.probeFTS()
	if ix.fts {
		if err := ix.installTriggers(); err != nil {
			ix.log.Warn("FTS trigger install failed; search falls back to LIKE", "err", err)
			ix.fts = false
		}
	}

	rdb, err := sql.Open("sqlite", ix.dsn(true))
	if err != nil {
		return err
	}
	rdb.SetMaxOpenConns(4)
	if err := rdb.Ping(); err != nil {
		rdb.Close()
		return err
	}
	ix.rdb = rdb
	return nil
}

func (ix *Index) dsn(readonly bool) string {
	// modernc takes PRAGMAs via _pragma= query params.
	p := "file:" + ix.dbPath +
		"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	if readonly {
		p += "&mode=ro&_pragma=query_only(true)"
	}
	return p
}

func integrityOK(db *sql.DB) error {
	var res string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&res); err != nil {
		return err
	}
	if res != "ok" {
		return fmt.Errorf("quick_check: %s", res)
	}
	return nil
}

func (ix *Index) probeFTS() bool {
	_, err := ix.wdb.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS files_fts USING fts5(
		name, path, artist, album, title,
		content='files', content_rowid='id',
		tokenize='unicode61 remove_diacritics 2')`)
	if err != nil {
		ix.log.Warn("FTS5 unavailable, search falls back to LIKE", "err", err)
		return false
	}
	return true
}

func (ix *Index) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS files (
			id INTEGER PRIMARY KEY,
			path TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			size INTEGER NOT NULL,
			modtime INTEGER NOT NULL,
			artist TEXT NOT NULL DEFAULT '',
			album TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL DEFAULT '',
			duration INTEGER NOT NULL DEFAULT 0,
			has_art INTEGER NOT NULL DEFAULT 0,
			indexed_at INTEGER NOT NULL,
			seen_gen INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_files_seen_gen ON files(seen_gen)`,
		fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion),
	}
	for _, s := range stmts {
		if _, err := ix.wdb.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// installTriggers keeps the external-content FTS index in sync. The AFTER UPDATE
// trigger's WHEN guard is load-bearing: incremental rescans re-stamp seen_gen on
// every unchanged file, and without the guard each would delete+reinsert its FTS
// row, turning a no-op pass into a full index rewrite.
func (ix *Index) installTriggers() error {
	stmts := []string{
		`CREATE TRIGGER IF NOT EXISTS files_ai AFTER INSERT ON files BEGIN
			INSERT INTO files_fts(rowid, name, path, artist, album, title)
			VALUES (new.id, new.name, new.path, new.artist, new.album, new.title);
		END`,
		`CREATE TRIGGER IF NOT EXISTS files_ad AFTER DELETE ON files BEGIN
			INSERT INTO files_fts(files_fts, rowid, name, path, artist, album, title)
			VALUES ('delete', old.id, old.name, old.path, old.artist, old.album, old.title);
		END`,
		`CREATE TRIGGER IF NOT EXISTS files_au AFTER UPDATE ON files
			WHEN old.name<>new.name OR old.path<>new.path OR old.artist<>new.artist
			  OR old.album<>new.album OR old.title<>new.title
		BEGIN
			INSERT INTO files_fts(files_fts, rowid, name, path, artist, album, title)
			VALUES ('delete', old.id, old.name, old.path, old.artist, old.album, old.title);
			INSERT INTO files_fts(rowid, name, path, artist, album, title)
			VALUES (new.id, new.name, new.path, new.artist, new.album, new.title);
		END`,
	}
	for _, s := range stmts {
		if _, err := ix.wdb.Exec(s); err != nil {
			return fmt.Errorf("install triggers: %w", err)
		}
	}
	return nil
}

// Start kicks the initial scan in the background (so a huge library never blocks
// daemon startup) and runs the periodic rescan loop until Close or ctx is done.
func (ix *Index) Start(ctx context.Context) {
	ix.wg.Add(1)
	go ix.loop(ctx)
}

func (ix *Index) loop(ctx context.Context) {
	defer ix.wg.Done()

	if err := ix.scan(ctx); err != nil && !errors.Is(err, context.Canceled) {
		ix.log.Warn("initial media scan failed", "err", err)
	}
	ix.ready.Store(true)

	t := time.NewTicker(ix.interval)
	defer t.Stop()
	for {
		select {
		case <-ix.done:
			return
		case <-ctx.Done():
			return
		case <-t.C:
			if err := ix.scan(ctx); err != nil && !errors.Is(err, context.Canceled) {
				ix.log.Warn("media rescan failed", "err", err)
			}
		}
	}
}

// Close stops the loop and releases the DB handles.
func (ix *Index) Close() error {
	select {
	case <-ix.done:
	default:
		close(ix.done)
	}
	ix.wg.Wait()
	ix.closeDBs()
	return nil
}

func (ix *Index) closeDBs() {
	if ix.rdb != nil {
		ix.rdb.Close()
		ix.rdb = nil
	}
	if ix.wdb != nil {
		ix.wdb.Close()
		ix.wdb = nil
	}
}

func (ix *Index) removeDBFiles() error {
	var firstErr error
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(ix.dbPath + suffix); err != nil && !os.IsNotExist(err) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

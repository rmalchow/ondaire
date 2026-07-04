package mediaindex

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ondaire/internal/audio"
)

type fileEntry struct {
	path string // rel to mediaDir, slash-separated
	name string
	size int64
	mod  int64
}

type scanStats struct {
	seen    int // media files found on disk
	indexed int // new or changed → tags (re)read
	touched int // unchanged → seen_gen re-stamped only
	removed int // rows deleted for vanished files
}

// scan runs one incremental pass and logs a summary.
func (ix *Index) scan(ctx context.Context) error {
	st, err := ix.scanOnce(ctx)
	if err != nil {
		return err
	}
	ix.log.Debug("media scan", "seen", st.seen, "indexed", st.indexed,
		"touched", st.touched, "removed", st.removed)
	return nil
}

// scanOnce walks MEDIA_DIR, upserts new/changed files (re-reading tags only when
// size or modtime changed), and mark-and-sweeps rows whose files vanished. The
// sweep runs only after a clean walk, so a transient read error can't wipe rows.
func (ix *Index) scanOnce(ctx context.Context) (scanStats, error) {
	var st scanStats
	gen := time.Now().UnixNano()

	batch := make([]fileEntry, 0, batchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := ix.flushBatch(ctx, batch, gen, &st); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}

	walkErr := filepath.WalkDir(ix.mediaDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // missing media dir → empty index, not an error
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !mediaExts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		rel, rerr := filepath.Rel(ix.mediaDir, p)
		if rerr != nil {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		st.seen++
		batch = append(batch, fileEntry{
			path: filepath.ToSlash(rel),
			name: d.Name(),
			size: info.Size(),
			mod:  info.ModTime().Unix(),
		})
		if len(batch) >= batchSize {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return flush()
		}
		return nil
	})
	if walkErr != nil {
		return st, walkErr
	}
	if err := flush(); err != nil {
		return st, err
	}

	// Sweep files that were not seen this generation.
	res, err := ix.wdb.ExecContext(ctx, `DELETE FROM files WHERE seen_gen<>?`, gen)
	if err != nil {
		return st, err
	}
	if n, e := res.RowsAffected(); e == nil {
		st.removed = int(n)
	}
	return st, nil
}

// flushBatch upserts one batch in a single transaction (short lock hold).
func (ix *Index) flushBatch(ctx context.Context, batch []fileEntry, gen int64, st *scanStats) error {
	tx, err := ix.wdb.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit

	sel, err := tx.PrepareContext(ctx, `SELECT size, modtime FROM files WHERE path=?`)
	if err != nil {
		return err
	}
	defer sel.Close()
	touch, err := tx.PrepareContext(ctx, `UPDATE files SET seen_gen=?, indexed_at=? WHERE path=?`)
	if err != nil {
		return err
	}
	defer touch.Close()
	up, err := tx.PrepareContext(ctx, `INSERT INTO files
		(path,name,size,modtime,artist,album,title,duration,has_art,indexed_at,seen_gen)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET
			name=excluded.name, size=excluded.size, modtime=excluded.modtime,
			artist=excluded.artist, album=excluded.album, title=excluded.title,
			duration=excluded.duration, has_art=excluded.has_art,
			indexed_at=excluded.indexed_at, seen_gen=excluded.seen_gen`)
	if err != nil {
		return err
	}
	defer up.Close()

	now := time.Now().Unix()
	for _, e := range batch {
		var size, mod int64
		serr := sel.QueryRowContext(ctx, e.path).Scan(&size, &mod)
		switch {
		case serr == nil && size == e.size && mod == e.mod:
			if _, err := touch.ExecContext(ctx, gen, now, e.path); err != nil {
				return err
			}
			st.touched++
		case serr == nil || errors.Is(serr, sql.ErrNoRows):
			md, _ := audio.Probe(ctx, "file:"+e.path, ix.mediaDir) // tags + folder cover
			if _, err := up.ExecContext(ctx, e.path, e.name, e.size, e.mod,
				md.Artist, md.Album, md.Title, md.DurationSec, boolToInt(md.HasArt),
				now, gen); err != nil {
				return err
			}
			st.indexed++
		default:
			return serr
		}
	}
	return tx.Commit()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

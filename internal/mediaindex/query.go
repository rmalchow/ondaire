package mediaindex

import (
	"cmp"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"ensemble/internal/api"
	"ensemble/internal/audio"
)

// selectCols is the column list shared by List and Search, in MediaFile order.
const selectCols = `path, name, size, modtime, artist, album, title, duration, has_art`

// List returns every indexed file, sorted by path. Before the first scan
// completes it falls back to a live directory walk, so a cold start on a large
// library never shows an empty library.
func (ix *Index) List() ([]api.MediaFile, error) {
	if !ix.ready.Load() {
		return ix.liveWalk()
	}
	rows, err := ix.rdb.Query(`SELECT ` + selectCols + ` FROM files ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

// Search returns files matching q, ranked (FTS bm25) then by path. An empty or
// all-punctuation query yields no results.
func (ix *Index) Search(q string, limit, offset int) ([]api.MediaFile, error) {
	limit, offset = clamp(limit, offset)
	if ix.fts {
		match := ftsMatch(q)
		if match == "" {
			return []api.MediaFile{}, nil
		}
		rows, err := ix.rdb.Query(`SELECT `+prefixed("f", selectCols)+`
			FROM files_fts JOIN files f ON f.id = files_fts.rowid
			WHERE files_fts MATCH ?
			ORDER BY bm25(files_fts), f.path
			LIMIT ? OFFSET ?`, match, limit, offset)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return scanRows(rows)
	}
	// LIKE fallback (FTS5 unavailable).
	term := strings.TrimSpace(q)
	if term == "" {
		return []api.MediaFile{}, nil
	}
	like := "%" + term + "%"
	rows, err := ix.rdb.Query(`SELECT `+selectCols+` FROM files
		WHERE name LIKE ? OR path LIKE ? OR artist LIKE ? OR album LIKE ? OR title LIKE ?
		ORDER BY path COLLATE NOCASE LIMIT ? OFFSET ?`,
		like, like, like, like, like, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

// Cover resolves cover art from disk (sibling image preferred, else embedded),
// reusing the audio package's path guard. The index only advertises has_art;
// bytes always come from the file, so there's no cache to invalidate.
func (ix *Index) Cover(uri string) (data []byte, contentType string, ok bool) {
	return audio.CoverArt(uri, ix.mediaDir)
}

func scanRows(rows *sql.Rows) ([]api.MediaFile, error) {
	out := []api.MediaFile{}
	for rows.Next() {
		var f api.MediaFile
		var hasArt int
		if err := rows.Scan(&f.Path, &f.Name, &f.SizeBytes, &f.ModTime,
			&f.Artist, &f.Album, &f.Title, &f.DurationSec, &hasArt); err != nil {
			return nil, err
		}
		f.HasArt = hasArt != 0
		out = append(out, f)
	}
	return out, rows.Err()
}

// prefixed qualifies a comma-separated column list with a table alias.
func prefixed(alias, cols string) string {
	parts := strings.Split(cols, ",")
	for i, p := range parts {
		parts[i] = alias + "." + strings.TrimSpace(p)
	}
	return strings.Join(parts, ", ")
}

func clamp(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// ftsMatch turns a free-text query into a safe FTS5 MATCH expression: each
// whitespace token becomes a quoted prefix term ("tok"*), so FTS5 operators in
// user input are inert and every token is an implicit-AND prefix match. Returns
// "" when nothing usable remains (caller must not run a bare MATCH ”).
func ftsMatch(q string) string {
	fields := strings.Fields(q)
	if len(fields) > maxQueryTokens {
		fields = fields[:maxQueryTokens]
	}
	var terms []string
	for _, tok := range fields {
		// Drop tokens that are only FTS separators/punctuation.
		if strings.Trim(tok, `"*^:()-+.`) == "" {
			continue
		}
		esc := strings.ReplaceAll(tok, `"`, `""`) // escape embedded quotes
		terms = append(terms, `"`+esc+`"*`)
	}
	return strings.Join(terms, " ")
}

// liveWalk mirrors api.fsLister.List for the pre-ready cold-start window.
func (ix *Index) liveWalk() ([]api.MediaFile, error) {
	out := []api.MediaFile{}
	err := filepath.WalkDir(ix.mediaDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !mediaExts[strings.ToLower(filepath.Ext(d.Name()))] {
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
		out = append(out, api.MediaFile{
			Path:      filepath.ToSlash(rel),
			Name:      d.Name(),
			SizeBytes: info.Size(),
			ModTime:   info.ModTime().Unix(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(out, func(a, b api.MediaFile) int { return cmp.Compare(a.Path, b.Path) })
	return out, nil
}

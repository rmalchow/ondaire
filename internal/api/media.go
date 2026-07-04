package api

import (
	"cmp"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"ondaire/internal/audio"
)

// mediaExts are the playable file extensions (§6, lowercase, with dot).
var mediaExts = map[string]bool{
	".wav":  true,
	".mp3":  true,
	".flac": true,
}

// fsLister implements Media by walking a media directory (§6). Paths in the
// result are relative to the directory root, with traversal kept inside it.
type fsLister struct {
	dir string
}

// NewMediaLister returns a Media implementation that recursively scans dir for
// playable files (§6 extensions .wav/.mp3/.flac), rescanned on each List call.
func NewMediaLister(dir string) Media {
	return &fsLister{dir: dir}
}

// List walks the media directory and returns playable files, sorted by path.
// A missing directory yields an empty list (not an error): a node may have no
// media. Symlink loops are bounded by WalkDir's own handling.
func (l *fsLister) List() ([]MediaFile, error) {
	var out []MediaFile
	err := filepath.WalkDir(l.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !mediaExts[strings.ToLower(filepath.Ext(d.Name()))] {
			return nil
		}
		rel, rerr := filepath.Rel(l.dir, p)
		if rerr != nil {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		out = append(out, MediaFile{
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
	slices.SortFunc(out, func(a, b MediaFile) int { return cmp.Compare(a.Path, b.Path) })
	return out, nil
}

// Search filters List() by a case-insensitive substring of name or path. The
// plain lister has no tag metadata; a media index (§6) provides richer matching.
// An empty query returns nothing (matches the index's contract).
func (l *fsLister) Search(q string, limit, offset int) ([]MediaFile, error) {
	term := strings.ToLower(strings.TrimSpace(q))
	if term == "" {
		return []MediaFile{}, nil
	}
	all, err := l.List()
	if err != nil {
		return nil, err
	}
	out := make([]MediaFile, 0, limit)
	skipped := 0
	for _, f := range all {
		if !strings.Contains(strings.ToLower(f.Name), term) &&
			!strings.Contains(strings.ToLower(f.Path), term) {
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		out = append(out, f)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// Cover resolves uri under the media root and returns its cover image (sibling
// cover.jpg/png/… preferred, else the embedded picture). Path resolution + the
// traversal guard live in the audio package (single source of truth for file:
// URIs); ok=false for non-file URIs, traversal, or no art.
func (l *fsLister) Cover(uri string) (data []byte, contentType string, ok bool) {
	return audio.CoverArt(uri, l.dir)
}

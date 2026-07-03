package mediaindex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ensemble/internal/api"
)

// --- helpers ---------------------------------------------------------------

// id3v2 builds a minimal ID3v2.3 tag with the given TEXT frames (mirrors the
// helper in internal/audio) so Probe has real tags to read.
func id3v2(frames ...[2]string) []byte {
	var body []byte
	for _, fr := range frames {
		payload := append([]byte{0x00}, []byte(fr[1])...) // 0x00 = ISO-8859-1
		sz := len(payload)
		body = append(body, fr[0][0], fr[0][1], fr[0][2], fr[0][3],
			byte(sz>>24), byte(sz>>16), byte(sz>>8), byte(sz), 0, 0)
		body = append(body, payload...)
	}
	n := len(body)
	ss := []byte{byte((n >> 21) & 0x7f), byte((n >> 14) & 0x7f), byte((n >> 7) & 0x7f), byte(n & 0x7f)}
	head := append([]byte{'I', 'D', '3', 0x03, 0x00, 0x00}, ss...)
	return append(head, body...)
}

func write(t *testing.T, dir, rel string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// openTest opens an index over mediaDir with a temp DB, closed by t.Cleanup. It
// does not scan.
func openTest(t *testing.T, mediaDir string) *Index {
	t.Helper()
	ix, err := Open(Options{
		MediaDir: mediaDir,
		DBPath:   filepath.Join(t.TempDir(), "media.db"),
		Interval: time.Hour,
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { ix.Close() })
	return ix
}

// reindex runs one synchronous scan and marks the index ready (so List reads the
// DB rather than the cold-start live walk), returning the scan stats.
func reindex(t *testing.T, ix *Index) scanStats {
	t.Helper()
	st, err := ix.scanOnce(context.Background())
	if err != nil {
		t.Fatalf("scanOnce: %v", err)
	}
	ix.ready.Store(true)
	return st
}

func paths(files []api.MediaFile) []string {
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.Path
	}
	return out
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// --- tests -----------------------------------------------------------------

func TestListParity(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "song.flac", []byte("x"))
	write(t, dir, "sub/track.mp3", []byte("x"))
	write(t, dir, "a.wav", []byte("x"))
	write(t, dir, "notes.txt", []byte("x"))  // skipped
	write(t, dir, "cover.jpg", []byte("x"))  // skipped
	write(t, dir, "sub/x.FLAC", []byte("x")) // case-insensitive ext

	ix := openTest(t, dir)
	reindex(t, ix)

	files, err := ix.List()
	if err != nil {
		t.Fatal(err)
	}
	got := paths(files)
	want := []string{"a.wav", "song.flac", "sub/track.mp3", "sub/x.FLAC"}
	if len(got) != len(want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List()[%d] = %q, want %q (full %v)", i, got[i], want[i], got)
		}
	}
}

func TestSearchFilenameAndTags(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "01.mp3", id3v2(
		[2]string{"TIT2", "So What"},
		[2]string{"TPE1", "Miles Davis"},
		[2]string{"TALB", "Kind of Blue"}))
	write(t, dir, "beethoven-symphony.flac", []byte("not really flac"))

	ix := openTest(t, dir)
	reindex(t, ix)

	cases := []struct {
		q    string
		want string // expected path in results ("" = expect none)
	}{
		{"miles", "01.mp3"},                      // artist
		{"blue", "01.mp3"},                       // album token
		{"so what", "01.mp3"},                    // title, multi-token AND
		{"beethoven", "beethoven-symphony.flac"}, // filename
		{"symph", "beethoven-symphony.flac"},     // prefix match
		{"nonexistentzzz", ""},                   // no hit
	}
	for _, c := range cases {
		res, err := ix.Search(c.q, 50, 0)
		if err != nil {
			t.Fatalf("Search(%q): %v", c.q, err)
		}
		if c.want == "" {
			if len(res) != 0 {
				t.Errorf("Search(%q) = %v, want none", c.q, paths(res))
			}
			continue
		}
		if !contains(paths(res), c.want) {
			t.Errorf("Search(%q) = %v, want to contain %q", c.q, paths(res), c.want)
		}
	}
}

func TestIncrementalNoReparseThenChange(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.mp3", id3v2([2]string{"TPE1", "First Artist"}))
	write(t, dir, "b.flac", []byte("x"))

	ix := openTest(t, dir)
	st := reindex(t, ix)
	if st.indexed != 2 {
		t.Fatalf("first scan indexed = %d, want 2", st.indexed)
	}

	// A no-op rescan must re-read no tags.
	st = reindex(t, ix)
	if st.indexed != 0 || st.touched != 2 {
		t.Fatalf("no-op rescan indexed=%d touched=%d, want 0/2", st.indexed, st.touched)
	}

	// Change a file's content + modtime → it must be re-indexed with new tags.
	p := write(t, dir, "a.mp3", id3v2([2]string{"TPE1", "Second Artist"}))
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	st = reindex(t, ix)
	if st.indexed != 1 {
		t.Fatalf("post-change rescan indexed = %d, want 1", st.indexed)
	}
	if res, _ := ix.Search("second", 10, 0); !contains(paths(res), "a.mp3") {
		t.Errorf(`Search("second") did not find the re-tagged a.mp3: %v`, paths(res))
	}
	if res, _ := ix.Search("first", 10, 0); contains(paths(res), "a.mp3") {
		t.Errorf(`Search("first") still matches the old tag`)
	}
}

func TestDeletionSweep(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "keep.flac", []byte("x"))
	gone := write(t, dir, "gone.mp3", []byte("x"))

	ix := openTest(t, dir)
	reindex(t, ix)
	if res, _ := ix.Search("gone", 10, 0); !contains(paths(res), "gone.mp3") {
		t.Fatalf("precondition: gone.mp3 should be indexed")
	}

	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	st := reindex(t, ix)
	if st.removed != 1 {
		t.Fatalf("removed = %d, want 1", st.removed)
	}
	if files, _ := ix.List(); contains(paths(files), "gone.mp3") {
		t.Errorf("List still contains gone.mp3: %v", paths(files))
	}
	if res, _ := ix.Search("gone", 10, 0); len(res) != 0 {
		t.Errorf("Search still finds gone.mp3: %v", paths(res))
	}
}

func TestSearchSanitization(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "song.flac", []byte("x"))
	ix := openTest(t, dir)
	reindex(t, ix)

	// FTS operator/punctuation input must never error.
	for _, q := range []string{`"`, `*`, `-`, `OR`, `(`, `song" OR 1=1`, `   `, ``, `^:()`} {
		if _, err := ix.Search(q, 10, 0); err != nil {
			t.Errorf("Search(%q) errored: %v", q, err)
		}
	}
	// Empty/whitespace/punct-only → no results.
	for _, q := range []string{"", "   ", `"*"`} {
		if res, _ := ix.Search(q, 10, 0); len(res) != 0 {
			t.Errorf("Search(%q) = %v, want none", q, paths(res))
		}
	}
}

func TestOpenUnwritablePathErrors(t *testing.T) {
	// A DB path whose parent is a regular file cannot be opened → Open errors so
	// main falls back to the filesystem lister.
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ix, err := Open(Options{MediaDir: t.TempDir(), DBPath: filepath.Join(f, "media.db")}); err == nil {
		ix.Close()
		t.Fatal("Open on unwritable path should have errored")
	}
}

func TestCorruptDBRebuilds(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "song.flac", []byte("x"))
	dbPath := filepath.Join(t.TempDir(), "media.db")
	if err := os.WriteFile(dbPath, []byte("this is not a sqlite database at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	ix, err := Open(Options{MediaDir: dir, DBPath: dbPath, Interval: time.Hour})
	if err != nil {
		t.Fatalf("Open should rebuild a corrupt DB, got: %v", err)
	}
	defer ix.Close()
	reindex(t, ix)
	if files, _ := ix.List(); len(files) != 1 {
		t.Fatalf("after rebuild List() = %d files, want 1", len(files))
	}
}

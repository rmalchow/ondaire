package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMediaListerWalksAndFilters(t *testing.T) {
	dir := t.TempDir()
	must := func(rel string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	must("song.flac")
	must("sub/track.mp3")
	must("a.wav")
	must("notes.txt")  // skipped
	must("cover.jpg")  // skipped
	must("sub/x.FLAC") // case-insensitive ext

	l := NewMediaLister(dir)
	files, err := l.List()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range files {
		got[f.Path] = true
	}
	want := []string{"a.wav", "song.flac", "sub/track.mp3", "sub/x.FLAC"}
	if len(files) != len(want) {
		t.Fatalf("got %d files %v, want %d", len(files), files, len(want))
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing %q in %v", w, got)
		}
	}
	// Sorted by path.
	for i := 1; i < len(files); i++ {
		if files[i-1].Path > files[i].Path {
			t.Errorf("not sorted: %q before %q", files[i-1].Path, files[i].Path)
		}
	}
	// Metadata present.
	if files[0].Name == "" || files[0].SizeBytes == 0 || files[0].ModTime == 0 {
		t.Errorf("metadata missing: %+v", files[0])
	}
}

func TestMediaListerMissingDir(t *testing.T) {
	l := NewMediaLister(filepath.Join(t.TempDir(), "does-not-exist"))
	files, err := l.List()
	if err != nil {
		t.Fatalf("missing dir should not error: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("missing dir should yield empty list, got %v", files)
	}
}

func TestMediaListerSearch(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"jazz/miles.flac", "rock/queen.mp3", "jazz/coltrane.mp3"} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	l := NewMediaLister(dir)

	// Substring on name or path, case-insensitive.
	if res, _ := l.Search("MILES", 10, 0); len(res) != 1 || res[0].Name != "miles.flac" {
		t.Errorf(`Search("MILES") = %+v, want just miles.flac`, res)
	}
	if res, _ := l.Search("jazz", 10, 0); len(res) != 2 { // matches the folder in path
		t.Errorf(`Search("jazz") matched %d, want 2`, len(res))
	}
	// Empty query → nothing (matches the index contract).
	if res, _ := l.Search("", 10, 0); len(res) != 0 {
		t.Errorf(`Search("") = %+v, want none`, res)
	}
	// limit + offset.
	if res, _ := l.Search("jazz", 1, 1); len(res) != 1 {
		t.Errorf("Search jazz limit=1 offset=1 returned %d, want 1", len(res))
	}
}

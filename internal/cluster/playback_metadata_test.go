package cluster

import (
	"testing"

	"ensemble/internal/contracts"
)

// Metadata (D57 now-playing) survives the contracts.Playback ↔ PlaybackRecord
// round-trip — it was dropped before, leaving the UI showing just "Spotify".
func TestPlaybackRecordCarriesMetadata(t *testing.T) {
	md := &contracts.TrackMetadata{Title: "Another Day in Paradise", Artist: "Phil Collins", Album: "...But Seriously"}
	rec := &PlaybackRecord{State: "playing", URI: "spotify:", Metadata: md}

	pb := resolvePlayback(rec)
	if pb.Metadata == nil {
		t.Fatal("resolvePlayback dropped metadata")
	}
	if pb.Metadata.Title != md.Title || pb.Metadata.Artist != md.Artist || pb.Metadata.Album != md.Album {
		t.Fatalf("metadata mismatch: %+v", pb.Metadata)
	}
}

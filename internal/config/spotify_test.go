package config

import (
	"testing"

	"ondaire/internal/contracts"
	"ondaire/internal/id"
)

func TestSetSpotifyEndpointsNormalizesAndPersists(t *testing.T) {
	s := NewStore(t.TempDir())
	nf, err := s.LoadOrCreate("lr")
	if err != nil {
		t.Fatal(err)
	}
	p := id.ID{}
	p[15] = 7

	in := []contracts.SpotifyEndpoint{
		{Name: "  Kitchen  ", Players: []id.ID{p, p, {}}}, // trimmed name; dup + zero player dropped
		{Name: "", Players: []id.ID{p}},                   // nameless → dropped
		{Name: "Kitchen"},                                 // slug collides with the first → -2 suffix
	}
	out, err := s.SetSpotifyEndpoints(nf.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	got := out.SpotifyEndpoints
	if len(got) != 2 {
		t.Fatalf("want 2 endpoints, got %d (%+v)", len(got), got)
	}
	if got[0].ID != "kitchen" || got[0].Name != "Kitchen" {
		t.Fatalf("ep0 = %+v", got[0])
	}
	if len(got[0].Players) != 1 || got[0].Players[0] != p {
		t.Fatalf("ep0 players = %v (want one, deduped, no zero)", got[0].Players)
	}
	if got[1].ID != "kitchen-2" {
		t.Fatalf("ep1 id = %q, want kitchen-2 (collision suffix)", got[1].ID)
	}

	// Persisted: reload sees the normalized list.
	back, err := s.LoadOrCreate("")
	if err != nil {
		t.Fatal(err)
	}
	if len(back.SpotifyEndpoints) != 2 || back.SpotifyEndpoints[0].ID != "kitchen" {
		t.Fatalf("reload = %+v", back.SpotifyEndpoints)
	}
}

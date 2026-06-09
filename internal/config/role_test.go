package config

import "testing"

func TestParseRole(t *testing.T) {
	both := Role{Master: true, Playback: true}
	cases := []struct {
		in   string
		want Role
		err  bool
	}{
		{"", both, false}, // default
		{"master", Role{Master: true}, false},
		{"playback", Role{Playback: true}, false},
		{"master,playback", both, false},
		{"playback,master", both, false},
		{"  Master , Playback ", both, false}, // case + whitespace insensitive
		{"both", both, false},
		{"all", both, false},
		{"master,", Role{Master: true}, false}, // trailing comma tolerated
		{"speaker", Role{}, true},              // unknown
		{",", Role{}, true},                    // no roles
	}
	for _, c := range cases {
		got, err := ParseRole(c.in)
		if c.err {
			if err == nil {
				t.Errorf("ParseRole(%q): want error, got %+v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseRole(%q): unexpected error %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseRole(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestRoleString(t *testing.T) {
	if s := (Role{Master: true, Playback: true}).String(); s != "master,playback" {
		t.Errorf("both.String() = %q", s)
	}
	if s := (Role{Playback: true}).String(); s != "playback" {
		t.Errorf("playback.String() = %q", s)
	}
	if s := (Role{Master: true}).String(); s != "master" {
		t.Errorf("master.String() = %q", s)
	}
}

func TestDefaultRoleIsBoth(t *testing.T) {
	if r := DefaultRole(); !r.Master || !r.Playback {
		t.Fatalf("DefaultRole = %+v, want both", r)
	}
}

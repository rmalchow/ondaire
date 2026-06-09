package config

import (
	"fmt"
	"sort"
	"strings"
)

// Role is the set of independently-enableable node roles (D49): "master" (gossips,
// owns cluster state, serves the API + SPA, sources audio) and "playback" (the
// receive-and-play role driven over the control plane / locally). A node runs
// either or both; the default is both. Role is runtime configuration — it is NOT a
// replicated cluster field; the *advertised* role goes into the mDNS TXT (D50/D51).
type Role struct {
	Master   bool
	Playback bool
}

// DefaultRole is both roles enabled (D49: "default both").
func DefaultRole() Role { return Role{Master: true, Playback: true} }

// String renders the role set as the canonical "master,playback" form (sorted,
// matching the mDNS TXT `role` value). Empty set renders as "" (never valid).
func (r Role) String() string {
	var parts []string
	if r.Master {
		parts = append(parts, "master")
	}
	if r.Playback {
		parts = append(parts, "playback")
	}
	return strings.Join(parts, ",")
}

// ParseRole parses a comma/space-separated role list ("master", "playback",
// "master,playback", "both"/"all" as aliases for both). Case-insensitive, order-
// and whitespace-insensitive. An empty string yields the default (both). At least
// one valid role must result, else an error — a node with no role does nothing.
func ParseRole(s string) (Role, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultRole(), nil
	}
	var r Role
	fields := strings.FieldsFunc(s, func(c rune) bool { return c == ',' || c == ' ' })
	for _, f := range fields {
		switch strings.ToLower(strings.TrimSpace(f)) {
		case "master":
			r.Master = true
		case "playback":
			r.Playback = true
		case "both", "all":
			r.Master, r.Playback = true, true
		case "":
			// skip empty field from a trailing comma
		default:
			return Role{}, fmt.Errorf("config: unknown role %q (want master|playback)", f)
		}
	}
	if !r.Master && !r.Playback {
		return Role{}, fmt.Errorf("config: role %q resolves to no roles", s)
	}
	return r, nil
}

// roleList is a small helper for tests / logging: the enabled roles, sorted.
func (r Role) roleList() []string {
	var out []string
	if r.Master {
		out = append(out, "master")
	}
	if r.Playback {
		out = append(out, "playback")
	}
	sort.Strings(out)
	return out
}

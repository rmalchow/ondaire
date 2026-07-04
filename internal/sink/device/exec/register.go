package exec

import (
	"log/slog"

	"ondaire/internal/sink/device"
)

// init wires the exec adapter into the device registry. No enumerator: exec has
// no host device list (it is a tool on $PATH, not a card), so it contributes
// nothing to the UI device picker.
func init() {
	device.Register("exec", factory, available)
	device.RegisterCandidates("exec", candidates)
}

// factory builds an exec sink from the spec arg (the part after the colon in
// ONDAIRE_OUTPUT). The arg selects between the two operating modes:
//
//	arg == ""         STANDALONE: auto-pick the first player on $PATH and self-heal
//	                  on write failure by respawning (ONDAIRE_OUTPUT=exec / auto).
//	arg == "<tool>"   FAILOVER-CANDIDATE: pin that specific tool with NO internal
//	                  respawn — the resilient chain owns retry, so a player death
//	                  surfaces as a write error and the chain rotates outputs.
func factory(arg string, log *slog.Logger) (device.Sink, error) {
	if arg == "" {
		return open("", true, log) // standalone: auto-pick + self-respawn
	}
	return open(arg, false, log) // pinned candidate: no internal respawn
}

// available reports whether any player tool is on $PATH (drives the registry's
// HasPlayback / capability checks). Pure lookup; no process spawn.
func available() bool {
	_, _, ok := lookExecTool()
	return ok
}

// candidates yields one failover candidate per player tool actually present on
// $PATH, in execTools preference order. The preferred arg is ignored: exec has no
// operator-selectable device, only a fixed tool preference order.
func candidates(preferred string) []device.Candidate {
	var out []device.Candidate
	for _, t := range execTools {
		if _, _, ok := lookExecToolNamed(t.name); !ok {
			continue
		}
		out = append(out, device.Candidate{
			Kind:  "exec",
			Arg:   t.name,
			Label: "exec(" + t.name + ")",
		})
	}
	return out
}

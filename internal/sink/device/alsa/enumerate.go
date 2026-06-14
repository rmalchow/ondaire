package alsa

import (
	"os"
	"strings"

	"ensemble/internal/contracts"
	"ensemble/internal/sink/device"
)

// procPCMPath is the kernel's ALSA PCM listing. Overridable in tests.
var procPCMPath = "/proc/asound/pcm"

// ListOutputDevices enumerates the host's ALSA playback devices (D37, §8.5),
// registered as the "alsa" enumerator. It returns a "default" entry (system
// default) followed by every playback-capable PCM parsed from /proc/asound/pcm. The
// list is empty when the alsa backend never bound (libasound absent — device
// selection is meaningless) OR /proc/asound/pcm is missing. Zero external deps.
func ListOutputDevices() []contracts.OutputDevice {
	if bound == nil {
		return nil
	}
	data, err := os.ReadFile(procPCMPath)
	if err != nil {
		return nil
	}
	return parseProcPCM(string(data))
}

// parseProcPCM parses /proc/asound/pcm content into the playback-capable device
// list, prepending the "default" entry. Pure, for testability.
//
// Each line looks like (fields colon-separated, the trailing playback/capture
// markers optional):
//
//	00-00: ALC892 Analog : ALC892 Analog : playback 1 : capture 1
//	01-03: HDMI 0 : HDMI 0 : playback 1
//
// The leading "CC-DD" is the card-device pair → "hw:C,D". Only lines that
// advertise "playback" are included. Returns nil when no playback device parses
// (so an empty proc file yields an empty list, not a lone "default").
func parseProcPCM(content string) []contracts.OutputDevice {
	var devs []contracts.OutputDevice
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 2 {
			continue
		}
		// Field 0 is "CC-DD <maybe id>"; the leading token is the card-device.
		head := strings.Fields(strings.TrimSpace(fields[0]))
		if len(head) == 0 {
			continue
		}
		cardDev := head[0]
		dash := strings.IndexByte(cardDev, '-')
		if dash <= 0 || dash >= len(cardDev)-1 {
			continue
		}
		card := strings.TrimLeft(cardDev[:dash], "0")
		if card == "" {
			card = "0"
		}
		dev := strings.TrimLeft(cardDev[dash+1:], "0")
		if dev == "" {
			dev = "0"
		}

		// Playback-capable only.
		playback := false
		for _, f := range fields {
			if strings.Contains(strings.ToLower(f), "playback") {
				playback = true
				break
			}
		}
		if !playback {
			continue
		}

		// Description: the id field (index 1), trimmed.
		desc := strings.TrimSpace(fields[1])
		if desc == "" {
			desc = cardDev
		}
		devs = append(devs, contracts.OutputDevice{
			ID:   "hw:" + card + "," + dev,
			Desc: desc,
		})
	}
	if len(devs) == 0 {
		return nil
	}
	out := make([]contracts.OutputDevice, 0, len(devs)+1)
	out = append(out, contracts.OutputDevice{ID: "default", Desc: "system default"})
	out = append(out, devs...)
	return out
}

// candidates yields the alsa failover chain (device.CandidateProvider), preferred
// device first: the operator/UI-selected device (D37) if any, then "default", then
// every enumerated hw:C,D, deduped stable by id. The resilient backend dedupes
// across providers too, but we dedupe here so a preferred="default" or a preferred
// that re-appears in the enumeration never yields a doubled entry.
func candidates(preferred string) []device.Candidate {
	var out []device.Candidate
	seen := map[string]bool{}
	add := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, device.Candidate{
			Kind:  "alsa",
			Arg:   id,
			Label: "alsa(" + id + ")",
		})
	}

	add(preferred) // operator/UI override first (D37)
	add("default") // then the system default
	for _, d := range ListOutputDevices() {
		add(d.ID) // then each enumerated device (includes "default" — already deduped)
	}
	return out
}

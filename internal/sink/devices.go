package sink

import (
	"os"
	"strings"

	"ensemble/internal/contracts"
)

// procPCMPath is the kernel's ALSA PCM listing. Overridable in tests.
var procPCMPath = "/proc/asound/pcm"

// ListOutputDevices enumerates the host's ALSA playback devices (D37, §8.5).
// It returns a "default" entry (system default) followed by every
// playback-capable PCM parsed from /proc/asound/pcm. The list is empty when
// libasound is not loadable (the alsa backend never registered, so device
// selection is meaningless) OR /proc/asound/pcm is missing. Zero external deps.
func ListOutputDevices() []contracts.OutputDevice {
	if !isRegistered("alsa") {
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

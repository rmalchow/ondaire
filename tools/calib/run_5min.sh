#!/bin/bash
# First hardware validation of the DAC-pull phase-lock servo (D66): one 5-minute
# dual-tone capture. Records the mic + plays calib_tones.wav on the master + polls
# /api/playback/statuses at 1 Hz (with a PLAY marker for compare_drift.py's t0).
# The stats log carries the new servo telemetry (ratePPM/phaseErrNs/deviceDelayNs/
# calibrated) per playback node — that is the primary convergence signal; the WAV
# is the acoustic cross-check. Run in background.
set -u
cd /home/rm/Git/gitlab.rand0m.me/share/ondaire
MASTER=http://192.168.71.63:8080
WAV=tools/calib/results/tones_5min.wav
LOG=tools/calib/results/tones_stats_5min.jsonl
DUR=315        # record seconds (~5 min + lead/margin)
POLLS=310      # ~1 Hz polls

: > "$LOG"
# Recorder first so the onset is captured.
arecord -d "$DUR" -f S16_LE -c 2 -r 48000 "$WAV" 2>/tmp/arecord_5min.err &
AREC=$!

# Trigger playback; record the response so a missing calib_tones.wav is visible.
PLAYRESP=$(curl -s -X POST "$MASTER/api/play" -H 'Content-Type: application/json' \
     -d '{"uri":"file:calib_tones.wav"}')
echo "$(date +%s.%N) PLAY resp=$PLAYRESP" >> "$LOG"

for i in $(seq 1 "$POLLS"); do
  echo "$(date +%s.%N) $(curl -s "$MASTER/api/playback/statuses")" >> "$LOG"
  sleep 1
done

wait "$AREC"
curl -s -X POST "$MASTER/api/stop" >/dev/null
echo "DONE wav=$WAV log=$LOG polls=$POLLS"

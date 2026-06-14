#!/bin/bash
# 10-minute continuous capture: play the 2h tone file (covers 10 min), record the
# mic, poll stats at 1 Hz. Single uninterrupted session. Feeds analyze_2h.py /
# offset_pass2.py / extras.py.
set -u
cd /home/rm/Git/gitlab.rand0m.me/share/ensemble
MASTER=http://192.168.71.63:8080
WAV=tools/calib/results/tones_10min.wav
LOG=tools/calib/results/tones_stats_10min.jsonl
DUR=615        # record ~10 min + margin
POLLS=610

: > "$LOG"
arecord -d "$DUR" -f S16_LE -c 2 -r 48000 "$WAV" 2>/tmp/arecord_10min.err &
AREC=$!
curl -s -X POST "$MASTER/api/play" -H 'Content-Type: application/json' \
     -d '{"uri":"file:calib_tones_2h.wav"}' >/dev/null
# Marker must be exactly "<epoch> PLAY": analyzers match the t0 anchor by exact string.
echo "$(date +%s.%N) PLAY" >> "$LOG"
for i in $(seq 1 "$POLLS"); do
  echo "$(date +%s.%N) $(curl -s "$MASTER/api/playback/statuses")" >> "$LOG"
  sleep 1
done
wait "$AREC"
curl -s -X POST "$MASTER/api/stop" >/dev/null
echo "DONE 10min wav=$WAV log=$LOG"

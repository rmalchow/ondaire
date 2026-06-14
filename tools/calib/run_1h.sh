#!/bin/bash
# 1-hour continuous capture: play the 2h tone file once (single uninterrupted
# session, so the servo runs without reset and we can see whether/when it
# converges), record the mic, poll stats at 1 Hz. Feeds analyze_2h.py.
set -u
cd /home/rm/Git/gitlab.rand0m.me/share/ensemble
MASTER=http://192.168.71.63:8080
WAV=tools/calib/results/tones_1h.wav
LOG=tools/calib/results/tones_stats_1h.jsonl
DUR=3615       # record ~60 min + margin
POLLS=3610

: > "$LOG"
arecord -d "$DUR" -f S16_LE -c 2 -r 48000 "$WAV" 2>/tmp/arecord_1h.err &
AREC=$!
curl -s -X POST "$MASTER/api/play" -H 'Content-Type: application/json' \
     -d '{"uri":"file:calib_tones_2h.wav"}' >/dev/null
# Marker must be exactly "<epoch> PLAY": analyzers match the t0 anchor by exact
# string equality, so no trailing resp= text here.
echo "$(date +%s.%N) PLAY" >> "$LOG"
for i in $(seq 1 "$POLLS"); do
  echo "$(date +%s.%N) $(curl -s "$MASTER/api/playback/statuses")" >> "$LOG"
  sleep 1
done
wait "$AREC"
curl -s -X POST "$MASTER/api/stop" >/dev/null
echo "DONE 1h wav=$WAV log=$LOG"

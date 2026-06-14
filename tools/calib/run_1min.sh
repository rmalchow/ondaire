#!/bin/bash
# Quick 1-minute dual-tone capture to validate the servo fix (fedPTS anchored to
# consumed-at-prime). Records mic + plays calib_tones.wav + polls statuses at 1 Hz.
set -u
cd /home/rm/Git/gitlab.rand0m.me/share/ensemble
MASTER=http://192.168.71.63:8080
WAV=tools/calib/results/tones_1min.wav
LOG=tools/calib/results/tones_stats_1min.jsonl
DUR=75
POLLS=70

: > "$LOG"
arecord -d "$DUR" -f S16_LE -c 2 -r 48000 "$WAV" 2>/tmp/arecord_1min.err &
AREC=$!
PLAYRESP=$(curl -s -X POST "$MASTER/api/play" -H 'Content-Type: application/json' \
     -d '{"uri":"file:calib_tones.wav"}')
echo "$(date +%s.%N) PLAY resp=$PLAYRESP" >> "$LOG"
for i in $(seq 1 "$POLLS"); do
  echo "$(date +%s.%N) $(curl -s "$MASTER/api/playback/statuses")" >> "$LOG"
  sleep 1
done
wait "$AREC"
curl -s -X POST "$MASTER/api/stop" >/dev/null
echo "DONE wav=$WAV log=$LOG"

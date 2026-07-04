#!/bin/bash
# 2-minute dual-tone capture. Records mic + plays calib_tones.wav + polls
# statuses at 1 Hz. Feeds analyze_servo.py / graph_servo_ppm.py.
set -u
cd /home/rm/Git/gitlab.rand0m.me/share/ondaire
MASTER=http://192.168.71.63:8080
WAV=tools/calib/results/tones_2min.wav
LOG=tools/calib/results/tones_stats_2min.jsonl
DUR=135
POLLS=130

: > "$LOG"
arecord -d "$DUR" -f S16_LE -c 2 -r 48000 "$WAV" 2>/tmp/arecord_2min.err &
AREC=$!
curl -s -X POST "$MASTER/api/play" -H 'Content-Type: application/json' \
     -d '{"uri":"file:calib_tones.wav"}' >/dev/null
# Marker must be exactly "<epoch> PLAY": the analyzers match the t0 anchor by
# exact string equality (rest == "PLAY"), so no trailing resp= text here.
echo "$(date +%s.%N) PLAY" >> "$LOG"
for i in $(seq 1 "$POLLS"); do
  echo "$(date +%s.%N) $(curl -s "$MASTER/api/playback/statuses")" >> "$LOG"
  sleep 1
done
wait "$AREC"
curl -s -X POST "$MASTER/api/stop" >/dev/null
echo "DONE wav=$WAV log=$LOG"

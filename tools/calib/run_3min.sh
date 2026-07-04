#!/bin/bash
set -u
cd /home/rm/Git/gitlab.rand0m.me/share/ondaire
M=http://192.168.71.63:8080
WAV=tools/calib/results/tones_v0150.wav
LOG=tools/calib/results/tones_stats_v0150.jsonl
: > "$LOG"
arecord -d 190 -f S16_LE -c 2 -r 48000 "$WAV" 2>/tmp/arec_v0150.err &
A=$!
curl -s -X POST $M/api/play -H 'Content-Type: application/json' -d '{"uri":"file:calib_tones.wav"}' >/dev/null
echo "$(date +%s.%N) PLAY" >> "$LOG"
for i in $(seq 1 180); do echo "$(date +%s.%N) $(curl -s $M/api/playback/statuses)" >> "$LOG"; sleep 1; done
wait $A
curl -s -X POST $M/api/stop >/dev/null
echo DONE

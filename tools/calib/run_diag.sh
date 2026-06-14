#!/bin/bash
set -u
cd /home/rm/Git/gitlab.rand0m.me/share/ensemble
M=http://192.168.71.63:8080
LOG=tools/calib/results/diag.jsonl
: > "$LOG"
PLAYRESP=$(curl -s -X POST $M/api/play -H 'Content-Type: application/json' -d '{"uri":"file:calib_tones.wav"}')
echo "$(date +%s.%N) PLAY resp=$PLAYRESP" >> "$LOG"
for i in $(seq 1 35); do echo "$(date +%s.%N) $(curl -s $M/api/playback/statuses)" >> "$LOG"; sleep 1; done
curl -s -X POST $M/api/stop >/dev/null
echo DONE

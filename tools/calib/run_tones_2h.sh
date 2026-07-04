#!/bin/bash
# 2-hour continuous capture: play the 2h tone file once (single session, so the
# servo runs uninterrupted and we see whether/when it converges), record the mic,
# poll stats at 1 Hz. Unattended baseline for the P-only servo convergence.
set -u
cd /home/rm/Git/gitlab.rand0m.me/share/ondaire
MASTER=http://192.168.71.63:8080
WAV=tools/calib/results/tones_2h.wav
LOG=tools/calib/results/tones_stats_2h.jsonl
DUR=7260       # record ~121 min (file is 120 min + margin)
POLLS=7230

: > "$LOG"
arecord -d "$DUR" -f S16_LE -c 2 -r 48000 "$WAV" 2>/tmp/arecord_2h.err &
AREC=$!
curl -s -X POST "$MASTER/api/play" -H 'Content-Type: application/json' \
     -d '{"uri":"file:calib_tones_2h.wav"}' >/dev/null
echo "$(date +%s.%N) PLAY" >> "$LOG"
for i in $(seq 1 "$POLLS"); do
  echo "$(date +%s.%N) $(curl -s "$MASTER/api/playback/statuses")" >> "$LOG"
  sleep 1
done
wait "$AREC"
curl -s -X POST "$MASTER/api/stop" >/dev/null
echo "DONE 2h wav=$WAV log=$LOG"

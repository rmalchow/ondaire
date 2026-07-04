#!/bin/bash
# Two consecutive 3-min captures with a full session restart (stop → 1 s → play)
# between them. Tests whether the offset/ppm ramp is thermal (continues / run2 >
# run1) or a per-session servo-convergence transient (run2 repeats run1). Each run
# records the mic + a 1 Hz stats poll with a PLAY marker, to separate files.
set -u
cd /home/rm/Git/gitlab.rand0m.me/share/ondaire
MASTER=http://192.168.71.63:8080
DUR=185        # ~3 min record
POLLS=180

do_run() {
  local tag=$1
  local WAV=tools/calib/results/tones_${tag}.wav
  local LOG=tools/calib/results/tones_stats_${tag}.jsonl
  : > "$LOG"
  arecord -d "$DUR" -f S16_LE -c 2 -r 48000 "$WAV" 2>/dev/null &
  local AREC=$!
  curl -s -X POST "$MASTER/api/play" -H 'Content-Type: application/json' \
       -d '{"uri":"file:calib_tones.wav"}' >/dev/null
  echo "$(date +%s.%N) PLAY" >> "$LOG"
  for i in $(seq 1 "$POLLS"); do
    echo "$(date +%s.%N) $(curl -s "$MASTER/api/playback/statuses")" >> "$LOG"
    sleep 1
  done
  wait "$AREC"
  curl -s -X POST "$MASTER/api/stop" >/dev/null
}

do_run split1
sleep 1   # full restart gap (session is torn down: servo + resampler + jb reset)
do_run split2
echo "DONE split1+split2"

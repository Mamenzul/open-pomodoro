#!/usr/bin/env bash
notify-send -u normal \
  "🍅 Pomodoro started" \
  "Cycle ${POMODORO_CYCLE} — focus for $(( POMODORO_DURATION / 60 )) min"

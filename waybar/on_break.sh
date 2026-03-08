#!/usr/bin/env bash
notify-send -u critical \
  "🍅 Pomodoro finished" \
  "Break time! (cycle ${POMODORO_CYCLE})"

pw-play /usr/share/sounds/freedesktop/stereo/complete.oga

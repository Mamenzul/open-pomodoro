#!/usr/bin/env bash
notify-send -u critical \
  "🎉 Daily goal reached!" \
  "${POMODORO_DAILY} pomodoros completed today"

pw-play /usr/share/sounds/freedesktop/stereo/complete.oga

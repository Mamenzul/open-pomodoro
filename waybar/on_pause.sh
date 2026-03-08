#!/usr/bin/env bash
notify-send -u low \
  "⏸ Pomodoro paused" \
  "$(( POMODORO_DURATION / 60 )) min remaining"

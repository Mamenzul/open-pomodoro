#!/usr/bin/env bash
notify-send -u low \
  "▶ Pomodoro resumed" \
  "$(( POMODORO_DURATION / 60 )) min remaining"

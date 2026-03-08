#!/usr/bin/env bash
# ~/.config/open-pomodoro/waybar.sh
#
# Waybar custom/script module for open-pomodoro.
#
# Waybar config:
#
#   "custom/pomodoro": {
#       "exec": "~/.config/open-pomodoro/hooks/waybar.sh",
#       "interval": 5,
#       "return-type": "json",
#       "on-click": "pomodoro start",
#       "on-click-middle": "pomodoro pause || pomodoro resume",
#       "on-click-right": "pomodoro break"
#   }
#
# CSS classes: "work", "break", "pause", "idle"
#
#   #custom-pomodoro.work  { color: #f38ba8; }
#   #custom-pomodoro.break { color: #a6e3a1; }
#   #custom-pomodoro.pause { color: #f9e2af; }
#   #custom-pomodoro.idle  { color: #6c7086; }

STATE_FILE="${XDG_CONFIG_HOME:-$HOME/.config}/open-pomodoro/state.json"

emit() {
  local text="$1" tooltip="$2" class="$3" pct="${4:-0}"
  printf '{"text":"%s","tooltip":"%s","class":"%s","percentage":%d}\n' \
    "$text" "$tooltip" "$class" "$pct"
}

if [[ ! -f "$STATE_FILE" ]]; then
  emit "⚪ Idle" "No session" "idle" 0
  exit 0
fi

# ── Parse JSON without jq ─────────────────────────────────────────────────────
# Each extractor targets a specific key to avoid greedy-match collisions.

get_str() {
  # get_str KEY — extracts "key": "value"
  grep -oP "\"$1\"\s*:\s*\"\\K[^\"]*" "$STATE_FILE" | head -1
}

get_num() {
  # get_num KEY — extracts "key": 12345
  grep -oP "\"$1\"\s*:\s*\K-?[0-9]+" "$STATE_FILE" | head -1
}

STATE=$(get_str state)
CYCLE=$(get_num cycle)
DAILY=$(get_num daily_count)

# time.Duration is serialised as int64 nanoseconds
DURATION_NS=$(get_num duration)
ELAPSED_NS=$(get_num elapsed)

# time.Time is serialised as RFC3339Nano: 2006-01-02T15:04:05.999999999Z07:00
START_RAW=$(get_str start_time)

# ── Convert nanoseconds → seconds ─────────────────────────────────────────────
DURATION_SEC=$(( ${DURATION_NS:-0} / 1000000000 ))
ELAPSED_SEC=$(( ${ELAPSED_NS:-0} / 1000000000 ))

# ── Parse RFC3339Nano start_time → unix epoch ─────────────────────────────────
# Strip sub-second part so `date` doesn't choke: 2006-01-02T15:04:05.999+02:00
#                                              → 2006-01-02T15:04:05+02:00
parse_epoch() {
  local raw="$1"
  [[ -z "$raw" ]] && echo 0 && return
  # Remove nanoseconds/microseconds between the seconds and the tz offset/Z
  local clean
  clean=$(echo "$raw" | sed 's/\.[0-9]*\(Z\|[+-]\)/\1/')
  date -d "$clean" +%s 2>/dev/null || echo 0
}

START_EPOCH=$(parse_epoch "$START_RAW")

# ── Compute remaining seconds ─────────────────────────────────────────────────
now=$(date +%s)

case "$STATE" in
  PAUSE)
    REM=$(( DURATION_SEC - ELAPSED_SEC ))
    ;;
  WORK|BREAK)
    END=$(( START_EPOCH + DURATION_SEC ))
    REM=$(( END - now ))
    ;;
  *)
    REM=0
    ;;
esac

(( REM < 0 )) && REM=0

# ── Percentage elapsed (for progress arc themes) ──────────────────────────────
if (( DURATION_SEC > 0 )); then
  PCT=$(( 100 - ( REM * 100 / DURATION_SEC ) ))
  (( PCT < 0   )) && PCT=0
  (( PCT > 100 )) && PCT=100
else
  PCT=0
fi

# ── Format mm:ss ──────────────────────────────────────────────────────────────
fmt() { printf "%02d:%02d" $(( $1 / 60 )) $(( $1 % 60 )); }
REMAINING=$(fmt "$REM")

# ── Emit JSON per state ───────────────────────────────────────────────────────
case "$STATE" in
  WORK)
    emit "🍅 ${REMAINING}" \
         "Working — cycle ${CYCLE}/4\\nDone today: ${DAILY}" \
         "work" "$PCT"
    ;;
  BREAK)
    emit "☕ ${REMAINING}" \
         "Break — cycle ${CYCLE}/4\\n${REM}s remaining" \
         "break" "$PCT"
    ;;
  PAUSE)
    emit "⏸ ${REMAINING}" \
         "Paused — ${REM}s remaining when resumed" \
         "pause" "$PCT"
    ;;
  *)
    emit "" \
         "No active session\\nDone today: ${DAILY}\\nClick to start" \
         "idle" 0
    ;;
esac

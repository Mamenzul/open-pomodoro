# open-pomodoro

A minimal, file-backed Pomodoro timer for the command line.

## Install

```bash
go install github.com/Mamenzul/open-pomodoro/cmd/pomodoro@latest
```

Make sure `$(go env GOPATH)/bin` is in your `$PATH`:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.bashrc
source ~/.bashrc
```

Then create your config file:

```bash
pomodoro init
```

## Shell Completion

**Bash**
```bash
echo 'eval "$(pomodoro completion bash)"' >> ~/.bashrc
source ~/.bashrc
```

**Zsh**
```bash
echo 'eval "$(pomodoro completion zsh)"' >> ~/.zshrc
source ~/.zshrc
```

**Fish**
```bash
pomodoro completion fish > ~/.config/fish/completions/pomodoro.fish
```

## Usage

```bash
pomodoro start             # begin a work session
pomodoro status            # check time remaining
pomodoro pause             # freeze the session
pomodoro resume            # unfreeze
pomodoro break             # end work, start a break (short or long)
pomodoro continue          # toggle: start break after work, or start work after break
pomodoro start             # cycle advances here, after the break
pomodoro stop              # abandon session, return to Idle
pomodoro clean             # reset state; --history also wipes history.jsonl
```

## Commands

| Command | Description |
|---------|-------------|
| `pomodoro init` | Write default `~/.config/open-pomodoro/settings` |
| `pomodoro start` | Begin a work session |
| `pomodoro break` | Begin a short or long break (auto-selected by cycle) |
| `pomodoro continue` | Continue: start work after a break, or start break after work |
| `pomodoro pause` | Freeze the current session |
| `pomodoro resume` | Unfreeze and continue |
| `pomodoro stop` | Abandon the current session, return to Idle |
| `pomodoro status` | Print timer status |
| `pomodoro clean` | Reset state to Idle; `--history` also wipes history.jsonl |

### Flags

**`start`** — `--duration 30m` · `--ago 5m` · `--wait 25m` (blocks with live countdown)

**`break`** — `--duration 10m` · `--ago 2m`

**`clean`** — `--history` (also deletes history.jsonl)

**`status`** — `-f / --format` — custom format string

### Status Format Tags

| Tag | Meaning | Example |
|-----|---------|---------|
| `%s` | Mode emoji + label | `🍅 Work` |
| `%r` | Time remaining (mm:ss) | `22:10` |
| `%c` | Current cycle number | `2` |
| `%C` | Work sessions completed today | `5` |
| `%I` | Long-break interval from config | `4` |

```
$ pomodoro status -f "%s [%r] Cycle: %c/%I — Done today: %C"
🍅 Work [22:10] Cycle: 2/4 — Done today: 3
```

## Cycle Model

**One cycle = one work session + one break.**

```
start → work → break → start → work → break → ...
  cycle 1            ↑ cycle 2 begins here
```

The cycle counter increments on `start`, but **only** when the previous
break was completed (`break` was called after the last work session).
Calling `start` twice in a row, or stopping mid-session, does not advance
the cycle. After `long_break_interval` cycles the counter resets to 1.

## Hooks

Set any hook key in `~/.config/open-pomodoro/settings` to a shell command
or script path. The hook runs via `$SHELL -c <value>` after each state
transition. Hook failures are printed to stderr but never abort the command.

```logfmt
on_start=notify-send "🍅 Pomodoro" "Cycle $POMODORO_CYCLE started"
on_break=~/.config/open-pomodoro/hooks/on_break.sh
on_pause=
on_resume=
on_stop=
on_complete=~/.config/open-pomodoro/hooks/on_complete.sh
```

### Hook Environment Variables

| Variable | Description |
|----------|-------------|
| `POMODORO_EVENT` | Event name (`on_start`, `on_break`, `on_pause`, `on_resume`, `on_stop`, `on_complete`) |
| `POMODORO_STATE` | New state after the transition (`WORK`, `BREAK`, `PAUSE`, `IDLE`) |
| `POMODORO_CYCLE` | Current cycle number |
| `POMODORO_DURATION` | Session duration in seconds |
| `POMODORO_DAILY` | Work sessions completed today |

`on_complete` fires when `DailyCount` reaches `daily_goal`.

### Example hook scripts

```bash
# on_break.sh
#!/usr/bin/env bash
notify-send -u critical \
  "🍅 Pomodoro finished" \
  "Break time! (cycle ${POMODORO_CYCLE})"
pw-play /usr/share/sounds/freedesktop/stereo/complete.oga
```

```bash
# on_complete.sh
#!/usr/bin/env bash
notify-send -u critical \
  "🎉 Daily goal reached!" \
  "${POMODORO_DAILY} pomodoros completed today"
pw-play /usr/share/sounds/freedesktop/stereo/complete.oga
```

## Waybar Integration

Use the bundled `waybar.sh` script as a `custom/script` module:

```json
"custom/pomodoro": {
    "exec": "~/.config/open-pomodoro/hooks/waybar.sh",
    "interval": 1,
    "return-type": "json",
    "on-click": "pomodoro start",
    "on-click-middle": "pomodoro pause || pomodoro resume || pomodoro continue",
    "on-click-right": "pomodoro break"
}
```

```css
#custom-pomodoro        { padding: 0 8px; }
#custom-pomodoro.work   { color: #f38ba8; }
#custom-pomodoro.break  { color: #a6e3a1; }
#custom-pomodoro.pause  { color: #f9e2af; }
#custom-pomodoro.idle   { color: #6c7086; }
```

## Configuration

`~/.config/open-pomodoro/settings` (logfmt, `key=value`, `#` comments):

```logfmt
work_duration=25m
short_break_duration=5m
long_break_duration=20m
long_break_interval=4
daily_goal=8

# hooks
on_start=notify-send "Pomodoro started" "Cycle $POMODORO_CYCLE"
on_break=notify-send "Break time"
on_pause=
on_resume=
on_stop=
on_complete=
```

All keys can also be overridden via environment variables with the `POMODORO_`
prefix (e.g. `POMODORO_WORK_DURATION=30m`).

## State & History

| File | Purpose |
|------|---------|
| `~/.config/open-pomodoro/state.json` | Active session (rewritten each command) |
| `~/.config/open-pomodoro/history.jsonl` | Append-only log of completed sessions |

## Project Structure

```
.
├── cmd/pomodoro/main.go         # Cobra root + all subcommands
├── internal/
│   ├── clock/clock.go           # Duration parsing, countdown, format helpers
│   ├── config/config.go         # Logfmt + Viper loading, RunHook()
│   └── state/state.go           # JSON persistence, pause/resume, history
├── go.mod
└── go.sum
```

## License

MIT

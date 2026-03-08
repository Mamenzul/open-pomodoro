package config

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all user-configurable settings.
type Config struct {
	WorkDuration       time.Duration
	ShortBreakDuration time.Duration
	LongBreakDuration  time.Duration
	LongBreakInterval  int
	DailyGoal          int
	// Hooks maps event names to shell script paths.
	// Supported events: on_start, on_break, on_pause, on_resume, on_stop, on_complete
	Hooks map[string]string
}

// defaults applied before reading the settings file.
var defaults = map[string]string{
	"work_duration":        "25m",
	"short_break_duration": "5m",
	"long_break_duration":  "20m",
	"long_break_interval":  "4",
	"daily_goal":           "8",
	// Hook keys — empty by default (no-op).
	"on_start":    "",
	"on_break":    "",
	"on_pause":    "",
	"on_resume":   "",
	"on_stop":     "",
	"on_complete": "",
}

// hookKeys is the canonical list of recognised hook event names.
var hookKeys = []string{"on_start", "on_break", "on_pause", "on_resume", "on_stop", "on_complete"}

// settingsPath returns the path to the logfmt settings file.
func settingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "open-pomodoro", "settings"), nil
}

// Load reads the logfmt settings file and returns a populated Config.
// Missing keys fall back to the compiled-in defaults.
func Load() (*Config, error) {
	raw := make(map[string]string)

	// Seed with defaults.
	for k, v := range defaults {
		raw[k] = v
	}

	path, err := settingsPath()
	if err != nil {
		return nil, err
	}

	// Parse the logfmt file; ignore "not found" — defaults suffice.
	if err := parseLogfmt(path, raw); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("loading settings: %w", err)
	}

	// Also let Viper pick up env-var overrides (POMODORO_WORK_DURATION, etc.)
	v := viper.New()
	v.SetEnvPrefix("POMODORO")
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	for k, val := range raw {
		v.SetDefault(k, val)
	}

	cfg := &Config{}

	cfg.WorkDuration, err = parseDuration(v.GetString("work_duration"))
	if err != nil {
		return nil, fmt.Errorf("work_duration: %w", err)
	}

	cfg.ShortBreakDuration, err = parseDuration(v.GetString("short_break_duration"))
	if err != nil {
		return nil, fmt.Errorf("short_break_duration: %w", err)
	}

	cfg.LongBreakDuration, err = parseDuration(v.GetString("long_break_duration"))
	if err != nil {
		return nil, fmt.Errorf("long_break_duration: %w", err)
	}

	cfg.LongBreakInterval = v.GetInt("long_break_interval")
	cfg.DailyGoal = v.GetInt("daily_goal")

	cfg.Hooks = make(map[string]string, len(hookKeys))
	for _, key := range hookKeys {
		cfg.Hooks[key] = v.GetString(key)
	}

	return cfg, nil
}

// parseLogfmt reads a key=value logfmt file (one pair per line, # comments).
func parseLogfmt(path string, out map[string]string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("line %d: invalid logfmt entry %q", lineNo, line)
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		out[key] = val
	}
	return scanner.Err()
}

// parseDuration wraps time.ParseDuration with a friendlier error.
func parseDuration(s string) (time.Duration, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: %w", s, err)
	}
	return d, nil
}

// RunHook fires the script configured for the given event, if any.
// The script is executed via $SHELL (falling back to /bin/sh) so it can be
// any executable or shell one-liner.
//
// The following environment variables are injected for the script:
//
//	POMODORO_EVENT      — event name (e.g. "on_start")
//	POMODORO_STATE      — new state  (e.g. "WORK")
//	POMODORO_CYCLE      — current cycle number
//	POMODORO_DURATION   — session duration in seconds
//	POMODORO_DAILY      — work sessions completed today
//
// Hook failures are printed to stderr but never abort the main command.
func RunHook(cfg *Config, event, newState string, cycle int, duration, daily string) {
	script, ok := cfg.Hooks[event]
	if !ok || script == "" {
		return
	}

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell, "-c", script)
	cmd.Env = append(os.Environ(),
		"POMODORO_EVENT="+event,
		"POMODORO_STATE="+newState,
		fmt.Sprintf("POMODORO_CYCLE=%d", cycle),
		"POMODORO_DURATION="+duration,
		"POMODORO_DAILY="+daily,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  hook %q exited with error: %v\n", event, err)
	}
}

// WriteDefaults creates a settings file with default values if one does not
// already exist. Useful for first-run initialisation.
func WriteDefaults() error {
	path, err := settingsPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return nil // already exists
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	content := `# open-pomodoro settings (logfmt)
work_duration=25m
short_break_duration=5m
long_break_duration=20m
long_break_interval=4
daily_goal=8

# Hooks — set to a shell command or script path to run on each event.
# Available env vars: POMODORO_EVENT, POMODORO_STATE, POMODORO_CYCLE,
#                     POMODORO_DURATION, POMODORO_DAILY
#
# on_start=notify-send "Pomodoro started" "Cycle $POMODORO_CYCLE"
# on_break=notify-send "Break time" "$POMODORO_EVENT"
# on_pause=
# on_resume=
# on_stop=
# on_complete=~/.config/open-pomodoro/hooks/on_complete.sh

# For convenience I use bash scripts for waybar setup : 
# on_start=notify-send "Pomodoro started" "Cycle $POMODORO_CYCLE"
# on_break=notify-send "Break time" "$POMODORO_EVENT"
# on_pause=
# on_resume=
# on_stop=
# on_complete=~/.config/open-pomodoro/hooks/on_complete.sh

`
	return os.WriteFile(path, []byte(content), 0o644)
}

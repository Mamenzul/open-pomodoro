package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SessionState represents the current mode of the pomodoro timer.
type SessionState string

const (
	StateIdle  SessionState = "IDLE"
	StateWork  SessionState = "WORK"
	StateBreak SessionState = "BREAK"
	StatePause SessionState = "PAUSE"
)

// CurrentStatus holds all mutable runtime state persisted to state.json.
type CurrentStatus struct {
	State        SessionState  `json:"state"`
	StartTime    time.Time     `json:"start_time"`
	Duration     time.Duration `json:"duration"`
	Cycle        int           `json:"cycle"`          // 1–4, resets after long break
	DailyCount   int           `json:"daily_count"`    // total completed work sessions today
	LastDate     string        `json:"last_date"`      // YYYY-MM-DD, used to detect day rollover
	PauseTime    time.Time     `json:"pause_time"`     // when the session was paused
	Elapsed      time.Duration `json:"elapsed"`        // time already worked before pause
	PrePauseState SessionState `json:"pre_pause_state"` // WORK or BREAK, restored on resume
	// BreakDone tracks whether the most-recently completed cycle included a break.
	// A full cycle = work session + break. Cycle number only advances on `start`
	// when the previous break has been completed (BreakDone == true).
	BreakDone bool `json:"break_done"`
}

// HistoryEvent is a single line in history.jsonl.
type HistoryEvent struct {
	Type      string        `json:"type"`       // "work" | "break" | "long_break"
	StartTime time.Time     `json:"start_time"`
	Duration  time.Duration `json:"duration"`
	Cycle     int           `json:"cycle"`
}

// configDir returns the base config directory, creating it if needed.
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "open-pomodoro")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("cannot create config dir: %w", err)
	}
	return dir, nil
}

func statePath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

func historyPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.jsonl"), nil
}

// Load reads the persisted state. Returns a default IDLE state if the file
// does not yet exist.
func Load() (*CurrentStatus, error) {
	path, err := statePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return defaultStatus(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var s CurrentStatus
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}

	// Day rollover — reset daily count if the date has changed.
	today := time.Now().Format("2006-01-02")
	if s.LastDate != today {
		s.DailyCount = 0
		s.LastDate = today
	}

	return &s, nil
}

// Save writes the current status atomically to state.json.
func Save(s *CurrentStatus) error {
	path, err := statePath()
	if err != nil {
		return err
	}

	s.LastDate = time.Now().Format("2006-01-02")

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling state: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing state temp file: %w", err)
	}
	return os.Rename(tmp, path)
}

// AppendHistory appends a completed session event to history.jsonl.
func AppendHistory(event HistoryEvent) error {
	path, err := historyPath()
	if err != nil {
		return err
	}

	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshalling history event: %w", err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening history file: %w", err)
	}
	defer f.Close()

	_, err = fmt.Fprintf(f, "%s\n", line)
	return err
}

// IsExpired reports whether the active session has passed its scheduled end.
func (s *CurrentStatus) IsExpired() bool {
	if s.State == StateIdle || s.State == StatePause {
		return false
	}
	return time.Now().After(s.StartTime.Add(s.Duration))
}

// Remaining returns how much time is left in the current session.
// When paused, returns the frozen remaining duration.
// Returns 0 if the session has already expired.
func (s *CurrentStatus) Remaining() time.Duration {
	switch s.State {
	case StateIdle:
		return 0
	case StatePause:
		// Remaining = total duration minus what was already elapsed before pause.
		r := s.Duration - s.Elapsed
		if r < 0 {
			return 0
		}
		return r
	default:
		remaining := time.Until(s.StartTime.Add(s.Duration))
		if remaining < 0 {
			return 0
		}
		return remaining
	}
}

func defaultStatus() *CurrentStatus {
	return &CurrentStatus{
		State:    StateIdle,
		Cycle:    0, // incremented to 1 on first `start`
		LastDate: time.Now().Format("2006-01-02"),
	}
}

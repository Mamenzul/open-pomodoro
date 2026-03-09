package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Mamenzul/open-pomodoro/internal/clock"
	"github.com/Mamenzul/open-pomodoro/internal/config"
	"github.com/Mamenzul/open-pomodoro/internal/state"
	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ── Root ─────────────────────────────────────────────────────────────────────

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "pomodoro",
		Short: "Open-Pomodoro — a minimal, file-backed Pomodoro timer",
		Long: `Open-Pomodoro manages work/break cycles via simple CLI commands.
State is stored in ~/.config/open-pomodoro/state.json.
History is appended to ~/.config/open-pomodoro/history.jsonl.

Cycle model: one full cycle = one work session + one break.
The cycle counter advances when you run 'start' after completing a break.`,
		SilenceUsage: true,
	}

	root.AddCommand(
		startCmd(),
		breakCmd(),
		continueCmd(),
		pauseCmd(),
		resumeCmd(),
		stopCmd(),
		statusCmd(),
		cleanCmd(),
		initCmd(),
	)
	return root
}

// ── Continue command ──────────────────────────────────────────────────────────
//
// `continue` toggles between work and break:
//
func continueCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "continue",
		Short: "Toggle: start break after work, or start work after break",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load current state/config.
			s, _, err := loadAll()
			if err != nil {
				return err
			}

			// Determine effective session type (account for Pause).
			effective := s.State
			if s.State == state.StatePause && s.PrePauseState != "" {
				effective = s.PrePauseState
			}

			// If we were working, invoke the existing `break` command.
			if effective == state.StateWork {
				bc := breakCmd()
				// Call the command's RunE directly to reuse its logic.
				if bc.RunE != nil {
					return bc.RunE(bc, []string{})
				}
				return fmt.Errorf("break command not available")
			}

			// Otherwise invoke the existing `start` command (works for BREAK, IDLE, etc).
			sc := startCmd()
			if sc.RunE != nil {
				return sc.RunE(sc, []string{})
			}
			return fmt.Errorf("start command not available")
		},
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func loadAll() (*state.CurrentStatus, *config.Config, error) {
	s, err := state.Load()
	if err != nil {
		return nil, nil, err
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	return s, cfg, nil
}

func hookDuration(d time.Duration) string {
	return fmt.Sprintf("%.0f", d.Seconds())
}

// ── `pomodoro start` ──────────────────────────────────────────────────────────

func startCmd() *cobra.Command {
	var (
		flagDuration string
		flagAgo      string
		flagWait     string
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Begin a work session",
		Long: `Begin a work session.

Cycle logic: a cycle = work + break. The cycle number increments here
only when the previous break was completed (BreakDone=true). Running
'start' twice in a row does NOT advance the cycle.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, cfg, err := loadAll()
			if err != nil {
				return err
			}

			dur := cfg.WorkDuration
			if flagDuration != "" {
				d, err := time.ParseDuration(flagDuration)
				if err != nil {
					return fmt.Errorf("invalid --duration: %w", err)
				}
				dur = d
			}

			startAt := time.Now()
			if flagAgo != "" {
				startAt, err = clock.ParseAgo(flagAgo)
				if err != nil {
					return err
				}
			}

			// Advance cycle only after a completed break.
			if s.BreakDone {
				s.Cycle++
				if s.Cycle > cfg.LongBreakInterval {
					s.Cycle = 1
				}
				s.BreakDone = false
			}
			if s.Cycle == 0 {
				s.Cycle = 1
			}

			s.State = state.StateWork
			s.StartTime = startAt
			s.Duration = dur
			s.Elapsed = 0
			s.PauseTime = time.Time{}
			s.PrePauseState = ""

			if err := state.Save(s); err != nil {
				return err
			}

			fmt.Printf("🍅 Work session started — %s (cycle %d/%d)\n",
				dur, s.Cycle, cfg.LongBreakInterval)

			config.RunHook(cfg, "on_start", string(s.State), s.Cycle,
				hookDuration(dur), fmt.Sprintf("%d", s.DailyCount))

			if flagWait != "" {
				fmt.Println("Waiting until session ends…")
				return clock.ParseWait(flagWait, func(remaining time.Duration) {
					fmt.Printf("\r  ⏳ %s remaining  ", clock.FormatRemaining(remaining))
				})
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&flagDuration, "duration", "", "Override work duration (e.g. 30m)")
	cmd.Flags().StringVar(&flagAgo, "ago", "", "Session started this long ago (e.g. 5m)")
	cmd.Flags().StringVar(&flagWait, "wait", "", "Block and display a countdown for this duration")
	return cmd
}

// ── `pomodoro break` ──────────────────────────────────────────────────────────

func breakCmd() *cobra.Command {
	var (
		flagDuration string
		flagAgo      string
	)

	cmd := &cobra.Command{
		Use:   "break",
		Short: "Begin a break (short or long depending on cycle position)",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, cfg, err := loadAll()
			if err != nil {
				return err
			}

			// Record completed work session.
			prevState := s.State
			if prevState == state.StateWork ||
				(prevState == state.StatePause && s.PrePauseState == state.StateWork) {
				s.DailyCount++
				_ = state.AppendHistory(state.HistoryEvent{
					Type:      "work",
					StartTime: s.StartTime,
					Duration:  s.Duration,
					Cycle:     s.Cycle,
				})
				if cfg.DailyGoal > 0 && s.DailyCount == cfg.DailyGoal {
					config.RunHook(cfg, "on_complete", "WORK", s.Cycle,
						hookDuration(s.Duration), fmt.Sprintf("%d", s.DailyCount))
				}
			}

			dur := cfg.ShortBreakDuration
			breakType := "short_break"
			label := "☕  Short break"
			if s.Cycle >= cfg.LongBreakInterval {
				dur = cfg.LongBreakDuration
				breakType = "long_break"
				label = "🛋️  Long break"
			}

			if flagDuration != "" {
				d, err := time.ParseDuration(flagDuration)
				if err != nil {
					return fmt.Errorf("invalid --duration: %w", err)
				}
				dur = d
			}

			startAt := time.Now()
			if flagAgo != "" {
				startAt, err = clock.ParseAgo(flagAgo)
				if err != nil {
					return err
				}
			}

			s.State = state.StateBreak
			s.StartTime = startAt
			s.Duration = dur
			s.Elapsed = 0
			s.PauseTime = time.Time{}
			s.PrePauseState = ""
			s.BreakDone = true // next `start` will advance the cycle

			if err := state.Save(s); err != nil {
				return err
			}

			_ = state.AppendHistory(state.HistoryEvent{
				Type:      breakType,
				StartTime: startAt,
				Duration:  dur,
				Cycle:     s.Cycle,
			})

			fmt.Printf("%s started — %s\n", label, dur)

			config.RunHook(cfg, "on_break", string(s.State), s.Cycle,
				hookDuration(dur), fmt.Sprintf("%d", s.DailyCount))

			return nil
		},
	}

	cmd.Flags().StringVar(&flagDuration, "duration", "", "Override break duration")
	cmd.Flags().StringVar(&flagAgo, "ago", "", "Break started this long ago (e.g. 2m)")
	return cmd
}

// ── `pomodoro pause` ─────────────────────────────────────────────────────────

func pauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause",
		Short: "Pause the current work or break session",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, cfg, err := loadAll()
			if err != nil {
				return err
			}

			if s.State != state.StateWork && s.State != state.StateBreak {
				return fmt.Errorf("nothing to pause (state is %s)", s.State)
			}

			s.Elapsed = time.Since(s.StartTime)
			if s.Elapsed > s.Duration {
				s.Elapsed = s.Duration
			}
			s.PauseTime = time.Now()
			s.PrePauseState = s.State
			s.State = state.StatePause

			if err := state.Save(s); err != nil {
				return err
			}

			remaining := s.Duration - s.Elapsed
			fmt.Printf("⏸  Paused (%s remaining)\n", clock.FormatRemaining(remaining))

			config.RunHook(cfg, "on_pause", string(s.State), s.Cycle,
				hookDuration(remaining), fmt.Sprintf("%d", s.DailyCount))

			return nil
		},
	}
}

// ── `pomodoro resume` ────────────────────────────────────────────────────────

func resumeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resume",
		Short: "Resume a paused session",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, cfg, err := loadAll()
			if err != nil {
				return err
			}

			if s.State != state.StatePause {
				return fmt.Errorf("nothing to resume (state is %s)", s.State)
			}

			remaining := s.Duration - s.Elapsed
			// Set a new start time so StartTime + Duration = now + remaining.
			s.StartTime = time.Now().Add(-s.Elapsed)
			s.State = s.PrePauseState
			s.PauseTime = time.Time{}
			s.PrePauseState = ""

			if err := state.Save(s); err != nil {
				return err
			}

			label := modeEmoji(s.State)
			fmt.Printf("▶️  Resumed %s (%s remaining)\n", label, clock.FormatRemaining(remaining))

			config.RunHook(cfg, "on_resume", string(s.State), s.Cycle,
				hookDuration(remaining), fmt.Sprintf("%d", s.DailyCount))

			return nil
		},
	}
}

// ── `pomodoro stop` ───────────────────────────────────────────────────────────

func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Abandon the current session and return to Idle",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, cfg, err := loadAll()
			if err != nil {
				return err
			}

			prev := s.State
			s.State = state.StateIdle
			s.StartTime = time.Time{}
			s.Duration = 0
			s.Elapsed = 0
			s.PauseTime = time.Time{}
			s.PrePauseState = ""

			if err := state.Save(s); err != nil {
				return err
			}

			fmt.Printf("⏹  Stopped (was %s). Now Idle.\n", prev)

			config.RunHook(cfg, "on_stop", string(s.State), s.Cycle,
				"0", fmt.Sprintf("%d", s.DailyCount))

			return nil
		},
	}
}

// ── `pomodoro status` ─────────────────────────────────────────────────────────

func statusCmd() *cobra.Command {
	const defaultFmt = "%s [%r] Cycle: %c/%I — Done today: %C"

	var flagFormat string

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print the current timer status",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, cfg, err := loadAll()
			if err != nil {
				return err
			}

			modeLabel := modeEmoji(s.State)
			remaining := clock.FormatRemaining(s.Remaining())

			format := flagFormat
			if format == "" {
				format = defaultFmt
			}

			intervalStr := fmt.Sprintf("%d", cfg.LongBreakInterval)
			output := replaceAll(format, map[string]string{"%I": intervalStr})
			output = clock.FormatStatus(output, modeLabel, remaining, s.Cycle, s.DailyCount)

			fmt.Println(output)
			return nil
		},
	}

	cmd.Flags().StringVarP(&flagFormat, "format", "f", "",
		`Output format. Tags: %s mode, %r remaining, %c cycle, %C daily count, %I interval`)
	return cmd
}

// ── `pomodoro clean` ─────────────────────────────────────────────────────────

func cleanCmd() *cobra.Command {
	var flagHistory bool

	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Reset timer state to Idle (wipe state.json)",
		Long: `Reset the timer to a clean Idle state.

By default only state.json is cleared (cycle and daily counts reset to 0).
Pass --history to also delete history.jsonl.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			dir := home + "/.config/open-pomodoro"

			blank := &state.CurrentStatus{
				State:    state.StateIdle,
				Cycle:    0,
				LastDate: time.Now().Format("2006-01-02"),
			}
			if err := state.Save(blank); err != nil {
				return fmt.Errorf("resetting state: %w", err)
			}
			fmt.Println("🧹 State reset to Idle.")

			if flagHistory {
				histPath := dir + "/history.jsonl"
				if err := os.Remove(histPath); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("removing history: %w", err)
				}
				fmt.Println("🗑  History wiped.")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&flagHistory, "history", false, "Also delete history.jsonl")
	return cmd
}

// ── `pomodoro init` ───────────────────────────────────────────────────────────

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create a default settings file if one does not exist",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.WriteDefaults(); err != nil {
				return err
			}
			fmt.Println("✅ Settings file ready at ~/.config/open-pomodoro/settings")
			return nil
		},
	}
}

// ── Utilities ─────────────────────────────────────────────────────────────────

func modeEmoji(s state.SessionState) string {
	switch s {
	case state.StateWork:
		return "🍅 Work"
	case state.StateBreak:
		return "☕ Break"
	case state.StatePause:
		return "⏸  Paused"
	default:
		return "⚪ Idle"
	}
}

func replaceAll(s string, pairs map[string]string) string {
	for old, newVal := range pairs {
		result := ""
		for i := 0; i < len(s); {
			if i+len(old) <= len(s) && s[i:i+len(old)] == old {
				result += newVal
				i += len(old)
			} else {
				result += string(s[i])
				i++
			}
		}
		s = result
	}
	return s
}

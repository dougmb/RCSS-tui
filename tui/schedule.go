package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/scheduler"
)

// Schedule screen: presets that register RCSS jobs with the host OS scheduler
// (crontab on Unix, Task Scheduler on Windows; no root/admin) calling the rcss
// binary headless — `rcss upload` daily and/or `rcss clean` weekly. Disabling
// both removes the managed jobs.

type scheduleState int

const (
	scForm scheduleState = iota
	scDone
)

type scheduleModel struct {
	cfg  config.Config
	form *huh.Form

	uploadEnabled bool
	uploadTime    string
	cleanEnabled  bool
	cleanTime     string

	state   scheduleState
	status  string
	failed  bool
	current []scheduler.Job
}

// parseHHMM validates and splits a "HH:MM" time.
func parseHHMM(s string) (hour, min int, err error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("use HH:MM")
	}
	hour, err = strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("hour must be 0–23")
	}
	min, err = strconv.Atoi(parts[1])
	if err != nil || min < 0 || min > 59 {
		return 0, 0, fmt.Errorf("minute must be 0–59")
	}
	return hour, min, nil
}

func validTime(s string) error { _, _, err := parseHHMM(s); return err }

func newScheduleModel(cfg config.Config) scheduleModel {
	current, _ := scheduler.Current(cfg.RemoteName)
	s := scheduleModel{
		cfg:        cfg,
		uploadTime: "03:00",
		cleanTime:  "05:00",
		state:      scForm,
		current:    current,
	}
	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().Key("up").Title("Schedule daily upload?").Value(&s.uploadEnabled),
			huh.NewInput().Key("uptime").Title("Upload time (HH:MM)").
				Validate(validTime).Value(&s.uploadTime),
		),
		huh.NewGroup(
			huh.NewConfirm().Key("cl").Title("Schedule weekly clean (Sundays)?").Value(&s.cleanEnabled),
			huh.NewInput().Key("cltime").Title("Clean time (HH:MM)").
				Validate(validTime).Value(&s.cleanTime),
		),
	).WithShowHelp(false)
	return s
}

func (s scheduleModel) Init() tea.Cmd { return s.form.Init() }

func (s *scheduleModel) setSize(w, h int) {
	s.form = s.form.WithWidth(w).WithHeight(h - 4)
}

// apply registers the chosen presets with the OS scheduler.
func (s scheduleModel) apply() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating rcss binary: %w", err)
	}
	logPath, err := s.cfg.ResolveLogFile()
	if err != nil {
		return err
	}

	var jobs []scheduler.Job
	if s.uploadEnabled {
		h, m, _ := parseHHMM(s.uploadTime)
		jobs = append(jobs, scheduler.Job{Kind: scheduler.Upload, Hour: h, Min: m})
	}
	if s.cleanEnabled {
		h, m, _ := parseHHMM(s.cleanTime)
		jobs = append(jobs, scheduler.Job{Kind: scheduler.Clean, Hour: h, Min: m, Weekly: true})
	}
	return scheduler.Apply(s.cfg.RemoteName, jobs, exe, logPath)
}

func (s scheduleModel) Update(msg tea.Msg) (scheduleModel, tea.Cmd) {
	if s.state == scDone {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "enter", "esc", "backspace":
				return s, func() tea.Msg { return goBackMsg{} }
			case "q":
				return s, tea.Quit
			}
		}
		return s, nil
	}

	form, cmd := s.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		s.form = f
	}

	switch s.form.State {
	case huh.StateCompleted:
		if err := s.apply(); err != nil {
			s.status, s.failed = fmt.Sprintf("Failed to update %s: %s", scheduler.Backend(), err.Error()), true
		} else if !s.uploadEnabled && !s.cleanEnabled {
			s.status, s.failed = "Schedule cleared — RCSS jobs removed.", false
		} else {
			s.status, s.failed = scheduler.Backend()+" updated.", false
		}
		s.current, _ = scheduler.Current(s.cfg.RemoteName)
		s.state = scDone
		return s, nil
	case huh.StateAborted:
		return s, func() tea.Msg { return goBackMsg{} }
	}
	return s, cmd
}

func (s scheduleModel) View() string {
	if s.state == scDone {
		var b strings.Builder
		if s.failed {
			b.WriteString(errorStyle.Render("✗ " + s.status))
		} else {
			b.WriteString(titleStyle.Render("✓ " + s.status))
		}
		b.WriteString("\n\n")
		b.WriteString(subtitleStyle.Render(s.currentScheduleText()))
		return b.String()
	}

	header := titleStyle.Render("Schedule") + "\n" +
		subtitleStyle.Render(s.currentScheduleText()) + "\n\n"
	return header + s.form.View()
}

func (s scheduleModel) footerHint() string {
	if s.state == scDone {
		return "enter/esc back • q quit"
	}
	return "tab/↑↓ navigate • enter confirm • esc cancel"
}

// currentScheduleText summarizes the active account's managed jobs.
func (s scheduleModel) currentScheduleText() string {
	if len(s.current) == 0 {
		return fmt.Sprintf("Current: no jobs scheduled for %s (%s).", s.cfg.RemoteName, scheduler.Backend())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Scheduled for %s (%s):", s.cfg.RemoteName, scheduler.Backend())
	for _, j := range s.current {
		fmt.Fprintf(&b, "\n  • %s — %s at %s", j.Kind.Title(), j.Cadence(), j.Time())
	}
	return b.String()
}

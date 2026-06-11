package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/cron"
)

// Schedule screen: presets that write a managed block to the user's crontab
// (no root) calling the rcss binary headless — `rcss upload` daily and/or
// `rcss clean` weekly. Disabling both removes the block.

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
	current []string
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
	current, _ := cron.ManagedLines()
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

// apply writes the crontab block from the chosen presets.
func (s scheduleModel) apply() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating rcss binary: %w", err)
	}
	logPath, err := s.cfg.ResolveLogFile()
	if err != nil {
		return err
	}

	var lines []string
	if s.uploadEnabled {
		h, m, _ := parseHHMM(s.uploadTime)
		lines = append(lines, fmt.Sprintf("%d %d * * * %s upload >> %s 2>&1", m, h, exe, logPath))
	}
	if s.cleanEnabled {
		h, m, _ := parseHHMM(s.cleanTime)
		lines = append(lines, fmt.Sprintf("%d %d * * 0 %s clean >> %s 2>&1", m, h, exe, logPath))
	}
	return cron.SetManaged(lines)
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
			s.status, s.failed = "Failed to update crontab: "+err.Error(), true
		} else if !s.uploadEnabled && !s.cleanEnabled {
			s.status, s.failed = "Schedule cleared — RCSS crontab block removed.", false
		} else {
			s.status, s.failed = "Crontab updated.", false
		}
		s.current, _ = cron.ManagedLines()
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
		b.WriteString(subtitleStyle.Render(currentScheduleText(s.current)))
		return b.String()
	}

	header := titleStyle.Render("Schedule") + "\n" +
		subtitleStyle.Render(currentScheduleText(s.current)) + "\n\n"
	return header + s.form.View()
}

func (s scheduleModel) footerHint() string {
	if s.state == scDone {
		return "enter/esc back • q quit"
	}
	return "tab/↑↓ navigate • enter confirm • esc cancel"
}

// currentScheduleText summarizes the active managed crontab lines.
func currentScheduleText(lines []string) string {
	if len(lines) == 0 {
		return "Current: no RCSS cron jobs scheduled."
	}
	return "Current managed crontab:\n" + strings.Join(lines, "\n")
}

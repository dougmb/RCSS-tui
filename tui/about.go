package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/dougmb/rcss-tui/backup"
	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/scheduler"
)

// About screen: app identity, version, dependency status, and file locations.
type aboutModel struct {
	cfg           config.Config
	rcloneMissing bool
	accountCount  int
	width         int
	height        int
	offset        int
}

func newAboutModel(cfg config.Config, rcloneMissing bool, accountCount int) aboutModel {
	return aboutModel{
		cfg: cfg, rcloneMissing: rcloneMissing, accountCount: accountCount,
		width: 80, height: 14,
	}
}

func (a *aboutModel) setSize(w, h int) {
	a.width, a.height = w, h
	a.clampOffset()
}

func (a aboutModel) viewportHeight() int {
	h := a.height - 1 // fixed title
	if h < 1 {
		return 1
	}
	return h
}

func (a aboutModel) maxOffset() int {
	max := len(a.contentLines()) - a.viewportHeight()
	if max < 0 {
		return 0
	}
	return max
}

func (a *aboutModel) clampOffset() {
	if a.offset < 0 {
		a.offset = 0
	}
	if max := a.maxOffset(); a.offset > max {
		a.offset = max
	}
}

func (a aboutModel) Update(msg tea.Msg) (aboutModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q":
			return a, tea.Quit
		case "esc", "backspace", "enter":
			return a, func() tea.Msg { return goBackMsg{} }
		case "up", "k":
			a.offset--
		case "down", "j":
			a.offset++
		case "pgup":
			a.offset -= a.viewportHeight()
		case "pgdown":
			a.offset += a.viewportHeight()
		case "home", "g":
			a.offset = 0
		case "end", "G":
			a.offset = a.maxOffset()
		}
		a.clampOffset()
	}
	return a, nil
}

func (a aboutModel) View() string {
	cw := a.width - 1 // reserve one column for the scrollbar
	if cw < 10 {
		cw = 10
	}
	height := a.viewportHeight()
	lines := a.contentLines()
	a.clampOffset()
	bar := scrollColumn(height, len(lines), a.offset)
	rows := make([]string, height)
	for i := 0; i < height; i++ {
		line := ""
		if a.offset+i < len(lines) {
			line = ansi.Truncate(lines[a.offset+i], cw, "…")
		}
		rows[i] = padLineTo(line, cw) + bar[i]
	}

	title := titleStyle.Render(clip("RCSS — Rclone Cloud Simple Scripts", cw))
	return title + "\n" + strings.Join(rows, "\n")
}

func (a aboutModel) contentLines() []string {
	lines := []string{
		subtitleStyle.Render("Per-project backups to an rclone cloud remote, in your terminal."),
		"",
		infoLine("Version", appVersion()),
	}
	active := a.cfg.RemoteName
	if active == "" {
		active = "—"
	}
	lines = append(lines,
		infoLine("Active account", active),
		infoLine("Accounts", fmt.Sprintf("%d", a.accountCount)),
	)
	if a.cfg.RemoteName != "" {
		lines = append(lines, lastBackupLine(a.cfg))
	}
	if a.rcloneMissing {
		lines = append(lines, infoLine("rclone", "")+warnStyle.Render("not found on PATH"))
	} else {
		lines = append(lines, infoLine("rclone", "installed"))
	}
	lines = append(lines, infoLine("Scheduler", scheduler.Backend()), "")
	if path, err := config.Path(); err == nil {
		lines = append(lines, infoLine("Config", path))
	}
	if logPath, err := a.cfg.ResolveLogFile(); err == nil {
		lines = append(lines, infoLine("Log", logPath))
	}
	lines = append(lines, "",
		infoLine("Project", "https://github.com/dougmb/RCSS-tui"),
		infoLine("rclone", "https://rclone.org"),
	)
	return lines
}

func (a aboutModel) footerHint() string {
	return "↑/↓ scroll • pgup/pgdown page • esc back • q quit"
}

// lastBackupLine renders an info line summarizing the most recent backup run.
func lastBackupLine(cfg config.Config) string {
	logPath, err := cfg.ResolveLogFile()
	if err != nil {
		return infoLine("Last backup", "unknown")
	}
	info, ok, err := backup.LastRun(logPath)
	if err != nil || !ok {
		return infoLine("Last backup", "never")
	}
	line := infoLine("Last backup", info.Time.Format("2006-01-02 15:04"))
	tag := "(" + info.Status + ")"
	if info.Status == "SUCCESS" {
		return line + "  " + okStyle.Render(tag)
	}
	return line + "  " + warnStyle.Render(tag)
}

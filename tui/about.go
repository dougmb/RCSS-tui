package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/scheduler"
)

// About screen: app identity, version, the rclone dependency status, and where
// RCSS keeps its config and log. Read-only — esc/backspace returns to the menu.

type aboutModel struct {
	cfg           config.Config
	rcloneMissing bool
	accountCount  int
}

func newAboutModel(cfg config.Config, rcloneMissing bool, accountCount int) aboutModel {
	return aboutModel{cfg: cfg, rcloneMissing: rcloneMissing, accountCount: accountCount}
}

func (a aboutModel) Update(msg tea.Msg) (aboutModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q":
			return a, tea.Quit
		case "esc", "backspace", "enter":
			return a, func() tea.Msg { return goBackMsg{} }
		}
	}
	return a, nil
}

func (a aboutModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("RCSS — Rclone Cloud Simple Scripts"))
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render("Per-project backups to an rclone cloud remote, in your terminal."))
	b.WriteString("\n\n")

	b.WriteString(infoLine("Version", appVersion()))
	b.WriteString("\n")
	active := a.cfg.RemoteName
	if active == "" {
		active = "—"
	}
	b.WriteString(infoLine("Active account", active))
	b.WriteString("\n")
	b.WriteString(infoLine("Accounts", fmt.Sprintf("%d", a.accountCount)))
	b.WriteString("\n")

	if a.rcloneMissing {
		b.WriteString(infoLine("rclone", "") + warnStyle.Render("not found on PATH"))
	} else {
		b.WriteString(infoLine("rclone", "installed"))
	}
	b.WriteString("\n")
	b.WriteString(infoLine("Scheduler", scheduler.Backend()))
	b.WriteString("\n\n")

	if path, err := config.Path(); err == nil {
		b.WriteString(infoLine("Config", path))
		b.WriteString("\n")
	}
	if logPath, err := a.cfg.ResolveLogFile(); err == nil {
		b.WriteString(infoLine("Log", logPath))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(infoLine("Project", "https://github.com/dougmb/RCSS-tui"))
	b.WriteString("\n")
	b.WriteString(infoLine("rclone", "https://rclone.org"))

	return b.String()
}

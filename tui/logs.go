package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/dougmb/rcss-tui/config"
)

// Logs screen: a scrollable viewport over the sync log with ERROR/WARN lines
// highlighted. Reads the same file the upload and clean operations append to.
type logsModel struct {
	cfg           config.Config
	vp            viewport.Model
	path          string
	width, height int
}

func newLogsModel(cfg config.Config) logsModel {
	return logsModel{cfg: cfg, vp: viewport.New(79, 14), width: 80, height: 14}
}

func (l *logsModel) setSize(w, h int) {
	if w < 2 {
		w = 2
	}
	if h < 1 {
		h = 1
	}
	l.width, l.height = w, h
	l.vp.Width = w - 1 // reserve the last column for the scrollbar
	l.vp.Height = h
}

// reload reads the log file and refreshes the viewport, scrolled to the end.
func (l *logsModel) reload() {
	path, err := l.cfg.ResolveLogFile()
	if err != nil {
		l.vp.SetContent(errorStyle.Render(err.Error()))
		return
	}
	l.path = path

	data, err := os.ReadFile(path)
	if err != nil {
		l.vp.SetContent(subtitleStyle.Render("No log yet at " + path))
		return
	}
	l.vp.SetContent(colorizeLog(string(data)))
	l.vp.GotoBottom()
}

func (l logsModel) Update(msg tea.Msg) (logsModel, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "q":
			return l, tea.Quit
		case "esc", "backspace":
			return l, func() tea.Msg { return goBackMsg{} }
		case "r":
			l.reload()
			return l, nil
		}
	}
	var cmd tea.Cmd
	l.vp, cmd = l.vp.Update(msg)
	return l, cmd
}

func (l logsModel) View() string {
	cw := l.width - 1
	if cw < 1 {
		cw = 1
	}
	height := l.height
	if height < 1 {
		height = 1
	}
	visible := strings.Split(l.vp.View(), "\n")
	bar := scrollColumn(height, l.vp.TotalLineCount(), l.vp.YOffset)
	rows := make([]string, height)
	for i := 0; i < height; i++ {
		line := ""
		if i < len(visible) {
			line = ansi.Truncate(visible[i], cw, "…")
		}
		rows[i] = padLineTo(line, cw) + bar[i]
	}
	return strings.Join(rows, "\n")
}

// colorizeLog highlights ERROR (red) and WARN (amber) lines in the log dump.
func colorizeLog(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		switch {
		case strings.Contains(line, "[ERROR"):
			lines[i] = errorStyle.Render(line)
		case strings.Contains(line, "[WARN"):
			lines[i] = warnStyle.Render(line)
		}
	}
	return strings.Join(lines, "\n")
}

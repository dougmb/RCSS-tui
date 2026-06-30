package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dougmb/rcss-tui/config"
)

func TestAboutViewStaysInsideSmallPanel(t *testing.T) {
	a := newAboutModel(config.Config{
		RemoteName: "a-very-long-remote-name-that-must-not-overflow:",
		LogFile:    "/a/very/long/path/that/must/be/truncated/backup.log",
	}, false, 3)
	a.setSize(24, 6)

	assertAboutBounds(t, a.View(), 24, 6)
	a, _ = a.Update(tea.KeyMsg{Type: tea.KeyEnd})
	view := a.View()
	assertAboutBounds(t, view, 24, 6)
	if !strings.Contains(view, "Project") || !strings.Contains(view, "rclone") {
		t.Fatalf("last rows should be reachable by scrolling:\n%s", view)
	}
}

func assertAboutBounds(t *testing.T, view string, width, height int) {
	t.Helper()
	lines := strings.Split(view, "\n")
	if len(lines) != height {
		t.Fatalf("view height = %d, want %d:\n%s", len(lines), height, view)
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("line %d width = %d, want <= %d: %q", i, got, width, line)
		}
	}
}

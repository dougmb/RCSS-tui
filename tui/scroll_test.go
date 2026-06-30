package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dougmb/rcss-tui/backup"
	"github.com/dougmb/rcss-tui/config"
)

func TestLogsShowsScrollbarOnlyWhenContentOverflows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup.log")
	var content strings.Builder
	for i := 0; i < 20; i++ {
		fmt.Fprintf(&content, "line %02d\n", i)
	}
	if err := os.WriteFile(path, []byte(content.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	logs := newLogsModel(config.Config{LogFile: path})
	logs.setSize(24, 6)
	logs.reload()
	view := logs.View()
	assertPanelBounds(t, view, 24, 6)
	if !strings.Contains(view, "█") {
		t.Fatalf("overflowing log should show a scrollbar:\n%s", view)
	}

	logs.vp.SetContent("one line")
	if view := logs.View(); strings.Contains(view, "█") || strings.Contains(view, "│") {
		t.Fatalf("short log should not show a scrollbar:\n%s", view)
	}
}

func TestSourcesShowsScrollbarAndKeepsCursorVisible(t *testing.T) {
	folders := make([]string, 10)
	for i := range folders {
		folders[i] = fmt.Sprintf("/source/folder-%02d", i)
	}
	s := newSourcesModel(folders, "/")
	s.setSize(24, 6)
	for i := 0; i < len(folders)-1; i++ {
		s, _ = s.Update(keyTypeDown())
	}
	view := s.View()
	assertPanelBounds(t, view, 24, 6)
	if !strings.Contains(view, "█") || !strings.Contains(view, "folder-09") {
		t.Fatalf("source list should scroll to the cursor and show a bar:\n%s", view)
	}
}

func TestCleanReportShowsScrollbarForCandidates(t *testing.T) {
	c := newCleanModel(config.Config{}, missingRclone())
	c.setSize(40, 6)
	c.state = clReport
	for i := 0; i < 10; i++ {
		size := int64(i + 1)
		c.preview.Candidates = append(c.preview.Candidates, backup.CleanupCandidate{
			Path: fmt.Sprintf("folder/file-%02d", i), Size: &size, ModTime: time.Now(),
		})
		c.selected = append(c.selected, true)
	}
	view := c.View()
	assertPanelBounds(t, view, 40, 6)
	if !strings.Contains(view, "█") {
		t.Fatalf("candidate list should show a scrollbar:\n%s", view)
	}
}

func keyTypeDown() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyDown} }

func assertPanelBounds(t *testing.T, view string, width, height int) {
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

func TestStaticOperationScreensFitSmallPanel(t *testing.T) {
	u := newUploadModel(config.Config{RemoteName: "a-very-long-remote-name-that-must-be-clipped:"}, missingRclone())
	u.setSize(40, 6)
	assertPanelBounds(t, u.View(), 40, 6)

	c := newCleanModel(config.Config{
		RemoteName:          "a-very-long-remote-name-that-must-be-clipped:",
		RemoteRetentionDays: 15, RemoteCleanupSafetyDays: 2,
	}, missingRclone())
	c.setSize(40, 6)
	assertPanelBounds(t, c.View(), 40, 6)
}

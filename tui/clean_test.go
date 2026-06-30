package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dougmb/rcss-tui/backup"
	"github.com/dougmb/rcss-tui/config"
)

func TestCleanActivityBarAdvancesWhileRcloneRuns(t *testing.T) {
	c := newCleanModel(config.Config{}, missingRclone())
	c.state = clDryRunning
	c.startedAt = time.Now()

	before := strings.Split(c.View(), "\n")[2]
	c, cmd := c.Update(cleanActivityTickMsg(time.Now()))
	after := strings.Split(c.View(), "\n")[2]
	if cmd == nil {
		t.Fatal("activity tick must schedule the next update")
	}
	if before == after {
		t.Fatalf("activity bar did not advance: %q", before)
	}
	if !strings.Contains(c.View(), "Elapsed:") || !strings.Contains(c.View(), "waiting for rclone") {
		t.Fatalf("missing activity context:\n%s", c.View())
	}

	c.state = clReport
	c, cmd = c.Update(cleanActivityTickMsg(time.Now()))
	if cmd != nil {
		t.Fatal("activity tick must stop after the operation finishes")
	}
}

func TestCleanSelectorAndTypedConfirmation(t *testing.T) {
	a, b := int64(10), int64(20)
	c := newCleanModel(config.Config{RemoteName: "drive:", RemoteDestination: "Backups", RemoteRetentionDays: 15}, missingRclone())
	p := backup.CleanPreview{Candidates: []backup.CleanupCandidate{
		{Path: "a", Size: &a, ModTime: time.Unix(1, 0)},
		{Path: "b", Size: &b, ModTime: time.Unix(2, 0)},
	}}
	c, _ = c.Update(cleanPreviewMsg{preview: p})
	if c.state != clReport || c.selectedCount() != 2 {
		t.Fatalf("initial state=%v selected=%d", c.state, c.selectedCount())
	}
	c, _ = c.Update(keyRune(' '))
	bytes, unknown := c.selectedTotals()
	if c.selectedCount() != 1 || bytes != 20 || unknown != 0 {
		t.Fatalf("selected=%d bytes=%d unknown=%d", c.selectedCount(), bytes, unknown)
	}
	c, _ = c.Update(keyRune('a'))
	if c.selectedCount() != 2 {
		t.Fatal("a should select all")
	}
	c, _ = c.Update(keyRune('a'))
	if c.selectedCount() != 0 {
		t.Fatal("a should clear all")
	}
	c, _ = c.Update(keyRune('x'))
	if c.state != clReport {
		t.Fatal("empty selection advanced")
	}
	c, _ = c.Update(keyRune('a'))
	c, _ = c.Update(keyRune('x'))
	if c.state != clConfirmSummary {
		t.Fatal("missing visual confirmation")
	}
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if c.state != clConfirmPhrase {
		t.Fatal("missing typed confirmation")
	}
	c.input.SetValue("delete drive:/Backups")
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if c.state != clConfirmPhrase || c.err == nil {
		t.Fatal("incorrect phrase should not execute")
	}
	c.input.SetValue("  DELETE drive:/Backups  ")
	c, cmd := c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if c.state != clRunning || cmd == nil {
		t.Fatal("correct phrase should start execution")
	}
}

package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmb/rcss-tui/backup"
	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

// missingRclone returns a client pointing at a binary that cannot exist, so the
// root model treats rclone as not installed regardless of the test host.
func missingRclone() *rclone.Client { return &rclone.Client{Bin: "rclone-does-not-exist-zzz"} }

func keyRune(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

// sized builds a root model and gives it a usable terminal size, returning it
// as a Model ready to drive. The store starts empty (no account configured).
func sized() Model {
	m := New(&config.Store{}, missingRclone())
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return tm.(Model)
}

// TestMissingRcloneOpensAndWarns checks the dependency fix: a missing rclone is
// recorded (not fatal) and surfaced in the menu preview.
func TestMissingRcloneOpensAndWarns(t *testing.T) {
	m := sized()
	if !m.rcloneMissing {
		t.Fatal("expected rcloneMissing to be true")
	}
	if got := m.View(); !strings.Contains(got, "rclone not found") {
		t.Errorf("expected rclone warning in preview, got:\n%s", got)
	}
}

// TestRcloneMissingLocksScreen checks that jumping to a cloud screen while
// rclone is missing locks it with an install hint instead of erroring out.
func TestRcloneMissingLocksScreen(t *testing.T) {
	m := sized()
	// '3' jumps to Back Up Now (3rd menu item), which requires rclone.
	tm, _ := m.Update(keyRune('3'))
	m = tm.(Model)
	if m.screen != screenUpload {
		t.Fatalf("expected screenUpload, got %v", m.screen)
	}
	if !m.locked {
		t.Fatal("expected the screen to be locked")
	}
	if got := m.View(); !strings.Contains(got, "rclone is not installed") {
		t.Errorf("expected locked install hint, got:\n%s", got)
	}
}

// TestNumberJumpToLocalScreen checks quick-jump opens a screen that does not
// need rclone (Logs, the 8th item) and moves focus into the detail pane.
func TestNumberJumpToLocalScreen(t *testing.T) {
	m := sized()
	tm, _ := m.Update(keyRune('8'))
	m = tm.(Model)
	if m.screen != screenLogs {
		t.Fatalf("expected screenLogs, got %v", m.screen)
	}
	if m.locked {
		t.Error("Logs should not be locked when rclone is missing")
	}
	if m.focus != focusDetail {
		t.Error("expected focus to move to the detail pane")
	}
}

// TestHelpOverlayToggles checks the `?` help overlay opens and closes.
func TestHelpOverlayToggles(t *testing.T) {
	m := sized()
	tm, _ := m.Update(keyRune('?'))
	m = tm.(Model)
	if !m.showHelp {
		t.Fatal("expected help overlay to be shown")
	}
	if got := m.View(); !strings.Contains(got, "Keyboard shortcuts") {
		t.Errorf("expected help overlay content, got:\n%s", got)
	}
	tm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = tm.(Model)
	if m.showHelp {
		t.Error("expected esc to close the help overlay")
	}
}

// TestActiveAccountIndicator checks the active account is derived from the
// store and shown in the sidebar (requirement 3).
func TestActiveAccountIndicator(t *testing.T) {
	store := &config.Store{ActiveAccount: "drive:", Accounts: []config.Config{config.NewAccount("drive:")}}
	m := New(store, missingRclone())
	tm, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = tm.(Model)
	if m.cfg.RemoteName != "drive:" {
		t.Fatalf("expected active account drive:, got %q", m.cfg.RemoteName)
	}
	if got := m.View(); !strings.Contains(got, "drive:") {
		t.Errorf("expected active account in the sidebar, got:\n%s", got)
	}
}

// TestAboutReachableAndRenders checks the About screen opens via its number
// shortcut (it never needs rclone) and shows the version.
func TestAboutReachableAndRenders(t *testing.T) {
	m := sized()
	tm, _ := m.Update(keyRune('9'))
	m = tm.(Model)
	if m.screen != screenAbout {
		t.Fatalf("expected screenAbout, got %v", m.screen)
	}
	if m.locked {
		t.Error("About should never be locked")
	}
	if got := m.View(); !strings.Contains(got, "RCSS") || !strings.Contains(got, "Version") {
		t.Errorf("expected About content, got:\n%s", got)
	}
}

// TestCleanSelectionAndConfirm checks selection and the two confirmation stages.
func TestCleanSelectionAndConfirm(t *testing.T) {
	cfg := config.Config{RemoteName: "r:", RemoteDestination: "Backups", RemoteRetentionDays: 15, RemoteCleanupSafetyDays: 2}
	c := newCleanModel(cfg, missingRclone())
	if c.state != clIntro {
		t.Fatalf("expected clIntro, got %v", c.state)
	}
	if got := c.View(); !strings.Contains(got, "CLOUD") || !strings.Contains(got, "Local files are NOT touched") {
		t.Errorf("intro should explain cloud-vs-local, got:\n%s", got)
	}

	c, _ = c.Update(keyRune('f'))
	if !c.force {
		t.Fatal("expected force on after 'f'")
	}
	size := int64(42)
	c.state = clReport
	c.preview = backup.CleanPreview{Candidates: []backup.CleanupCandidate{{Path: "old/file", Size: &size}}}
	c.selected = []bool{true}
	c, _ = c.Update(keyRune('x'))
	if c.state != clConfirmSummary {
		t.Fatalf("expected summary confirmation after x, got %v", c.state)
	}
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if c.state != clReport {
		t.Fatalf("expected cancel back to report, got %v", c.state)
	}
	c, _ = c.Update(keyRune(' '))
	c, _ = c.Update(keyRune('x'))
	if c.state != clReport {
		t.Fatal("empty selection must block deletion")
	}
}

// TestSettingsSavedConfirmation checks the done state renders a visible
// confirmation (success and error paths).
func TestSettingsSavedConfirmation(t *testing.T) {
	s := newSettingsModel(config.Config{RemoteName: "r:", SourceFolders: []string{"/tmp"}, RemoteDestination: "Backups"})
	s.done = true
	if got := s.View(); !strings.Contains(got, "Settings saved") {
		t.Errorf("expected saved confirmation, got:\n%s", got)
	}
	if s.footerHint() != "enter/esc back • q quit" {
		t.Errorf("unexpected done footer: %q", s.footerHint())
	}
	s.saveErr = fmt.Errorf("disk full")
	if got := s.View(); !strings.Contains(got, "Could not save") {
		t.Errorf("expected error view, got:\n%s", got)
	}
}

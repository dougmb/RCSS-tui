package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmb/rcss-tui/config"
)

func sendSet(s settingsModel, k tea.KeyMsg) settingsModel {
	s, _ = s.Update(k)
	return s
}

var spaceKey = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}}

// drainSaved runs the command a save() returns and extracts the emitted config.
func drainSaved(t *testing.T, cmd tea.Cmd) config.Config {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command from save, got nil")
	}
	return extractSaved(t, cmd())
}

// extractSaved walks a BatchMsg (or single message) and returns the first
// settingsSavedMsg it finds.
func extractSaved(t *testing.T, msg tea.Msg) config.Config {
	t.Helper()
	switch m := msg.(type) {
	case settingsSavedMsg:
		return m.cfg
	case tea.BatchMsg:
		for _, c := range m {
			if c != nil {
				if cfg, ok := tryExtractSaved(c()); ok {
					return cfg
				}
			}
		}
	}
	t.Fatalf("expected settingsSavedMsg inside command output, got %T", msg)
	return config.Config{}
}

func tryExtractSaved(msg tea.Msg) (config.Config, bool) {
	switch m := msg.(type) {
	case settingsSavedMsg:
		return m.cfg, true
	case tea.BatchMsg:
		for _, c := range m {
			if c != nil {
				if cfg, ok := tryExtractSaved(c()); ok {
					return cfg, true
				}
			}
		}
	}
	return config.Config{}, false
}

func settingsVisibleHas(s settingsModel, key string) bool {
	target := s.fieldIndex(key)
	for _, fi := range s.visibleIndices() {
		if fi == target {
			return true
		}
	}
	return false
}

// TestSettingsNestedRevealAndSave drives the editor headless: a sub-setting is
// hidden until its parent toggle is enabled, then editable; the saved config
// reflects the nested values. It also checks the digit-only numeric filter and
// the "Skip file formats" default pre-fill.
func TestSettingsNestedRevealAndSave(t *testing.T) {
	s := newSettingsModel(config.Config{RemoteName: "drive:", RemoteRetentionDays: 15, RemoteCleanupSafetyDays: 2})
	s.setSize(80, 24)
	down := keyType(tea.KeyDown)

	if settingsVisibleHas(s, "ret") {
		t.Fatal("'Keep local files for (days)' should be hidden while delete-after-upload is off")
	}

	// Walk to "Delete local after upload" (visible index 4) and enable it.
	for i := 0; i < 4; i++ {
		s = sendSet(s, down)
	}
	s = sendSet(s, spaceKey)
	if !settingsVisibleHas(s, "ret") {
		t.Fatal("enabling delete-after-upload should reveal the retention sub-setting")
	}

	// Into the revealed retention field: clear, reject a letter, type 14.
	s = sendSet(s, down)
	s = sendSet(s, keyType(tea.KeyBackspace)) // clear the "0" default
	s = sendSet(s, keyRune('x'))              // non-digit ignored
	s = sendSet(s, keyRune('1'))
	s = sendSet(s, keyRune('4'))

	// Next field is "Skip file formats"; enable it to pre-fill defaults.
	s = sendSet(s, down)
	s = sendSet(s, spaceKey)
	if !settingsVisibleHas(s, "formats") {
		t.Fatal("enabling skip-formats should reveal the formats box")
	}

	got := drainSaved(t, mustSave(t, &s))
	if !got.DeleteAfterUpload {
		t.Error("DeleteAfterUpload should be on")
	}
	if got.RetentionDays != 14 {
		t.Errorf("RetentionDays = %d, want 14", got.RetentionDays)
	}
	if strings.Join(got.SkipFormats, " ") != defaultSkipFormats {
		t.Errorf("SkipFormats = %v, want the defaults %q", got.SkipFormats, defaultSkipFormats)
	}
	if got.RemoteRetentionDays != 15 {
		t.Errorf("RemoteRetentionDays = %d, want 15 (untouched)", got.RemoteRetentionDays)
	}
}

// mustSave calls save() and returns the command, failing if none is produced.
func mustSave(t *testing.T, s *settingsModel) tea.Cmd {
	t.Helper()
	m, cmd := s.save()
	*s = m
	return cmd
}

// TestSettingsDisabledToggleDropsSubSetting checks a sub-setting's value is not
// persisted while its parent toggle is off (no skip formats when off).
func TestSettingsDisabledToggleDropsSubSetting(t *testing.T) {
	s := newSettingsModel(config.Config{RemoteName: "drive:", SkipFormats: []string{"tmp"}})
	s.setSize(80, 24)
	// skipfmt starts on (SkipFormats non-empty); toggle it off.
	for i := 0; i < 5; i++ { // index 5 = skipfmt when delete-after-upload is off
		s = sendSet(s, keyType(tea.KeyDown))
	}
	if !settingsVisibleHas(s, "formats") {
		t.Fatal("formats should be visible while skip-formats is on")
	}
	s = sendSet(s, spaceKey) // turn skip-formats off
	got := drainSaved(t, mustSave(t, &s))
	if len(got.SkipFormats) != 0 {
		t.Errorf("SkipFormats should be empty when the toggle is off, got %v", got.SkipFormats)
	}
}

// TestSettingsEmptyNumericBlocksSave checks that clearing a numeric field shows
// an inline error and refuses to save (no config emitted).
func TestSettingsEmptyNumericBlocksSave(t *testing.T) {
	s := newSettingsModel(config.Config{RemoteName: "r:", RemoteRetentionDays: 0})
	s.setSize(80, 24)

	for i := 0; i < 2; i++ { // index 2 = Remote retention (days)
		s = sendSet(s, keyType(tea.KeyDown))
	}
	for i := 0; i < 3; i++ {
		s = sendSet(s, keyType(tea.KeyBackspace))
	}

	s2, cmd := s.save()
	if cmd != nil {
		t.Error("save should not emit a config when a numeric field is empty")
	}
	if s2.done {
		t.Error("save should not flip done on a validation error")
	}
	if !s2.failed || !strings.Contains(s2.status, "must be a number") {
		t.Errorf("expected an inline number error, got failed=%v status=%q", s2.failed, s2.status)
	}
}

// TestSettingsStatusHint checks the focused row exposes its help for the root
// status bar, and the Save row gets a save hint.
func TestSettingsStatusHint(t *testing.T) {
	s := newSettingsModel(config.Config{RemoteName: "drive:"})
	s.setSize(80, 24)

	if h := s.statusHint(); !strings.Contains(h, "remote") {
		t.Errorf("status hint for first field = %q", h)
	}
	// Jump focus to the Save row.
	s.focus = len(s.visibleIndices())
	if h := s.statusHint(); !strings.Contains(strings.ToLower(h), "save") {
		t.Errorf("status hint on Save = %q", h)
	}
}

// TestSettingsViewHasScrollbar checks the page renders a header and, when the
// content is taller than the window, a scrollbar thumb.
func TestSettingsViewHasScrollbar(t *testing.T) {
	s := newSettingsModel(config.Config{RemoteName: "drive:"})
	s.setSize(80, 10) // short window forces scrolling

	v := s.View()
	if !strings.Contains(v, "Settings — drive:") {
		t.Error("view should render the Settings header")
	}
	if !strings.Contains(v, "█") {
		t.Error("a short window should render a scrollbar thumb")
	}
}

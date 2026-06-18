package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"github.com/dougmb/rcss-tui/config"
)

// Settings screen: a huh form editing config.toml. Submitting saves the TOML;
// aborting (esc) returns to the menu without changes. Replaces hand-editing
// backup.env.

// settingsSavedMsg carries the edited config back to the root for persistence.
type settingsSavedMsg struct{ cfg config.Config }

type settingsModel struct {
	form *huh.Form

	remoteName, syncRoot, driveDest        string
	retention, remoteRetention, safetyDays string
	deleteAfter, skipDotfiles              bool
	ignored, logFile                       string

	// done flips to true once the form is submitted, so the screen shows a
	// "Saved ✓" confirmation instead of silently returning to the menu. saveErr
	// is set by the root after it persists the config.
	done    bool
	saveErr error
}

// numeric validates that a huh input holds a non-negative integer.
func numeric(s string) error {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return fmt.Errorf("must be a non-negative number")
	}
	return nil
}

func newSettingsModel(cfg config.Config) settingsModel {
	s := settingsModel{
		remoteName:      cfg.RemoteName,
		syncRoot:        cfg.SourceRoot,
		driveDest:       cfg.RemoteDestination,
		retention:       strconv.Itoa(cfg.RetentionDays),
		remoteRetention: strconv.Itoa(cfg.RemoteRetentionDays),
		safetyDays:      strconv.Itoa(cfg.RemoteCleanupSafetyDays),
		deleteAfter:     cfg.DeleteAfterUpload,
		skipDotfiles:    cfg.SkipDotfiles,
		ignored:         strings.Join(cfg.IgnoredFolders, " "),
		logFile:         cfg.LogFile,
	}

	// The account's rclone remote is chosen on the Account screen, not here, so
	// these settings stay scoped to the active account (s.remoteName is kept and
	// re-applied in toConfig).
	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Key("root").Title("Backup source").
				Description("Local folder holding the projects").Value(&s.syncRoot),
			huh.NewInput().Key("dest").Title("Destination folder").
				Description("Folder on the remote where backups are stored").Value(&s.driveDest),
		),
		huh.NewGroup(
			huh.NewInput().Key("ret").Title("Local retention (days)").
				Validate(numeric).Value(&s.retention),
			huh.NewInput().Key("rret").Title("Remote retention (days)").
				Validate(numeric).Value(&s.remoteRetention),
			huh.NewInput().Key("safety").Title("Cleanup safety (days)").
				Validate(numeric).Value(&s.safetyDays),
		),
		huh.NewGroup(
			huh.NewConfirm().Key("delaft").Title("Delete local files after upload?").
				Value(&s.deleteAfter),
			huh.NewConfirm().Key("skipdot").Title("Skip dotfiles?").Value(&s.skipDotfiles),
			huh.NewInput().Key("ignored").Title("Ignored folders (space-separated)").
				Value(&s.ignored),
			huh.NewInput().Key("log").Title("Log file (blank = default)").Value(&s.logFile),
		),
	).WithShowHelp(false)

	return s
}

func (s settingsModel) Init() tea.Cmd { return s.form.Init() }

func (s *settingsModel) setSize(w, h int) {
	s.form = s.form.WithWidth(w).WithHeight(h)
}

// toConfig assembles a config from the form fields. Numeric inputs are already
// validated, so Atoi cannot fail here.
func (s settingsModel) toConfig() config.Config {
	atoi := func(v string) int { n, _ := strconv.Atoi(strings.TrimSpace(v)); return n }
	return config.Config{
		RemoteName:              strings.TrimSpace(s.remoteName),
		SourceRoot:              strings.TrimSpace(s.syncRoot),
		RemoteDestination:       strings.TrimSpace(s.driveDest),
		RetentionDays:           atoi(s.retention),
		RemoteRetentionDays:     atoi(s.remoteRetention),
		RemoteCleanupSafetyDays: atoi(s.safetyDays),
		DeleteAfterUpload:       s.deleteAfter,
		SkipDotfiles:            s.skipDotfiles,
		IgnoredFolders:          strings.Fields(s.ignored),
		LogFile:                 strings.TrimSpace(s.logFile),
	}
}

func (s settingsModel) Update(msg tea.Msg) (settingsModel, tea.Cmd) {
	// Once saved, the screen is a confirmation; any key returns to the menu.
	if s.done {
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "q":
				return s, tea.Quit
			case "enter", "esc", "backspace":
				return s, func() tea.Msg { return goBackMsg{} }
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
		s.done = true
		cfg := s.toConfig()
		return s, func() tea.Msg { return settingsSavedMsg{cfg: cfg} }
	case huh.StateAborted:
		return s, func() tea.Msg { return goBackMsg{} }
	}
	return s, cmd
}

func (s settingsModel) View() string {
	if !s.done {
		return s.form.View()
	}
	if s.saveErr != nil {
		return errorStyle.Render("✗ Could not save settings") + "\n\n" +
			subtitleStyle.Render(s.saveErr.Error())
	}
	cfg := s.toConfig()
	var b strings.Builder
	b.WriteString(titleStyle.Render("✓ Settings saved"))
	b.WriteString("\n\n")
	b.WriteString(infoLine("Remote", cfg.RemoteName) + "\n")
	b.WriteString(infoLine("Backup source", cfg.SourceRoot) + "\n")
	b.WriteString(infoLine("Destination", cfg.RemoteDestination) + "\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf("Retention: %dd local • %dd remote • %dd safety",
		cfg.RetentionDays, cfg.RemoteRetentionDays, cfg.RemoteCleanupSafetyDays)))
	return b.String()
}

// footerHint returns the key hints for the current settings state.
func (s settingsModel) footerHint() string {
	if s.done {
		return "enter/esc back • q quit"
	}
	return "tab/↑↓ navigate • enter next • esc cancel"
}

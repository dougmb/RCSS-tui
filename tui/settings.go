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
		syncRoot:        cfg.SyncRoot,
		driveDest:       cfg.DriveDestination,
		retention:       strconv.Itoa(cfg.RetentionDays),
		remoteRetention: strconv.Itoa(cfg.RemoteRetentionDays),
		safetyDays:      strconv.Itoa(cfg.RemoteCleanupSafetyDays),
		deleteAfter:     cfg.DeleteAfterUpload,
		skipDotfiles:    cfg.SkipDotfiles,
		ignored:         strings.Join(cfg.IgnoredFolders, " "),
		logFile:         cfg.LogFile,
	}

	s.form = huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Key("remote").Title("Remote name").
				Description("rclone remote, e.g. drive:").Value(&s.remoteName),
			huh.NewInput().Key("root").Title("Sync root").
				Description("Local folder holding the projects").Value(&s.syncRoot),
			huh.NewInput().Key("dest").Title("Drive destination").Value(&s.driveDest),
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
		SyncRoot:                strings.TrimSpace(s.syncRoot),
		DriveDestination:        strings.TrimSpace(s.driveDest),
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
	form, cmd := s.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		s.form = f
	}

	switch s.form.State {
	case huh.StateCompleted:
		cfg := s.toConfig()
		return s, func() tea.Msg { return settingsSavedMsg{cfg: cfg} }
	case huh.StateAborted:
		return s, func() tea.Msg { return goBackMsg{} }
	}
	return s, cmd
}

func (s settingsModel) View() string { return s.form.View() }

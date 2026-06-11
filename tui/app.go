// Package tui implements the RCSS terminal UI on top of Bubbletea (the Elm
// architecture: Model/Update/View). app.go holds the root model: it tracks the
// window size, enforces the minimum size guard, routes between screens, and
// handles the global keybindings. Individual screens land in later steps.
package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

// Minimum terminal size. Below this the UI renders only a centered warning,
// per the project's confirmed decision.
const (
	MinWidth  = 80
	MinHeight = 24
)

// screen identifies the active sub-view. The menu is the entry point; the
// other screens are filled in by their respective steps and until then render
// a placeholder.
type screen int

const (
	screenMenu screen = iota
	screenAccount
	screenFolder
	screenBackups
	screenUpload
	screenClean
	screenSettings
	screenSchedule
	screenLogs
)

// screenName is the human label used in placeholder views and titles.
func (s screen) String() string {
	switch s {
	case screenAccount:
		return "Account"
	case screenFolder:
		return "Backup folder"
	case screenBackups:
		return "Backups"
	case screenUpload:
		return "Upload"
	case screenClean:
		return "Clean"
	case screenSettings:
		return "Settings"
	case screenSchedule:
		return "Schedule"
	case screenLogs:
		return "Logs"
	default:
		return "Menu"
	}
}

// Model is the root Bubbletea model.
type Model struct {
	cfg config.Config
	rc  *rclone.Client

	width, height int
	ready         bool

	screen   screen
	menu     list.Model
	account  accountModel
	folder   folderModel
	backups  backupsModel
	upload   uploadModel
	clean    cleanModel
	settings settingsModel
	schedule scheduleModel
	logs     logsModel
	saveErr  error
	quitting bool
}

// folderStart is the directory the picker opens at: the configured BackupRoot
// when set, otherwise the user's home directory.
func (m Model) folderStart() string {
	if m.cfg.BackupRoot != "" {
		return m.cfg.BackupRoot
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}

// New builds the root model with its dependencies. The config and rclone
// client are held for the screens added in later steps.
func New(cfg config.Config, rc *rclone.Client) Model {
	return Model{
		cfg:      cfg,
		rc:       rc,
		screen:   screenMenu,
		menu:     newMenu(),
		account:  newAccountModel(rc, cfg.RemoteName),
		backups:  newBackupsModel(cfg, rc),
		upload:   newUploadModel(cfg, rc),
		clean:    newCleanModel(cfg, rc),
		settings: newSettingsModel(cfg),
		schedule: newScheduleModel(cfg),
		logs:     newLogsModel(cfg),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model: it handles window resizes, screen routing, and
// the global keybindings, delegating the rest to the active screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		// Reserve room for the app frame padding and the footer line.
		m.menu.SetSize(m.width-4, m.height-3)
		m.account.setSize(m.width-4, m.height-3)
		m.backups.setSize(m.width-4, m.height-4)
		m.upload.setHeight(m.height - 4)
		m.clean.setHeight(m.height - 4)
		m.settings.setSize(m.width-4, m.height-3)
		m.schedule.setSize(m.width-4, m.height-3)
		m.logs.setSize(m.width-4, m.height-3)
		// Picker body sits below a 3-line header and above the footer.
		m.folder.setHeight(m.height - 7)
		return m, nil

	case switchScreenMsg:
		m.screen = msg.screen
		switch msg.screen {
		case screenAccount:
			// Re-list remotes and reflect the current selection each entry.
			m.account.current = m.cfg.RemoteName
			return m, m.account.load()
		case screenFolder:
			// Open the picker fresh at the configured root (or home).
			m.folder = newFolderModel(m.folderStart())
			m.folder.setHeight(m.height - 7)
			return m, m.folder.Init()
		case screenBackups:
			// Re-fetch the remote project list on each entry.
			m.backups = newBackupsModel(m.cfg, m.rc)
			m.backups.setSize(m.width-4, m.height-4)
			return m, tea.Batch(m.backups.loadProjects(), m.backups.spinner.Tick)
		case screenUpload:
			// Reset to the idle/confirm state with the latest config.
			m.upload = newUploadModel(m.cfg, m.rc)
			m.upload.setHeight(m.height - 4)
			return m, nil
		case screenClean:
			// Start the dry-run preview immediately.
			m.clean = newCleanModel(m.cfg, m.rc)
			m.clean.setHeight(m.height - 4)
			var cmd tea.Cmd
			m.clean, cmd = m.clean.run(true)
			return m, cmd
		case screenSettings:
			m.settings = newSettingsModel(m.cfg)
			m.settings.setSize(m.width-4, m.height-3)
			return m, m.settings.Init()
		case screenSchedule:
			m.schedule = newScheduleModel(m.cfg)
			m.schedule.setSize(m.width-4, m.height-3)
			return m, m.schedule.Init()
		case screenLogs:
			m.logs = newLogsModel(m.cfg)
			m.logs.setSize(m.width-4, m.height-3)
			m.logs.reload()
			return m, nil
		}
		return m, nil

	case settingsSavedMsg:
		m.cfg = msg.cfg
		m.saveErr = config.Save(m.cfg)
		m.screen = screenMenu
		return m, nil

	case remoteChosenMsg:
		m.cfg.RemoteName = msg.name
		m.saveErr = config.Save(m.cfg)
		m.screen = screenMenu
		return m, nil

	case folderChosenMsg:
		m.cfg.BackupRoot = msg.path
		m.saveErr = config.Save(m.cfg)
		m.screen = screenMenu
		return m, nil

	case goBackMsg:
		m.screen = screenMenu
		return m, nil

	case tea.KeyMsg:
		// Ctrl+C always quits, regardless of the active screen or filtering.
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}

		switch m.screen {
		case screenMenu:
			var cmd tea.Cmd
			m, cmd = m.updateMenu(msg)
			return m, cmd
		case screenAccount:
			var cmd tea.Cmd
			m.account, cmd = m.account.Update(msg)
			return m, cmd
		case screenFolder:
			var cmd tea.Cmd
			m.folder, cmd = m.folder.Update(msg)
			return m, cmd
		case screenBackups:
			var cmd tea.Cmd
			m.backups, cmd = m.backups.Update(msg)
			return m, cmd
		case screenUpload:
			var cmd tea.Cmd
			m.upload, cmd = m.upload.Update(msg)
			return m, cmd
		case screenClean:
			var cmd tea.Cmd
			m.clean, cmd = m.clean.Update(msg)
			return m, cmd
		case screenSettings:
			var cmd tea.Cmd
			m.settings, cmd = m.settings.Update(msg)
			return m, cmd
		case screenSchedule:
			var cmd tea.Cmd
			m.schedule, cmd = m.schedule.Update(msg)
			return m, cmd
		case screenLogs:
			var cmd tea.Cmd
			m.logs, cmd = m.logs.Update(msg)
			return m, cmd
		default:
			// Placeholder screens: esc/backspace return to the menu, q quits.
			switch msg.String() {
			case "esc", "backspace":
				m.screen = screenMenu
				return m, nil
			case "q":
				m.quitting = true
				return m, tea.Quit
			}
		}
	}

	// Non-key messages (e.g. remotesLoadedMsg, filepicker's readDirMsg) route
	// to the active screen.
	switch m.screen {
	case screenAccount:
		var cmd tea.Cmd
		m.account, cmd = m.account.Update(msg)
		return m, cmd
	case screenFolder:
		var cmd tea.Cmd
		m.folder, cmd = m.folder.Update(msg)
		return m, cmd
	case screenBackups:
		var cmd tea.Cmd
		m.backups, cmd = m.backups.Update(msg)
		return m, cmd
	case screenUpload:
		var cmd tea.Cmd
		m.upload, cmd = m.upload.Update(msg)
		return m, cmd
	case screenClean:
		var cmd tea.Cmd
		m.clean, cmd = m.clean.Update(msg)
		return m, cmd
	case screenSettings:
		var cmd tea.Cmd
		m.settings, cmd = m.settings.Update(msg)
		return m, cmd
	case screenSchedule:
		var cmd tea.Cmd
		m.schedule, cmd = m.schedule.Update(msg)
		return m, cmd
	case screenLogs:
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View implements tea.Model: it applies the size guard, then renders the
// active screen.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if !m.ready {
		return "" // wait for the first WindowSizeMsg
	}
	if m.width < MinWidth || m.height < MinHeight {
		return m.viewTooSmall()
	}

	switch m.screen {
	case screenMenu:
		return m.viewMenu()
	case screenAccount:
		return m.viewAccount()
	case screenFolder:
		return m.viewFolder()
	case screenBackups:
		return m.viewBackups()
	case screenUpload:
		body := appStyle.Render(m.upload.View())
		footer := helpStyle.PaddingLeft(2).Render(m.upload.footerHint())
		return m.withFooter(body, footer)
	case screenClean:
		body := appStyle.Render(m.clean.View())
		footer := helpStyle.PaddingLeft(2).Render(m.clean.footerHint())
		return m.withFooter(body, footer)
	case screenSettings:
		body := appStyle.Render(m.settings.View())
		footer := helpStyle.PaddingLeft(2).Render("tab/↑↓ navigate • enter next • esc cancel")
		return m.withFooter(body, footer)
	case screenSchedule:
		body := appStyle.Render(m.schedule.View())
		footer := helpStyle.PaddingLeft(2).Render(m.schedule.footerHint())
		return m.withFooter(body, footer)
	case screenLogs:
		body := appStyle.Render(m.logs.View())
		footer := helpStyle.PaddingLeft(2).Render("↑/↓ scroll • r reload • esc back • q quit")
		return m.withFooter(body, footer)
	default:
		return m.viewPlaceholder(m.screen)
	}
}

// viewBackups frames the backups sub-model with a state-dependent footer.
func (m Model) viewBackups() string {
	body := appStyle.Render(m.backups.View())
	footer := helpStyle.PaddingLeft(2).Render(m.backups.footerHint())
	return m.withFooter(body, footer)
}

// viewFolder frames the directory picker with its footer.
func (m Model) viewFolder() string {
	body := appStyle.Render(m.folder.View())
	footer := helpStyle.PaddingLeft(2).Render("↑/↓ move • →/l open • ←/h up • enter select dir • esc back • q quit")
	return m.withFooter(body, footer)
}

// viewAccount frames the account sub-model with its footer.
func (m Model) viewAccount() string {
	body := appStyle.Render(m.account.View())
	footer := helpStyle.PaddingLeft(2).Render("↑/↓ move • enter select • r refresh • / filter • esc back • q quit")
	return m.withFooter(body, footer)
}

// viewTooSmall centers the fixed warning when the terminal is below 80×24.
func (m Model) viewTooSmall() string {
	box := tooSmallStyle.Render("Not enough space to render panels")
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

// viewMenu renders the bubbles/list main menu with the shared footer.
func (m Model) viewMenu() string {
	body := appStyle.Render(m.menu.View())

	hint := "↑/↓ move • enter select • / filter • q quit"
	if m.saveErr != nil {
		hint = errorStyle.Render("Could not save config: "+m.saveErr.Error()) + "  •  " + hint
	}
	footer := helpStyle.PaddingLeft(2).Render(hint)
	return m.withFooter(body, footer)
}

// viewPlaceholder renders a not-yet-implemented screen so navigation is usable
// before each screen's step lands.
func (m Model) viewPlaceholder(s screen) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(s.String()))
	b.WriteString("\n\n")
	b.WriteString(subtitleStyle.Render("Coming soon."))

	body := appStyle.Render(b.String())
	footer := helpStyle.PaddingLeft(2).Render("esc back • q quit")
	return m.withFooter(body, footer)
}

// withFooter pins the footer to the bottom of the screen without overflowing
// the available height.
func (m Model) withFooter(body, footer string) string {
	gap := m.height - lipgloss.Height(body) - lipgloss.Height(footer)
	if gap < 0 {
		gap = 0
	}
	return body + strings.Repeat("\n", gap) + footer
}

// Run loads the dependencies into the root model and starts the program in the
// alternate screen buffer. It is the entry point used by `rcss` with no
// subcommand.
func Run(cfg config.Config, rc *rclone.Client) error {
	p := tea.NewProgram(New(cfg, rc), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

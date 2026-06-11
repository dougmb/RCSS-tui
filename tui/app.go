// Package tui implements the RCSS terminal UI on top of Bubbletea (the Elm
// architecture: Model/Update/View). app.go holds the root model: a two-pane
// layout — a sidebar menu on the left and a detail pane on the right that
// previews or edits the selected item. It tracks window size, enforces the
// minimum-size guard, manages focus between the panes, and routes input.
package tui

import (
	"os"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dougmb/rcss-tui/config"
	"github.com/dougmb/rcss-tui/rclone"
)

// Minimum terminal size. Below this the UI renders only a centered warning.
const (
	MinWidth  = 80
	MinHeight = 12
)

// sidebarWidth is the inner content width of the menu pane.
const sidebarWidth = 24

// screen identifies the detail-pane sub-view selected from the menu.
type screen int

const (
	screenAccount screen = iota
	screenFolder
	screenBackups
	screenUpload
	screenClean
	screenSettings
	screenSchedule
	screenLogs
)

// focus indicates which pane currently receives input.
type focusArea int

const (
	focusSidebar focusArea = iota
	focusDetail
)

// Model is the root Bubbletea model.
type Model struct {
	cfg config.Config
	rc  *rclone.Client

	width, height int
	detailW       int
	detailH       int
	ready         bool

	focus  focusArea
	screen screen
	locked bool

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

// New builds the root model with its dependencies and all sub-models.
func New(cfg config.Config, rc *rclone.Client) Model {
	return Model{
		cfg:      cfg,
		rc:       rc,
		focus:    focusSidebar,
		screen:   screenAccount,
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

// folderStart is the directory the picker opens at: the configured SyncRoot
// when set, otherwise the user's home directory.
func (m Model) folderStart() string {
	if m.cfg.SyncRoot != "" {
		return m.cfg.SyncRoot
	}
	if home, err := os.UserHomeDir(); err == nil {
		return home
	}
	return "."
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// contentW is the text width inside the detail pane (pane width minus its
// horizontal padding).
func (m Model) contentW() int {
	if w := m.detailW - 2; w > 1 {
		return w
	}
	return 1
}

// resizeDetail propagates the current detail-pane dimensions to every
// sub-model and the sidebar.
func (m *Model) resizeDetail() {
	w, h := m.contentW(), m.detailH
	m.menu.SetSize(sidebarWidth-2, h)
	m.account.setSize(w, h)
	m.backups.setSize(w, h)
	m.upload.setHeight(h)
	m.clean.setHeight(h)
	m.settings.setSize(w, h)
	m.schedule.setSize(w, h)
	m.logs.setSize(w, h)
	// The picker keeps a 3-line header inside the detail pane.
	m.folder.setHeight(h - 4)
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		// Pane borders take 2 cols each, plus a 1-col gap; the footer takes 1
		// row and borders take 2 rows.
		m.detailW = m.width - sidebarWidth - 5
		if m.detailW < 1 {
			m.detailW = 1
		}
		m.detailH = m.height - 3
		if m.detailH < 1 {
			m.detailH = 1
		}
		m.resizeDetail()
		return m, nil

	case settingsSavedMsg:
		m.cfg = msg.cfg
		m.saveErr = config.Save(m.cfg)
		m.focus = focusSidebar
		return m, nil

	case remoteChosenMsg:
		m.cfg.RemoteName = msg.name
		m.saveErr = config.Save(m.cfg)
		m.focus = focusSidebar
		return m, nil

	case folderChosenMsg:
		m.cfg.SyncRoot = msg.path
		m.saveErr = config.Save(m.cfg)
		m.focus = focusSidebar
		return m, nil

	case goBackMsg:
		m.focus = focusSidebar
		m.locked = false
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
		if m.focus == focusSidebar {
			return m.updateSidebar(msg)
		}
		return m.updateDetail(msg)
	}

	// Non-key messages route to the active detail screen.
	return m.updateDetail(msg)
}

// updateSidebar handles input while the menu pane is focused.
func (m Model) updateSidebar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.menu.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.menu, cmd = m.menu.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "enter", "right", "l", "tab":
		return m.activate()
	}
	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(msg)
	return m, cmd
}

// activate opens the highlighted menu item in the detail pane and moves focus
// there.
func (m Model) activate() (tea.Model, tea.Cmd) {
	it, ok := m.menu.SelectedItem().(menuItem)
	if !ok {
		return m, nil
	}
	m.screen = it.target
	m.focus = focusDetail
	if requiresRemote(it.target) && m.cfg.RemoteName == "" {
		m.locked = true
		return m, nil
	}
	m.locked = false
	return m.enterScreen(it.target)
}

// requiresRemote reports whether a screen needs a configured rclone remote.
func requiresRemote(s screen) bool {
	switch s {
	case screenFolder, screenBackups, screenUpload, screenClean, screenSchedule:
		return true
	}
	return false
}

// enterScreen (re)initializes the chosen screen and returns its load/init
// command, so each entry starts fresh against the current config.
func (m Model) enterScreen(s screen) (tea.Model, tea.Cmd) {
	switch s {
	case screenAccount:
		m.account.current = m.cfg.RemoteName
		return m, m.account.load()
	case screenFolder:
		m.folder = newFolderModel(m.folderStart())
		m.folder.setHeight(m.detailH - 4)
		return m, m.folder.Init()
	case screenBackups:
		m.backups = newBackupsModel(m.cfg, m.rc)
		m.backups.setSize(m.contentW(), m.detailH)
		return m, tea.Batch(m.backups.loadProjects(), m.backups.spinner.Tick)
	case screenUpload:
		m.upload = newUploadModel(m.cfg, m.rc)
		m.upload.setHeight(m.detailH)
		return m, nil
	case screenClean:
		m.clean = newCleanModel(m.cfg, m.rc)
		m.clean.setHeight(m.detailH)
		var cmd tea.Cmd
		m.clean, cmd = m.clean.run(true)
		return m, cmd
	case screenSettings:
		m.settings = newSettingsModel(m.cfg)
		m.settings.setSize(m.contentW(), m.detailH)
		return m, m.settings.Init()
	case screenSchedule:
		m.schedule = newScheduleModel(m.cfg)
		m.schedule.setSize(m.contentW(), m.detailH)
		return m, m.schedule.Init()
	case screenLogs:
		m.logs = newLogsModel(m.cfg)
		m.logs.setSize(m.contentW(), m.detailH)
		m.logs.reload()
		return m, nil
	}
	return m, nil
}

// updateDetail routes a message to the active detail sub-model.
func (m Model) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.screen {
	case screenAccount:
		m.account, cmd = m.account.Update(msg)
	case screenFolder:
		m.folder, cmd = m.folder.Update(msg)
	case screenBackups:
		m.backups, cmd = m.backups.Update(msg)
	case screenUpload:
		m.upload, cmd = m.upload.Update(msg)
	case screenClean:
		m.clean, cmd = m.clean.Update(msg)
	case screenSettings:
		m.settings, cmd = m.settings.Update(msg)
	case screenSchedule:
		m.schedule, cmd = m.schedule.Update(msg)
	case screenLogs:
		m.logs, cmd = m.logs.Update(msg)
	}
	return m, cmd
}

// View implements tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if !m.ready {
		return ""
	}
	if m.width < MinWidth || m.height < MinHeight {
		box := tooSmallStyle.Render("Not enough space to render panels")
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
	}

	sidebar := paneStyle(m.focus == focusSidebar).
		Width(sidebarWidth).Height(m.detailH).
		Render(m.menu.View())

	detail := paneStyle(m.focus == focusDetail).
		Width(m.detailW).Height(m.detailH).
		MarginLeft(1).
		Render(m.detailView())

	row := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, detail)
	footer := helpStyle.PaddingLeft(1).Render(m.footerText())
	return row + "\n" + footer
}

// detailView renders the right pane: a preview of the highlighted item when
// the sidebar is focused, otherwise the active sub-model.
func (m Model) detailView() string {
	if m.focus == focusSidebar {
		return m.previewView()
	}
	if m.locked {
		return m.viewLocked()
	}
	switch m.screen {
	case screenAccount:
		return m.account.View()
	case screenFolder:
		return m.folder.View()
	case screenBackups:
		return m.backups.View()
	case screenUpload:
		return m.upload.View()
	case screenClean:
		return m.clean.View()
	case screenSettings:
		return m.settings.View()
	case screenSchedule:
		return m.schedule.View()
	case screenLogs:
		return m.logs.View()
	}
	return ""
}

// viewLocked shows a warning when a screen requires a remote but none is configured.
func (m Model) viewLocked() string {
	it, ok := m.menu.SelectedItem().(menuItem)
	if !ok {
		return ""
	}
	body := titleStyle.Render(it.title) + "\n\n"
	body += warnStyle.Render("No rclone account configured.")
	body += "\n\n"
	body += subtitleStyle.Render("Please configure an account first (Account → select or add a remote).")
	return body
}

// previewView shows the highlighted menu item's description plus relevant
// current config, with a hint to open it.
func (m Model) previewView() string {
	it, ok := m.menu.SelectedItem().(menuItem)
	if !ok {
		return ""
	}
	body := titleStyle.Render(it.title) + "\n\n" + subtitleStyle.Render(it.desc)

	switch it.target {
	case screenAccount:
		body += "\n\n" + infoLine("Current remote", m.cfg.RemoteName)
	case screenFolder:
		body += "\n\n" + infoLine("Sync folder", m.cfg.SyncRoot)
	case screenUpload, screenClean, screenBackups:
		body += "\n\n" + infoLine("Remote", m.cfg.RemoteName) +
			"\n" + infoLine("Destination", m.cfg.DriveDestination)
	}

	if m.saveErr != nil {
		body += "\n\n" + errorStyle.Render("Could not save config: "+m.saveErr.Error())
	}
	body += "\n\n" + helpStyle.Render("Press enter to open →")
	return body
}

// infoLine renders a "label: value" line, showing a dash for empty values.
func infoLine(label, value string) string {
	if value == "" {
		value = "—"
	}
	return subtitleStyle.Render(label+": ") + value
}

// footerText returns the key hints for the focused pane.
func (m Model) footerText() string {
	if m.focus == focusSidebar {
		return "↑/↓ move • enter/→ open • / filter • q quit"
	}
	return m.detailFooter()
}

// detailFooter returns the key hints for the active detail screen.
func (m Model) detailFooter() string {
	if m.locked {
		return "esc back"
	}
	switch m.screen {
	case screenAccount:
		return "↑/↓ move • enter select • r refresh • / filter • esc back"
	case screenFolder:
		return "↑/↓ move • →/l open • ←/h up • enter select dir • esc back"
	case screenBackups:
		return m.backups.footerHint()
	case screenUpload:
		return m.upload.footerHint()
	case screenClean:
		return m.clean.footerHint()
	case screenSettings:
		return "tab/↑↓ navigate • enter next • esc cancel"
	case screenSchedule:
		return m.schedule.footerHint()
	case screenLogs:
		return "↑/↓ scroll • r reload • esc back"
	}
	return "esc back • q quit"
}

// Run loads the dependencies into the root model and starts the program in the
// alternate screen buffer. It is the entry point used by `rcss` with no
// subcommand.
func Run(cfg config.Config, rc *rclone.Client) error {
	p := tea.NewProgram(New(cfg, rc), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

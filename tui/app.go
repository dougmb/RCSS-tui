// Package tui implements the RCSS terminal UI on top of Bubbletea (the Elm
// architecture: Model/Update/View). app.go holds the root model: a two-pane
// layout — a sidebar menu on the left and a detail pane on the right that
// previews or edits the selected item. It tracks window size, enforces the
// minimum-size guard, manages focus between the panes, and routes input.
package tui

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
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
	screenAbout
)

// focus indicates which pane currently receives input.
type focusArea int

const (
	focusSidebar focusArea = iota
	focusDetail
)

// Model is the root Bubbletea model.
type Model struct {
	// store holds every account; cfg is a copy of the active account, kept for
	// the many screens that read a single Config.
	store *config.Store
	cfg   config.Config
	rc    *rclone.Client

	width, height int
	detailW       int
	detailH       int
	ready         bool

	focus  focusArea
	screen screen
	locked bool

	help          help.Model
	showHelp      bool
	rcloneMissing bool

	menu     list.Model
	account  accountModel
	folder   folderModel
	backups  backupsModel
	upload   uploadModel
	clean    cleanModel
	settings settingsModel
	schedule scheduleModel
	logs     logsModel
	about    aboutModel

	saveErr  error
	quitting bool
}

// New builds the root model with its dependencies and all sub-models. rclone's
// absence is recorded (not fatal) so the UI still opens and can warn about the
// missing dependency instead of refusing to start.
func New(store *config.Store, rc *rclone.Client) Model {
	cfg, _ := store.Active()
	rcMissing := rc.EnsureInstalled() != nil
	return Model{
		store:         store,
		cfg:           cfg,
		rc:            rc,
		focus:         focusSidebar,
		screen:        screenAccount,
		help:          help.New(),
		rcloneMissing: rcMissing,
		menu:          newMenu(),
		account:       newAccountModel(rc, cfg.RemoteName, store.Names()),
		backups:       newBackupsModel(cfg, rc),
		upload:        newUploadModel(cfg, rc),
		clean:         newCleanModel(cfg, rc),
		settings:      newSettingsModel(cfg),
		schedule:      newScheduleModel(cfg),
		logs:          newLogsModel(cfg),
		about:         newAboutModel(cfg, rcMissing, len(store.Accounts)),
	}
}

// folderStart is the directory the picker opens at: the configured SourceRoot
// when set, otherwise the user's home directory.
func (m Model) folderStart() string {
	if m.cfg.SourceRoot != "" {
		return m.cfg.SourceRoot
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
	// The sidebar reserves one row for the active-account badge above the menu.
	m.menu.SetSize(sidebarWidth-2, h-1)
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
		m.help.Width = m.width
		m.resizeDetail()
		return m, nil

	case settingsSavedMsg:
		// Persist the edited account, then leave focus on the Settings screen so
		// it can show a visible "Saved ✓" confirmation (the model already moved
		// to its done state); the result is fed back for display.
		m.cfg = msg.cfg
		m.store.Upsert(m.cfg)
		m.saveErr = m.store.Save()
		m.settings.saveErr = m.saveErr
		m.about = newAboutModel(m.cfg, m.rcloneMissing, len(m.store.Accounts))
		return m, nil

	case remoteChosenMsg:
		// Switch the active account, creating its settings with defaults the
		// first time a remote is chosen. Each account is fully isolated.
		if !m.store.Has(msg.name) {
			m.store.Upsert(config.NewAccount(msg.name))
		}
		m.store.SetActive(msg.name)
		m.cfg, _ = m.store.Active()
		m.saveErr = m.store.Save()
		m.about = newAboutModel(m.cfg, m.rcloneMissing, len(m.store.Accounts))
		m.focus = focusSidebar
		return m, nil

	case accountForgetMsg:
		// Forget an account's RCSS settings (the rclone remote is untouched).
		m.store.Remove(msg.name)
		m.cfg, _ = m.store.Active()
		m.saveErr = m.store.Save()
		m.account = newAccountModel(m.rc, m.cfg.RemoteName, m.store.Names())
		m.account.setSize(m.contentW(), m.detailH)
		m.about = newAboutModel(m.cfg, m.rcloneMissing, len(m.store.Accounts))
		return m, m.account.load()

	case folderChosenMsg:
		m.cfg.SourceRoot = msg.path
		m.store.Upsert(m.cfg)
		m.saveErr = m.store.Save()
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
		// The help overlay captures input first: any of ?/esc/q closes it.
		if m.showHelp {
			switch msg.String() {
			case "?", "esc", "q", "enter":
				m.showHelp = false
			}
			return m, nil
		}
		if key.Matches(msg, keys.Help) && m.helpToggleAllowed() {
			m.showHelp = true
			return m, nil
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
	// Number keys 1–9 jump straight to a screen and open it.
	if s := msg.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		if idx := int(s[0] - '1'); idx < len(m.menu.Items()) {
			m.menu.Select(idx)
			return m.activate()
		}
	}
	switch {
	case key.Matches(msg, keys.Quit):
		m.quitting = true
		return m, tea.Quit
	case key.Matches(msg, keys.Open):
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
	if m.locksScreen(it.target) {
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

// needsRclone reports whether a screen drives the rclone binary at all
// (Account lists/configures remotes; the others read or write the cloud).
func needsRclone(s screen) bool {
	return s == screenAccount || requiresRemote(s)
}

// locksScreen reports whether a screen can't be opened in the current state:
// when rclone itself is missing, every rclone-backed screen (Account included)
// is locked; otherwise screens that need a remote stay locked until one is set.
func (m Model) locksScreen(s screen) bool {
	if m.rcloneMissing {
		return needsRclone(s)
	}
	return requiresRemote(s) && m.cfg.RemoteName == ""
}

// helpToggleAllowed reports whether `?` should open the help overlay rather
// than be treated as input. It is suppressed where `?` is a typeable character
// (the huh forms) or while a list is capturing filter text.
func (m Model) helpToggleAllowed() bool {
	if m.focus == focusSidebar {
		return m.menu.FilterState() != list.Filtering
	}
	switch m.screen {
	case screenSettings:
		return m.settings.done // a huh form while editing; free once saved
	case screenSchedule:
		return m.schedule.state == scDone
	case screenAccount:
		return m.account.list.FilterState() != list.Filtering
	case screenBackups:
		return !m.backups.filtering()
	}
	return true
}

// enterScreen (re)initializes the chosen screen and returns its load/init
// command, so each entry starts fresh against the current config.
func (m Model) enterScreen(s screen) (tea.Model, tea.Cmd) {
	switch s {
	case screenAccount:
		m.account = newAccountModel(m.rc, m.cfg.RemoteName, m.store.Names())
		m.account.setSize(m.contentW(), m.detailH)
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
		// Clean opens on an intro/options screen explaining what it deletes and
		// exposing the Force toggle; the dry-run is launched from there.
		m.clean = newCleanModel(m.cfg, m.rc)
		m.clean.setHeight(m.detailH)
		return m, nil
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
	case screenAbout:
		m.about = newAboutModel(m.cfg, m.rcloneMissing, len(m.store.Accounts))
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
	case screenAbout:
		m.about, cmd = m.about.Update(msg)
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
		Render(m.accountBadge() + "\n" + m.menu.View())

	detail := paneStyle(m.focus == focusDetail).
		Width(m.detailW).Height(m.detailH).
		MarginLeft(1).
		Render(m.detailView())

	row := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, detail)
	return row + "\n" + m.footer()
}

// footer renders the bottom hint line: a keymap-driven short help while the
// menu is focused, the close hint while the help overlay is open, or the active
// screen's contextual hints in the detail pane.
func (m Model) footer() string {
	pad := lipgloss.NewStyle().PaddingLeft(1)
	switch {
	case m.showHelp:
		return helpStyle.PaddingLeft(1).Render("? / esc close help")
	case m.focus == focusSidebar:
		return pad.Render(m.help.ShortHelpView(sidebarShortHelp()))
	default:
		return helpStyle.PaddingLeft(1).Render(m.detailFooter())
	}
}

// detailView renders the right pane: a preview of the highlighted item when
// the sidebar is focused, otherwise the active sub-model.
func (m Model) detailView() string {
	if m.showHelp {
		return m.helpBody()
	}
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
	case screenAbout:
		return m.about.View()
	}
	return ""
}

// helpBody renders the full keybinding reference shown by the `?` overlay,
// generated from the central keymap so it never drifts from the real bindings.
func (m Model) helpBody() string {
	return titleStyle.Render("Keyboard shortcuts") + "\n\n" + m.help.FullHelpView(fullHelp())
}

// viewLocked shows a warning when a screen can't be opened: rclone is not
// installed, or no remote is configured yet.
func (m Model) viewLocked() string {
	it, ok := m.menu.SelectedItem().(menuItem)
	if !ok {
		return ""
	}
	body := titleStyle.Render(it.title) + "\n\n"
	if m.rcloneMissing {
		body += warnStyle.Render("rclone is not installed.")
		body += "\n\n"
		body += subtitleStyle.Render("RCSS drives the rclone binary, which was not found on your PATH.\n" +
			"Install it from https://rclone.org/install/ and restart RCSS.")
		return body
	}
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

	if m.rcloneMissing {
		body += "\n\n" + warnStyle.Render("⚠ rclone not found on PATH — install it from") +
			"\n" + warnStyle.Render("  https://rclone.org/install/ to enable cloud features.")
	}

	switch it.target {
	case screenAccount:
		body += "\n\n" + infoLine("Active account", m.cfg.RemoteName) +
			"\n" + infoLine("Accounts configured", fmt.Sprintf("%d", len(m.store.Accounts)))
	case screenFolder:
		body += "\n\n" + infoLine("Backup source", m.cfg.SourceRoot)
	case screenUpload, screenClean, screenBackups:
		body += "\n\n" + infoLine("Remote", m.cfg.RemoteName) +
			"\n" + infoLine("Destination", m.cfg.RemoteDestination)
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

// accountBadge renders the active-account indicator shown atop the sidebar,
// truncated to the sidebar width.
func (m Model) accountBadge() string {
	if m.cfg.RemoteName == "" {
		return warnStyle.Render("▸ no account")
	}
	label := "▸ " + m.cfg.RemoteName
	if r := []rune(label); len(r) > sidebarWidth-2 {
		label = string(r[:sidebarWidth-3]) + "…"
	}
	return titleStyle.Render(label)
}

// detailFooter returns the key hints for the active detail screen, appending a
// "? help" hint on screens where the overlay is available.
func (m Model) detailFooter() string {
	if m.locked {
		return "esc back"
	}
	var hint string
	switch m.screen {
	case screenAccount:
		hint = "↑/↓ move • enter switch • d forget • r refresh • / filter • esc back"
	case screenFolder:
		hint = "↑/↓ move • →/l open • ←/h up • enter select dir • esc back"
	case screenBackups:
		hint = m.backups.footerHint()
	case screenUpload:
		hint = m.upload.footerHint()
	case screenClean:
		hint = m.clean.footerHint()
	case screenSettings:
		hint = m.settings.footerHint()
	case screenSchedule:
		hint = m.schedule.footerHint()
	case screenLogs:
		hint = "↑/↓ scroll • r reload • esc back"
	case screenAbout:
		hint = "esc back • q quit"
	default:
		hint = "esc back • q quit"
	}
	if m.helpToggleAllowed() {
		hint += " • ? help"
	}
	return hint
}

// Run loads the dependencies into the root model and starts the program in the
// alternate screen buffer. It is the entry point used by `rcss` with no
// subcommand.
func Run(store *config.Store, rc *rclone.Client) error {
	p := tea.NewProgram(New(store, rc), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

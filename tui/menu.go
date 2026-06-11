package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// menuItem is a main-menu row. Selecting it routes to its target screen.
type menuItem struct {
	title  string
	desc   string
	target screen
}

// Title, Description and FilterValue satisfy list.Item / DefaultItem so the
// bubbles/list default delegate can render and filter the menu.
func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.desc }
func (i menuItem) FilterValue() string { return i.title }

// newMenu builds the main-menu list. Each entry maps to the screen its step
// implements; until then the target renders a "coming soon" placeholder.
func newMenu() list.Model {
	items := []list.Item{
		menuItem{"Account", "Choose the rclone remote / configure a new one", screenAccount},
		menuItem{"Backup folder", "Pick the local folder to back up", screenFolder},
		menuItem{"Backups", "Browse remote backups and restore", screenBackups},
		menuItem{"Upload", "Run a backup now", screenUpload},
		menuItem{"Clean", "Remove old remote backups", screenClean},
		menuItem{"Settings", "Edit retention and behavior", screenSettings},
		menuItem{"Schedule", "Manage cron jobs", screenSchedule},
		menuItem{"Logs", "View the sync log", screenLogs},
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "RCSS — Rclone Cloud Simple Scripts"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false) // the root model renders its own footer
	l.Styles.Title = titleStyle
	return l
}

// switchScreenMsg requests a screen change. updateMenu emits it on selection.
type switchScreenMsg struct{ screen screen }

// switchTo returns a command that routes to the given screen.
func switchTo(s screen) tea.Cmd {
	return func() tea.Msg { return switchScreenMsg{screen: s} }
}

// updateMenu handles key input while the menu is active. While the list is
// filtering it forwards every key to the list (so typing, esc and enter act on
// the filter). Otherwise enter routes to the selected screen, q quits, and the
// rest drives list navigation.
func (m Model) updateMenu(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.menu.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.menu, cmd = m.menu.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "enter":
		if it, ok := m.menu.SelectedItem().(menuItem); ok {
			return m, switchTo(it.target)
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.menu, cmd = m.menu.Update(msg)
	return m, cmd
}

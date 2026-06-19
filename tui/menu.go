package tui

import (
	"github.com/charmbracelet/bubbles/list"
)

// menuItem is a sidebar row. Selecting it opens its target screen in the detail
// pane.
type menuItem struct {
	title  string
	desc   string
	target screen
}

// Title, Description and FilterValue satisfy list.Item / DefaultItem.
func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.desc }
func (i menuItem) FilterValue() string { return i.title }

// newMenu builds the sidebar menu list. The delegate is compact (titles only)
// so the narrow sidebar fits all entries; the description is shown in the
// detail pane preview instead.
func newMenu() list.Model {
	items := []list.Item{
		menuItem{"Rclone Account", "Select or configure an rclone remote (e.g. Google Drive).", screenAccount},
		menuItem{"Backup source", "Choose the local directory whose sub-folders are backed up to the cloud.", screenFolder},
		menuItem{"Back Up Now", "Copy all projects to the cloud now (one-way upload), with live progress.", screenUpload},
		menuItem{"Restore", "Browse cloud backups by project and restore individual files.", screenBackups},
		menuItem{"Clean", "Remove old CLOUD backups beyond retention, with a dry-run preview and safety lock.", screenClean},
		menuItem{"Settings", "Configure retention days, dotfiles, ignored folders and behavior.", screenSettings},
		menuItem{"Schedule", "Set up automatic backup and clean schedules via your OS scheduler (crontab / Task Scheduler).", screenSchedule},
		menuItem{"Logs", "View the sync log with colorized ERROR and WARN entries.", screenLogs},
		menuItem{"About", "About RCSS: version, dependency status, and config locations.", screenAbout},
	}

	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetSpacing(0)

	l := list.New(items, d, 0, 0)
	l.Title = "RCSS"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetShowPagination(false)
	l.Styles.Title = titleStyle
	return l
}

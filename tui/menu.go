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
		menuItem{"Account", "Choose the rclone remote, or configure a new one with `rclone config`.", screenAccount},
		menuItem{"Backup folder", "Pick the local folder whose sub-folders and files are backed up.", screenFolder},
		menuItem{"Backups", "Browse remote folders and files and restore one.", screenBackups},
		menuItem{"Upload", "Run a backup now, streaming rclone progress.", screenUpload},
		menuItem{"Clean", "Preview (dry-run) then delete old remote backups.", screenClean},
		menuItem{"Settings", "Edit retention and behavior, saved to config.toml.", screenSettings},
		menuItem{"Schedule", "Install daily-upload / weekly-clean jobs into your crontab.", screenSchedule},
		menuItem{"Logs", "Scroll the sync log with ERROR/WARN highlighting.", screenLogs},
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

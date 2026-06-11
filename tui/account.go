package tui

import (
	"context"
	"os/exec"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmb/rcss-tui/rclone"
)

// Account screen: pick the rclone remote backups are stored under, or shell out
// to `rclone config` to set up a new one (re-listing on return). Ports the
// account-selection intent of the scripts (which only read RCLONE_REMOTE from
// backup.env) into an interactive picker.

// remoteItem is a selectable rclone remote.
type remoteItem struct {
	name    string
	current bool
}

func (i remoteItem) Title() string {
	if i.current {
		return i.name + "  (current)"
	}
	return i.name
}
func (i remoteItem) Description() string { return "rclone remote" }
func (i remoteItem) FilterValue() string { return i.name }

// configItem is the special row that launches `rclone config`.
type configItem struct{}

func (configItem) Title() string       { return "＋ Configure a new account…" }
func (configItem) Description() string { return "Opens `rclone config`" }
func (configItem) FilterValue() string { return "configure new account" }

// --- messages ---

// remotesLoadedMsg carries the result of rclone.ListRemotes.
type remotesLoadedMsg struct {
	remotes []string
	err     error
}

// reloadRemotesMsg asks the account screen to re-list remotes (e.g. after
// returning from `rclone config`).
type reloadRemotesMsg struct{}

// remoteChosenMsg tells the root model the user picked a remote.
type remoteChosenMsg struct{ name string }

// goBackMsg returns to the main menu.
type goBackMsg struct{}

// accountModel is the account screen's sub-model.
type accountModel struct {
	rc      *rclone.Client
	current string
	list    list.Model
	loading bool
	err     error
}

func newAccountModel(rc *rclone.Client, current string) accountModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Select rclone remote"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = titleStyle
	return accountModel{rc: rc, current: current, list: l, loading: true}
}

func (a *accountModel) setSize(w, h int) { a.list.SetSize(w, h) }

// load returns a command that fetches the configured remotes.
func (a accountModel) load() tea.Cmd {
	rc := a.rc
	return func() tea.Msg {
		remotes, err := rc.ListRemotes(context.Background())
		return remotesLoadedMsg{remotes: remotes, err: err}
	}
}

// Update handles the account screen's messages and keys.
func (a accountModel) Update(msg tea.Msg) (accountModel, tea.Cmd) {
	switch msg := msg.(type) {
	case remotesLoadedMsg:
		a.loading = false
		a.err = msg.err
		items := make([]list.Item, 0, len(msg.remotes)+1)
		for _, r := range msg.remotes {
			items = append(items, remoteItem{name: r, current: r == a.current})
		}
		items = append(items, configItem{})
		a.list.SetItems(items)
		return a, nil

	case reloadRemotesMsg:
		a.loading = true
		a.err = nil
		return a, a.load()

	case tea.KeyMsg:
		// While filtering, let the list consume keys.
		if a.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			a.list, cmd = a.list.Update(msg)
			return a, cmd
		}
		switch msg.String() {
		case "esc", "backspace":
			return a, func() tea.Msg { return goBackMsg{} }
		case "q":
			return a, tea.Quit
		case "r":
			a.loading = true
			a.err = nil
			return a, a.load()
		case "enter":
			switch it := a.list.SelectedItem().(type) {
			case remoteItem:
				name := it.name
				return a, func() tea.Msg { return remoteChosenMsg{name: name} }
			case configItem:
				cmd := exec.Command("rclone", "config")
				return a, tea.ExecProcess(cmd, func(error) tea.Msg {
					return reloadRemotesMsg{}
				})
			}
			return a, nil
		}
	}

	var cmd tea.Cmd
	a.list, cmd = a.list.Update(msg)
	return a, cmd
}

// View renders the inner body of the account screen (the root frames it and
// adds the footer).
func (a accountModel) View() string {
	if a.loading {
		return subtitleStyle.Render("Loading remotes…")
	}
	if a.err != nil {
		return errorStyle.Render("Failed to list remotes: " + a.err.Error())
	}
	return a.list.View()
}

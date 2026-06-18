package tui

import (
	"context"
	"os/exec"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/dougmb/rcss-tui/rclone"
)

// Account screen: the multi-account manager. It lists the rclone remotes and
// marks which one is the active RCSS account and which already have isolated
// RCSS settings. Selecting a remote switches the active account (creating its
// settings on first use); `d` forgets an account's settings; "Configure a new
// account…" shells out to `rclone config`. Each account (one per rclone remote)
// keeps its own folders, retention, log, and schedule.

// remoteItem is a selectable rclone remote.
type remoteItem struct {
	name       string
	current    bool // the active account
	configured bool // has stored RCSS settings
}

func (i remoteItem) Title() string {
	switch {
	case i.current:
		return i.name + "  ● active"
	case i.configured:
		return i.name + "  · configured"
	default:
		return i.name
	}
}
func (i remoteItem) Description() string {
	if i.configured {
		return "rclone remote · RCSS account"
	}
	return "rclone remote"
}
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

// remoteChosenMsg tells the root model to make a remote the active account.
type remoteChosenMsg struct{ name string }

// accountForgetMsg asks the root to drop an account's RCSS settings (the rclone
// remote is left intact).
type accountForgetMsg struct{ name string }

// goBackMsg returns to the main menu.
type goBackMsg struct{}

// accountModel is the account screen's sub-model.
type accountModel struct {
	rc         *rclone.Client
	current    string          // active account name
	configured map[string]bool // remotes with stored RCSS settings
	list       list.Model
	loading    bool
	err        error
}

func newAccountModel(rc *rclone.Client, current string, accounts []string) accountModel {
	l := list.New(nil, list.NewDefaultDelegate(), 0, 0)
	l.Title = "Accounts (rclone remotes)"
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.Styles.Title = titleStyle

	set := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		set[a] = true
	}
	return accountModel{rc: rc, current: current, configured: set, list: l, loading: true}
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
			items = append(items, remoteItem{name: r, current: r == a.current, configured: a.configured[r]})
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
		case "d":
			if it, ok := a.list.SelectedItem().(remoteItem); ok && it.configured {
				name := it.name
				return a, func() tea.Msg { return accountForgetMsg{name: name} }
			}
			return a, nil
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

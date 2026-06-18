package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap centralizes RCSS keybindings so the footer hints and the `?` help
// overlay are generated from a single source of truth — the idiomatic
// bubbles/key + bubbles/help pattern. Defining bindings here (instead of
// scattering string comparisons across screens) keeps shortcuts consistent and
// self-documenting.
type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Open    key.Binding
	Back    key.Binding
	Filter  key.Binding
	Jump    key.Binding // 1–8 quick-jump to a screen (documented; matched separately)
	Refresh key.Binding
	Help    key.Binding
	Quit    key.Binding
}

// keys is the single shared keymap instance.
var keys = keyMap{
	Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
	Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
	Open:    key.NewBinding(key.WithKeys("enter", "right", "l", "tab"), key.WithHelp("enter/→", "open")),
	Back:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
	Jump:    key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"), key.WithHelp("1–9", "jump to screen")),
	Refresh: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "refresh")),
	Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

// Screen-specific actions, documented in the full-help overlay so every
// keypress the detail screens accept is discoverable from one place.
var (
	actStart   = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "start / select"))
	actExecute = key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "execute deletion (Clean)"))
	actReload  = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "reload (Logs/Account)"))
)

// sidebarShortHelp is the compact footer help shown while the menu is focused.
func sidebarShortHelp() []key.Binding {
	return []key.Binding{keys.Up, keys.Down, keys.Open, keys.Jump, keys.Filter, keys.Help, keys.Quit}
}

// fullHelp is the multi-column layout shown in the `?` overlay: global
// navigation, screen jumps, and the per-screen actions.
func fullHelp() [][]key.Binding {
	return [][]key.Binding{
		{keys.Up, keys.Down, keys.Open, keys.Back},
		{keys.Jump, keys.Filter, keys.Refresh},
		{actStart, actExecute, actReload},
		{keys.Help, keys.Quit},
	}
}

package tui

import "github.com/charmbracelet/lipgloss"

// Palette — a small, terminal-256 friendly theme. Kept in one place so every
// screen shares the same look.
var (
	colorAccent  = lipgloss.Color("39")  // cyan/blue — titles, selection
	colorPrimary = lipgloss.Color("12")  // bright blue
	colorMuted   = lipgloss.Color("241") // grey — help, subtitles
	colorWarn    = lipgloss.Color("214") // amber — warnings
	colorError   = lipgloss.Color("196") // red — errors
)

var (
	// appStyle frames the whole UI with a little breathing room.
	appStyle = lipgloss.NewStyle().Padding(1, 2)

	// titleStyle renders the app/screen title.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	// subtitleStyle renders secondary descriptive text under a title.
	subtitleStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// helpStyle renders the footer key hints.
	helpStyle = lipgloss.NewStyle().Foreground(colorMuted)

	// errorStyle renders inline error messages and ERROR log lines.
	errorStyle = lipgloss.NewStyle().Foreground(colorError)

	// warnStyle highlights WARN log lines.
	warnStyle = lipgloss.NewStyle().Foreground(colorWarn)

	// tooSmallStyle is the centered warning shown below the minimum size.
	tooSmallStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWarn).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorWarn).
			Padding(1, 3)

	// menuItemStyle / menuSelectedStyle render the placeholder menu rows until
	// the real bubbles/list menu lands in the next step.
	menuItemStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(colorMuted)

	menuSelectedStyle = lipgloss.NewStyle().
				PaddingLeft(1).
				Foreground(colorPrimary).
				Bold(true)
)

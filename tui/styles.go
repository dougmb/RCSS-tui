package tui

import "github.com/charmbracelet/lipgloss"

// paneStyle returns the bordered box style for a pane; the active pane gets an
// accent border, the inactive one a muted border.
func paneStyle(active bool) lipgloss.Style {
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	if active {
		return s.BorderForeground(colorAccent)
	}
	return s.BorderForeground(colorMuted)
}

// Palette — a small, terminal-256 friendly theme. Kept in one place so every
// screen shares the same look.
var (
	colorAccent = lipgloss.Color("39")  // cyan/blue — titles, selection
	colorMuted  = lipgloss.Color("241") // grey — help, subtitles
	colorWarn   = lipgloss.Color("214") // amber — warnings
	colorError  = lipgloss.Color("196") // red — errors
)

var (
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
)

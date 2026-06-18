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
// screen shares the same look. Colors are adaptive: the Dark variants preserve
// the original look on dark terminals, while the Light variants keep text and
// accents legible on light backgrounds.
var (
	colorAccent = lipgloss.AdaptiveColor{Light: "25", Dark: "39"}   // blue/cyan — titles, selection
	colorMuted  = lipgloss.AdaptiveColor{Light: "240", Dark: "241"} // grey — help, subtitles
	colorWarn   = lipgloss.AdaptiveColor{Light: "130", Dark: "214"} // amber — warnings
	colorError  = lipgloss.AdaptiveColor{Light: "124", Dark: "196"} // red — errors
	colorOK     = lipgloss.AdaptiveColor{Light: "28", Dark: "42"}   // green — success
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

	// okStyle highlights success states (e.g. a successful last backup).
	okStyle = lipgloss.NewStyle().Foreground(colorOK)

	// tooSmallStyle is the centered warning shown below the minimum size.
	tooSmallStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorWarn).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorWarn).
			Padding(1, 3)
)

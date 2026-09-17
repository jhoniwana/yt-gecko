package tui

import "github.com/charmbracelet/lipgloss"

// Styles groups the lip gloss styling used across the TUI.
type Styles struct {
	App     lipgloss.Style
	Box     lipgloss.Style
	Title   lipgloss.Style
	Accent  lipgloss.Style
	Header  lipgloss.Style
	Active  lipgloss.Style
	Dim     lipgloss.Style
	Error   lipgloss.Style
	Hint    lipgloss.Style
	Success lipgloss.Style
	Warn    lipgloss.Style
	Bar     lipgloss.Style
	Help    lipgloss.Style
	Muted   lipgloss.Style
	Hover   lipgloss.Style
	Sel     lipgloss.Style
}

func defaultStyles() Styles {
	// YouTube's palette: red on black with white/gray text. Red 196 is the
	// classic YouTube red; 88/52 are its dark shades for borders and
	// selection so the thumbnails keep reading well.
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("88")).
		Padding(0, 2)
	return Styles{
		App:     lipgloss.NewStyle().Padding(0, 2),
		Box:     box,
		Title:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")),
		Accent:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")),
		Header:  lipgloss.NewStyle().Foreground(lipgloss.Color("255")),
		Active:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")),
		Dim:     lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		Hint:    lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Success: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("196")),
		Warn:    lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		Bar:     lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		Help:    lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		Muted:   lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		// Hover lights up clickable things under the mouse; Sel marks the
		// keyboard-selected card. Both are dark red so the UI stays YouTube.
		Hover: lipgloss.NewStyle().Background(lipgloss.Color("52")).Foreground(lipgloss.Color("255")),
		Sel:   lipgloss.NewStyle().Background(lipgloss.Color("88")).Foreground(lipgloss.Color("255")),
	}
}

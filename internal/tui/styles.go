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
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("237")).
		Padding(0, 2)
	return Styles{
		App:     lipgloss.NewStyle().Padding(0, 2),
		Box:     box,
		Title:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("119")),
		Accent:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")),
		Header:  lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		Active:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("220")),
		Dim:     lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Error:   lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		Hint:    lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		Success: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("84")),
		Warn:    lipgloss.NewStyle().Foreground(lipgloss.Color("208")),
		Bar:     lipgloss.NewStyle().Foreground(lipgloss.Color("208")),
		Help:    lipgloss.NewStyle().Foreground(lipgloss.Color("250")),
		Muted:   lipgloss.NewStyle().Foreground(lipgloss.Color("238")),
		// Hover lights up clickable things under the mouse; Sel marks the
		// keyboard-selected card. Both are subtle dark backgrounds so the
		// thumbnails still read well.
		Hover: lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("255")),
		Sel:   lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("255")),
	}
}

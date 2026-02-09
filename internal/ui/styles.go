package ui

import "github.com/charmbracelet/lipgloss"

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("12"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("8"))

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")).
			Bold(true)

	suggestionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("7"))

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	historyOverlayStyle = lipgloss.NewStyle().
				Border(lipgloss.DoubleBorder()).
				BorderForeground(lipgloss.Color("12")).
				Padding(1, 2)
)

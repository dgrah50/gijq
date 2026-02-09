package jq

// Colorize adds syntax highlighting to JSON
// Note: Currently disabled because lipgloss viewport strips ANSI codes
func Colorize(s string) string {
	// TODO: Implement colorization within the View() function using lipgloss
	// instead of pre-colorizing, to avoid ANSI code stripping
	return s
}

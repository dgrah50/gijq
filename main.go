package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/dayangraham/gijq/internal/autocomplete"
	"github.com/dayangraham/gijq/internal/clipboard"
	"github.com/dayangraham/gijq/internal/history"
	"github.com/dayangraham/gijq/internal/jq"
	"github.com/dayangraham/gijq/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	if wantsHelp(os.Args[1:]) {
		printHelp(os.Stdout)
		return nil
	}

	// Determine input source
	jsonData, filename, filepath, err := loadInput()
	if err != nil {
		return err
	}

	// Create services
	jqSvc, err := jq.NewService(jsonData)
	if err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	acSvc := autocomplete.NewService(jqSvc)

	histPath := getHistoryPath()
	hist, err := history.NewStore(histPath)
	if err != nil {
		return fmt.Errorf("failed to load history: %w", err)
	}

	clip := clipboard.NewService()

	// Create and run TUI
	model := ui.NewModel(jqSvc, acSvc, hist, clip, ui.Config{
		Filename: filename,
		Filepath: filepath,
	})

	p := tea.NewProgram(model, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func loadInput() ([]byte, string, string, error) {
	// Check for piped input
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		// Reading from pipe
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to read stdin: %w", err)
		}
		return data, "<stdin>", "<stdin>", nil
	}

	// Read from file argument
	if len(os.Args) < 2 {
		return nil, "", "", errors.New(usageText())
	}

	path := os.Args[1]
	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read %s: %w", path, err)
	}

	return data, filepath.Base(path), absPath, nil
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func printHelp(w io.Writer) {
	_, _ = io.WriteString(w, usageText()+"\n")
}

func usageText() string {
	lines := []string{
		"usage: gijq <file.json>",
		"       cat file.json | gijq",
		"",
		"options:",
		"  -h, --help   show help",
	}
	return strings.Join(lines, "\n")
}

func getHistoryPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = os.Getenv("HOME")
	}
	return filepath.Join(configDir, "gijq", "history.json")
}

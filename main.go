package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

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

	stdinIsTTY := term.IsTerminal(os.Stdin.Fd())
	jsonData, filename, filepath, err := loadInput(os.Args[1:], os.Stdin, stdinIsTTY)
	if err != nil {
		return err
	}

	jqSvc, err := jq.NewService(jsonData)
	if err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}
	acSvc := autocomplete.NewService(jqSvc)

	// History is a convenience: problems with it never stop the program.
	hist, err := history.NewStore(getHistoryPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning:", err)
	}

	clip := clipboard.NewService()
	telemetryEnabled := envEnabled("GIJQ_TELEMETRY")

	// Create and run TUI
	model := ui.NewModel(jqSvc, acSvc, hist, clip, ui.Config{
		Filename:  filename,
		Filepath:  filepath,
		Telemetry: telemetryEnabled,
	})

	// The TUI must never write to a piped stdout (the result goes there) and
	// must read keys from the terminal when stdin carries the JSON. Alt screen
	// is declared on the model's View.
	opts, closeTTY, err := ttyOptions(stdinIsTTY)
	if err != nil {
		return err
	}
	defer closeTTY()

	p := tea.NewProgram(model, opts...)
	finalModel, err := p.Run()
	final, isModel := finalModel.(ui.Model)
	if telemetryEnabled && isModel {
		if summary, ok := final.TelemetrySummary(); ok {
			fmt.Fprintln(os.Stderr, summary)
		}
	}
	if err != nil {
		return err
	}
	// Print only after Run returns so the alt screen is already torn down.
	if out, ok := final.Output(); isModel && ok {
		if _, err := out.WriteTo(os.Stdout); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
	}
	return nil
}

// ttyOptions redirects Bubble Tea's I/O to the controlling terminal when
// stdin or stdout is not a terminal.
func ttyOptions(inTTY bool) ([]tea.ProgramOption, func(), error) {
	outTTY := term.IsTerminal(os.Stdout.Fd())
	if inTTY && outTTY {
		return nil, func() {}, nil
	}
	ttyIn, ttyOut, err := tea.OpenTTY()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open terminal: %w", err)
	}
	closer := func() {
		_ = ttyIn.Close()
		_ = ttyOut.Close()
	}
	var opts []tea.ProgramOption
	if !inTTY {
		opts = append(opts, tea.WithInput(ttyIn))
	}
	if !outTTY {
		opts = append(opts, tea.WithOutput(ttyOut))
	}
	return opts, closer, nil
}

// loadInput reads the JSON to explore. A file argument always wins; without
// one, piped stdin is used. It returns the data, a display name and the path
// used to key history.
func loadInput(args []string, stdin io.Reader, stdinIsTTY bool) ([]byte, string, string, error) {
	if len(args) > 0 {
		path := args[0]
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, "", "", fmt.Errorf("failed to read %s: %w", path, err)
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			absPath = path
		}
		return data, filepath.Base(path), absPath, nil
	}

	if stdinIsTTY {
		return nil, "", "", errors.New(usageText())
	}
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to read stdin: %w", err)
	}
	return data, "<stdin>", "<stdin>", nil
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

func envEnabled(name string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

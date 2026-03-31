package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/wuqiang1985/go-route-impact/internal/analyzer"
)

var (
	projectDir string
	configFile string
)

var rootCmd = &cobra.Command{
	Use:           "go-route-impact",
	Short:         "Function-level route impact analysis for Go projects",
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `go-route-impact uses function-level call graph analysis to determine
which HTTP routes are affected by code changes.`,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&projectDir, "project", "p", ".", "Project root directory")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Config file path (default: .route-impact.yaml)")
	rootCmd.CompletionOptions.HiddenDefaultCmd = true

	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(routesCmd)
	rootCmd.AddCommand(gitCmd)
	rootCmd.AddCommand(hookCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(graphCmd)
}

var (
	errColor  = color.New(color.FgRed, color.Bold)
	hintColor = color.New(color.FgYellow)
	cmdColor  = color.New(color.FgCyan)
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		var ue *analyzer.UserError
		if errors.As(err, &ue) {
			printUserError(ue)
		} else {
			errColor.Fprint(os.Stderr, "Error: ")
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}

func printUserError(ue *analyzer.UserError) {
	fmt.Fprintln(os.Stderr)
	errColor.Fprintf(os.Stderr, "  ✘ %s\n", ue.Message)
	fmt.Fprintln(os.Stderr)
	hintColor.Fprintln(os.Stderr, "  Hint:")
	for _, line := range splitLines(ue.Hint) {
		if len(line) > 0 && line[0] == ' ' {
			// Indented lines are commands, highlight them
			fmt.Fprint(os.Stderr, "    ")
			cmdColor.Fprintln(os.Stderr, line)
		} else {
			fmt.Fprintf(os.Stderr, "    %s\n", line)
		}
	}
	fmt.Fprintln(os.Stderr)
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

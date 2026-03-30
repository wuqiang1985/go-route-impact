package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pnt-team/go-route-impact-v2/internal/analyzer"
	"github.com/pnt-team/go-route-impact-v2/internal/config"
	"github.com/pnt-team/go-route-impact-v2/internal/gitutil"
	"github.com/pnt-team/go-route-impact-v2/internal/output"
)

var (
	gitStaged      bool
	gitUncommitted bool
	gitBranch      string
	gitFormat      string
)

var gitCmd = &cobra.Command{
	Use:   "git",
	Short: "Analyze function-level route impact for git changes",
	Long:  `Analyzes staged, uncommitted, or branch changes at function level and shows affected routes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// Determine git diff mode
		var mode string
		switch {
		case gitStaged:
			mode = "staged"
		case gitUncommitted:
			mode = "uncommitted"
		case gitBranch != "":
			mode = gitBranch
		default:
			return fmt.Errorf("specify one of: --staged, --uncommitted, or --branch")
		}

		// Get changed lines from git diff
		changedLines, err := gitutil.ChangedFilesWithLines(projectDir, mode)
		if err != nil {
			return fmt.Errorf("get git changes: %w", err)
		}

		if len(changedLines) == 0 {
			fmt.Fprintln(os.Stdout, "No .go files changed.")
			return nil
		}

		// Initialize analyzer
		a, err := analyzer.New(projectDir, cfg)
		if err != nil {
			return fmt.Errorf("initialize analyzer: %w", err)
		}

		// Map changed lines to functions
		changedFuncs := gitutil.ChangedFuncs(changedLines, a.GetParsedFiles(), a.Resolver)

		if len(changedFuncs) == 0 {
			fmt.Fprintln(os.Stdout, "No functions changed (changes may be outside function bodies).")
			return nil
		}

		// Analyze impact
		result := a.ImpactByChangedFuncs(changedFuncs)

		// Print stats
		funcs, edges := a.GraphStats()
		fmt.Fprintf(os.Stderr, "Call graph: %d functions, %d edges\n", funcs, edges)
		fmt.Fprintf(os.Stderr, "Routes: %d total\n\n", len(a.AllRoutes()))

		// Output
		switch gitFormat {
		case "json":
			return output.PrintImpactJSON(os.Stdout, result)
		case "md", "markdown":
			output.PrintImpactMarkdown(os.Stdout, result)
		default:
			output.PrintImpactResult(os.Stdout, result)
		}

		return nil
	},
}

func init() {
	gitCmd.Flags().BoolVar(&gitStaged, "staged", false, "Analyze staged changes")
	gitCmd.Flags().BoolVar(&gitUncommitted, "uncommitted", false, "Analyze all uncommitted changes")
	gitCmd.Flags().StringVar(&gitBranch, "branch", "", "Analyze changes against a branch")
	gitCmd.Flags().StringVar(&gitFormat, "format", "table", "Output format: table, json, md")
}

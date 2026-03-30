package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/wuqiang1985/go-route-impact/internal/analyzer"
	"github.com/wuqiang1985/go-route-impact/internal/config"
	"github.com/wuqiang1985/go-route-impact/internal/output"
	"github.com/wuqiang1985/go-route-impact/pkg/model"
)

var (
	graphFunc   string
	graphFormat string
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Show call graph for a function",
	RunE: func(cmd *cobra.Command, args []string) error {
		if graphFunc == "" {
			return fmt.Errorf("--func is required")
		}

		cfg, err := config.Load(configFile, projectDir)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		a, err := analyzer.New(projectDir, cfg)
		if err != nil {
			return fmt.Errorf("initialize analyzer: %w", err)
		}

		// Find the function
		matches := a.GetCallGraph().FindFunc(graphFunc)
		if len(matches) == 0 {
			return fmt.Errorf("function not found: %s", graphFunc)
		}

		funcID := matches[0]
		result := a.ImpactByFunc(funcID)

		switch graphFormat {
		case "json":
			return output.PrintImpactJSON(os.Stdout, &model.ImpactResult{
				ChangedFuncs:   []model.FuncLocation{{FuncID: funcID}},
				AffectedRoutes: result.AffectedRoutes,
			})
		default:
			output.PrintCallGraph(os.Stdout, funcID, result)
		}

		return nil
	},
}

func init() {
	graphCmd.Flags().StringVar(&graphFunc, "func", "", `Function to analyze (e.g., "services.gameInfoService.GetEnvConfig")`)
	graphCmd.Flags().StringVar(&graphFormat, "format", "ascii", "Output format: ascii, json")
	_ = graphCmd.MarkFlagRequired("func")
}

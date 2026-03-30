package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pnt-team/go-route-impact-v2/internal/analyzer"
	"github.com/pnt-team/go-route-impact-v2/internal/config"
	"github.com/pnt-team/go-route-impact-v2/internal/output"
)

var routesFormat string

var routesCmd = &cobra.Command{
	Use:   "routes",
	Short: "List all routes in the project",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(configFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		a, err := analyzer.New(projectDir, cfg)
		if err != nil {
			return fmt.Errorf("initialize analyzer: %w", err)
		}

		routes := a.AllRoutes()

		// Print stats
		funcs, edges := a.GraphStats()
		fmt.Fprintf(os.Stderr, "Call graph: %d functions, %d edges\n", funcs, edges)

		switch routesFormat {
		case "json":
			return output.PrintRoutesJSON(os.Stdout, routes)
		case "md", "markdown":
			output.PrintRoutesMarkdown(os.Stdout, routes)
		default:
			output.PrintRouteTable(os.Stdout, routes)
		}

		return nil
	},
}

func init() {
	routesCmd.Flags().StringVar(&routesFormat, "format", "table", "Output format: table, json, md")
}

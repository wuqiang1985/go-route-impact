package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	projectDir string
	configFile string
)

var rootCmd = &cobra.Command{
	Use:   "go-route-impact",
	Short: "Function-level route impact analysis for Go projects (v2)",
	Long: `go-route-impact v2 uses function-level call graph analysis to determine
which HTTP routes are affected by code changes.

Unlike v1 (file-level), v2 traces from the exact changed function through
the call graph to find only the truly affected routes.

  v1: change service/game_info.go     → import chain → 118 routes (whole file)
  v2: change GetEnvConfig() function  → call graph   →   1 route  (precise)`,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&projectDir, "project", "p", ".", "Project root directory")
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Config file path (default: .route-impact.yaml)")

	rootCmd.AddCommand(checkCmd)
	rootCmd.AddCommand(routesCmd)
	rootCmd.AddCommand(gitCmd)
	rootCmd.AddCommand(hookCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(graphCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

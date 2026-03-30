package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/wuqiang1985/go-route-impact/internal/analyzer"
	"github.com/wuqiang1985/go-route-impact/internal/config"
	"github.com/wuqiang1985/go-route-impact/internal/output"
	"github.com/wuqiang1985/go-route-impact/pkg/model"
)

var (
	checkFunc string
	checkFile string
	checkLine string
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Analyze which routes are affected by a function change",
	Long: `Analyze route impact by specifying either:
  --func "services.gameInfoService.GetEnvConfig"   (function name)
  --file path/to/file.go --line 120                (file + line number)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if checkFunc == "" && checkFile == "" {
			return fmt.Errorf("specify --func or --file+--line")
		}

		cfg, err := config.Load(configFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		a, err := analyzer.New(projectDir, cfg)
		if err != nil {
			return fmt.Errorf("initialize analyzer: %w", err)
		}

		var result *model.ImpactResult

		if checkFunc != "" {
			result, err = a.ImpactByFuncName(checkFunc)
		} else {
			line, parseErr := strconv.Atoi(checkLine)
			if parseErr != nil {
				return fmt.Errorf("invalid --line: %w", parseErr)
			}
			result, err = a.ImpactByFileLine(checkFile, line)
		}

		if err != nil {
			return fmt.Errorf("analyze impact: %w", err)
		}

		output.PrintImpactResult(os.Stdout, result)
		return nil
	},
}

func init() {
	checkCmd.Flags().StringVar(&checkFunc, "func", "", `Function to analyze (e.g., "services.gameInfoService.GetEnvConfig")`)
	checkCmd.Flags().StringVar(&checkFile, "file", "", "File path (used with --line)")
	checkCmd.Flags().StringVar(&checkLine, "line", "", "Line number (used with --file)")
}

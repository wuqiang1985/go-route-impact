package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/wuqiang1985/go-route-impact/internal/config"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a .route-impact.yaml config file",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.DefaultConfig()
		if err := config.Save("", cfg); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		fmt.Println("✅ Created .route-impact.yaml with default settings.")
		fmt.Println("   Edit the file to customize framework, entry point, and exclusions.")
		return nil
	},
}

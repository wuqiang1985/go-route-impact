package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pnt-team/go-route-impact-v2/internal/gitutil"
)

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage git hooks",
}

var hookInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install pre-commit hook for automatic function-level route impact analysis",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := gitutil.InstallHook(projectDir); err != nil {
			return err
		}
		fmt.Println("✅ Pre-commit hook installed successfully!")
		fmt.Println("   Function-level route impact will be analyzed on every commit.")
		return nil
	},
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove pre-commit hook",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := gitutil.UninstallHook(projectDir); err != nil {
			return err
		}
		fmt.Println("✅ Pre-commit hook removed.")
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
}

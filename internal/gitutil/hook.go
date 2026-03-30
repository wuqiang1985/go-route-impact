package gitutil

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const hookScript = `#!/bin/sh
# go-route-impact pre-commit hook (v2 - function-level analysis)
# Analyzes which routes are affected by staged changes

echo ""
echo "🔍 Analyzing function-level route impact for staged changes..."
echo ""

go-route-impact git --staged --project "$(git rev-parse --show-toplevel)"

echo ""
`

// InstallHook installs the pre-commit git hook.
func InstallHook(projectRoot string) error {
	hooksDir := filepath.Join(projectRoot, ".git", "hooks")
	hookPath := filepath.Join(hooksDir, "pre-commit")

	if _, err := os.Stat(hookPath); err == nil {
		existing, err := os.ReadFile(hookPath)
		if err != nil {
			return fmt.Errorf("read existing hook: %w", err)
		}

		if strings.Contains(string(existing), "go-route-impact") {
			return fmt.Errorf("go-route-impact hook already installed")
		}

		f, err := os.OpenFile(hookPath, os.O_APPEND|os.O_WRONLY, 0755)
		if err != nil {
			return fmt.Errorf("open existing hook: %w", err)
		}
		defer f.Close()

		_, err = f.WriteString("\n" + hookScript)
		return err
	}

	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("create hooks directory: %w", err)
	}

	return os.WriteFile(hookPath, []byte(hookScript), 0755)
}

// UninstallHook removes the pre-commit git hook.
func UninstallHook(projectRoot string) error {
	hookPath := filepath.Join(projectRoot, ".git", "hooks", "pre-commit")

	existing, err := os.ReadFile(hookPath)
	if err != nil {
		return fmt.Errorf("no pre-commit hook found")
	}

	if !strings.Contains(string(existing), "go-route-impact") {
		return fmt.Errorf("go-route-impact hook not found in pre-commit")
	}

	return os.Remove(hookPath)
}

// GitRoot finds the root of the git repository.
func GitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return filepath.Clean(strings.TrimSpace(string(out))), nil
}

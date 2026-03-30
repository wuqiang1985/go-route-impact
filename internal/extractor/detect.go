package extractor

import (
	"os"
	"path/filepath"
	"strings"
)

// DetectFramework scans go.mod to determine which web framework is used.
// Returns "iris", "gin", or empty string if unknown.
func DetectFramework(projectRoot string) string {
	goModPath := filepath.Join(projectRoot, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}

	content := string(data)

	// Check each line for known framework imports
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "kataras/iris") {
			return "iris"
		}
		if strings.Contains(line, "gin-gonic/gin") {
			return "gin"
		}
	}

	return ""
}

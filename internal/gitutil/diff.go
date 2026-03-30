package gitutil

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pnt-team/go-route-impact-v2/internal/astutil"
	"github.com/pnt-team/go-route-impact-v2/pkg/model"
)

// ChangedLine represents a range of changed lines in a file.
type ChangedLine struct {
	File      string // project-relative path
	StartLine int
	EndLine   int
}

// ChangedFilesWithLines returns changed .go files with their changed line ranges.
// Uses git diff --unified=0 to get exact line numbers.
func ChangedFilesWithLines(projectRoot string, mode string) ([]ChangedLine, error) {
	var args []string

	switch mode {
	case "staged":
		args = []string{"diff", "--cached", "--unified=0", "--diff-filter=ACMR"}
	case "uncommitted":
		args = []string{"diff", "--unified=0", "--diff-filter=ACMR", "HEAD"}
	default:
		args = []string{"diff", "--unified=0", "--diff-filter=ACMR", mode + "...HEAD"}
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		// For uncommitted mode, also get unstaged changes
		if mode == "uncommitted" {
			return changedLinesUncommitted(projectRoot)
		}
		return nil, fmt.Errorf("git diff: %w", err)
	}

	return parseDiffLines(string(out), projectRoot), nil
}

// changedLinesUncommitted collects both staged and unstaged changes.
func changedLinesUncommitted(projectRoot string) ([]ChangedLine, error) {
	// Staged changes
	staged, _ := ChangedFilesWithLines(projectRoot, "staged")

	// Unstaged changes
	cmd := exec.Command("git", "diff", "--unified=0", "--diff-filter=ACMR")
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return staged, nil
	}

	unstaged := parseDiffLines(string(out), projectRoot)

	// Merge
	return append(staged, unstaged...), nil
}

// parseDiffLines parses unified diff output with --unified=0 to extract changed line ranges.
var hunkHeaderRe = regexp.MustCompile(`^@@\s+.*\+(\d+)(?:,(\d+))?\s+@@`)

func parseDiffLines(diffOutput string, projectRoot string) []ChangedLine {
	var result []ChangedLine
	var currentFile string

	lines := strings.Split(diffOutput, "\n")
	for _, line := range lines {
		// Detect file header: +++ b/path/to/file.go
		if strings.HasPrefix(line, "+++ b/") {
			file := line[6:]
			if strings.HasSuffix(file, ".go") && !strings.HasSuffix(file, "_test.go") {
				currentFile = file
			} else {
				currentFile = ""
			}
			continue
		}

		// Parse hunk headers: @@ -X,Y +Z,W @@
		if currentFile != "" && strings.HasPrefix(line, "@@") {
			matches := hunkHeaderRe.FindStringSubmatch(line)
			if matches == nil {
				continue
			}

			startLine, _ := strconv.Atoi(matches[1])
			count := 1
			if matches[2] != "" {
				count, _ = strconv.Atoi(matches[2])
			}

			if count == 0 {
				// Pure deletion, mark the line after
				continue
			}

			result = append(result, ChangedLine{
				File:      currentFile,
				StartLine: startLine,
				EndLine:   startLine + count - 1,
			})
		}
	}

	return result
}

// ChangedFuncs maps changed lines to their containing functions using AST.
func ChangedFuncs(changedLines []ChangedLine, parsedFiles []astutil.ParsedFile, resolver *astutil.Resolver) []model.FuncLocation {
	// Build file lookup map
	fileMap := make(map[string]*astutil.ParsedFile)
	for i := range parsedFiles {
		relPath, err := resolver.RelPath(parsedFiles[i].FilePath)
		if err == nil {
			fileMap[relPath] = &parsedFiles[i]
		}
		// Also index by absolute path
		fileMap[parsedFiles[i].FilePath] = &parsedFiles[i]
	}

	seen := make(map[string]bool)
	var result []model.FuncLocation

	for _, cl := range changedLines {
		pf := fileMap[cl.File]
		if pf == nil {
			// Try with absolute path
			absPath := filepath.Join(resolver.ProjectRoot, cl.File)
			pf = fileMap[absPath]
		}
		if pf == nil {
			continue
		}

		pkgPath, err := resolver.FileToPackage(pf.FilePath)
		if err != nil {
			continue
		}

		// Check each changed line against function ranges
		funcRanges := astutil.AllFuncRanges(pf.File, pf.Fset)
		for _, fr := range funcRanges {
			// Check if any changed line falls within this function
			if cl.EndLine < fr.StartLine || cl.StartLine > fr.EndLine {
				continue
			}

			receiver := astutil.ReceiverTypeName(fr.Decl)
			funcID := model.FuncID{
				Pkg:      pkgPath,
				Receiver: receiver,
				Name:     fr.Decl.Name.Name,
			}

			key := funcID.Key()
			if seen[key] {
				continue
			}
			seen[key] = true

			relPath, _ := resolver.RelPath(pf.FilePath)
			result = append(result, model.FuncLocation{
				FuncID:    funcID,
				File:      relPath,
				StartLine: fr.StartLine,
				EndLine:   fr.EndLine,
			})
		}
	}

	return result
}

// ChangedGoFiles returns just the list of changed .go file paths (for backwards compat).
func ChangedGoFiles(projectRoot string, mode string) ([]string, error) {
	var args []string

	switch mode {
	case "staged":
		args = []string{"diff", "--cached", "--name-only", "--diff-filter=ACMR"}
	case "uncommitted":
		args = []string{"diff", "--name-only", "--diff-filter=ACMR", "HEAD"}
	default:
		args = []string{"diff", "--name-only", "--diff-filter=ACMR", mode + "...HEAD"}
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return filterGoFiles(strings.TrimSpace(string(out))), nil
}

func filterGoFiles(output string) []string {
	if output == "" {
		return nil
	}

	var goFiles []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".go") && !strings.HasSuffix(line, "_test.go") {
			goFiles = append(goFiles, line)
		}
	}

	return goFiles
}

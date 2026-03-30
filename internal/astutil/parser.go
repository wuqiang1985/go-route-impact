package astutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// ParsedFile holds a fully-parsed Go file with AST and position info.
type ParsedFile struct {
	// FilePath is the absolute path to the file.
	FilePath string
	// File is the parsed AST.
	File *ast.File
	// Fset is the token file set for position info.
	Fset *token.FileSet
	// PackageName is the declared package name.
	PackageName string
}

// ParseProjectFull walks the project directory and fully parses all .go files.
// It returns parsed files with full AST (needed for call graph construction).
func ParseProjectFull(projectRoot string, excludeDirs []string) ([]ParsedFile, error) {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}

	excludeSet := make(map[string]bool, len(excludeDirs))
	for _, d := range excludeDirs {
		excludeSet[strings.TrimSuffix(d, "/")] = true
	}

	// Collect all .go files first
	var goFiles []string
	err = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			relDir, _ := filepath.Rel(absRoot, path)
			for excluded := range excludeSet {
				if relDir == excluded || strings.HasPrefix(relDir, excluded+"/") {
					return filepath.SkipDir
				}
			}
			if strings.HasPrefix(info.Name(), ".") && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			goFiles = append(goFiles, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Parse files in parallel
	type result struct {
		pf  ParsedFile
		err error
	}

	results := make([]result, len(goFiles))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 16)

	for i, f := range goFiles {
		wg.Add(1)
		go func(idx int, filePath string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			fset := token.NewFileSet()
			af, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
			if err != nil {
				results[idx] = result{err: err}
				return
			}

			results[idx] = result{pf: ParsedFile{
				FilePath:    filePath,
				File:        af,
				Fset:        fset,
				PackageName: af.Name.Name,
			}}
		}(i, f)
	}

	wg.Wait()

	parsed := make([]ParsedFile, 0, len(results))
	for _, r := range results {
		if r.err != nil {
			continue
		}
		parsed = append(parsed, r.pf)
	}

	return parsed, nil
}

// ParseFullFile parses a single Go file with full AST.
func ParseFullFile(filePath string) (*ast.File, *token.FileSet, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}
	return f, fset, nil
}

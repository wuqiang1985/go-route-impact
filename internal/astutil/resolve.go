package astutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// Resolver maps file system paths to Go import paths and vice versa.
type Resolver struct {
	// ProjectRoot is the absolute path to the project root (where go.mod lives).
	ProjectRoot string
	// ModulePath is the module path from go.mod (e.g., "github.com/org/repo").
	ModulePath string
}

// NewResolver creates a Resolver by reading go.mod from the project root.
func NewResolver(projectRoot string) (*Resolver, error) {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}

	goModPath := filepath.Join(absRoot, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("read go.mod: %w", err)
	}

	mf, err := modfile.Parse(goModPath, data, nil)
	if err != nil {
		return nil, fmt.Errorf("parse go.mod: %w", err)
	}

	return &Resolver{
		ProjectRoot: absRoot,
		ModulePath:  mf.Module.Mod.Path,
	}, nil
}

// FileToPackage converts a file path (absolute or relative to project root)
// to its Go import path.
func (r *Resolver) FileToPackage(filePath string) (string, error) {
	absPath := filePath
	if !filepath.IsAbs(filePath) {
		absPath = filepath.Join(r.ProjectRoot, filePath)
	}

	dir := filepath.Dir(absPath)
	relDir, err := filepath.Rel(r.ProjectRoot, dir)
	if err != nil {
		return "", fmt.Errorf("relative path: %w", err)
	}

	if relDir == "." {
		return r.ModulePath, nil
	}

	return r.ModulePath + "/" + filepath.ToSlash(relDir), nil
}

// PackageToDir converts a Go import path to the absolute directory path.
// Returns empty string if the package is not within this module.
func (r *Resolver) PackageToDir(pkgPath string) string {
	if !strings.HasPrefix(pkgPath, r.ModulePath) {
		return ""
	}

	suffix := strings.TrimPrefix(pkgPath, r.ModulePath)
	suffix = strings.TrimPrefix(suffix, "/")

	if suffix == "" {
		return r.ProjectRoot
	}

	return filepath.Join(r.ProjectRoot, filepath.FromSlash(suffix))
}

// IsInternal returns true if the import path belongs to this module.
func (r *Resolver) IsInternal(importPath string) bool {
	return importPath == r.ModulePath || strings.HasPrefix(importPath, r.ModulePath+"/")
}

// RelPath returns the project-relative path for an absolute path.
func (r *Resolver) RelPath(absPath string) (string, error) {
	return filepath.Rel(r.ProjectRoot, absPath)
}

// RelPkg converts an absolute package path to a project-relative one.
func (r *Resolver) RelPkg(pkgPath string) string {
	if len(pkgPath) > len(r.ModulePath) {
		return pkgPath[len(r.ModulePath)+1:]
	}
	return pkgPath
}

// ShortPkg returns only the last segment of the package path.
func (r *Resolver) ShortPkg(pkgPath string) string {
	idx := strings.LastIndex(pkgPath, "/")
	if idx >= 0 {
		return pkgPath[idx+1:]
	}
	return pkgPath
}

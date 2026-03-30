package analyzer

import (
	"path/filepath"

	"github.com/pnt-team/go-route-impact-v2/internal/astutil"
	"github.com/pnt-team/go-route-impact-v2/pkg/model"
)

// ImpactByFunc analyzes which routes are affected by changes to the given function.
func (a *Analyzer) ImpactByFunc(funcID model.FuncID) *model.ImpactResult {
	result := &model.ImpactResult{}

	// Record the changed function
	if loc, ok := a.CallGraph.LookupFunc(funcID); ok {
		result.ChangedFuncs = []model.FuncLocation{loc}
	} else {
		result.ChangedFuncs = []model.FuncLocation{{FuncID: funcID}}
	}

	// If the function itself is a route handler, include its routes directly
	if routes := a.RoutesForHandler(funcID); len(routes) > 0 {
		for _, r := range routes {
			result.AffectedRoutes = append(result.AffectedRoutes, model.CallChain{
				Chain: []model.FuncID{funcID},
				Route: r,
			})
		}
		return result
	}

	// BFS: traverse reverse call graph until we hit route handlers
	chains := a.CallGraph.CallersBFS(funcID, func(id model.FuncID) bool {
		return a.IsRouteHandler(id)
	})

	// Map chains to routes
	for _, chain := range chains {
		lastFunc := chain[len(chain)-1]
		routes := a.RoutesForHandler(lastFunc)
		for _, r := range routes {
			result.AffectedRoutes = append(result.AffectedRoutes, model.CallChain{
				Chain: chain,
				Route: r,
			})
		}
	}

	return result
}

// ImpactByFuncName finds the function by short name and analyzes impact.
func (a *Analyzer) ImpactByFuncName(name string) (*model.ImpactResult, error) {
	matches := a.CallGraph.FindFunc(name)
	if len(matches) == 0 {
		return &model.ImpactResult{}, nil
	}

	// Merge results from all matching functions
	merged := &model.ImpactResult{}
	for _, funcID := range matches {
		result := a.ImpactByFunc(funcID)
		merged.ChangedFuncs = append(merged.ChangedFuncs, result.ChangedFuncs...)
		merged.AffectedRoutes = append(merged.AffectedRoutes, result.AffectedRoutes...)
	}

	return merged, nil
}

// ImpactByFileLine analyzes which routes are affected by a change at the given file:line.
func (a *Analyzer) ImpactByFileLine(file string, line int) (*model.ImpactResult, error) {
	absPath := file
	if !filepath.IsAbs(file) {
		absPath = filepath.Join(a.ProjectRoot, file)
	}

	// Find the parsed file
	var targetFile *astutil.ParsedFile
	for i := range a.parsedFiles {
		if a.parsedFiles[i].FilePath == absPath {
			targetFile = &a.parsedFiles[i]
			break
		}
	}

	if targetFile == nil {
		return &model.ImpactResult{}, nil
	}

	// Locate the function containing this line
	fn := astutil.LocateFunc(targetFile.File, targetFile.Fset, line)
	if fn == nil {
		return &model.ImpactResult{}, nil
	}

	pkgPath, err := a.Resolver.FileToPackage(absPath)
	if err != nil {
		return &model.ImpactResult{}, nil
	}

	receiver := astutil.ReceiverTypeName(fn)
	funcID := model.FuncID{
		Pkg:      pkgPath,
		Receiver: receiver,
		Name:     fn.Name.Name,
	}

	return a.ImpactByFunc(funcID), nil
}

// ImpactByFileLines analyzes impact for multiple changed lines in a file.
// It deduplicates functions (multiple lines in the same function → one analysis).
func (a *Analyzer) ImpactByFileLines(file string, lines []int) (*model.ImpactResult, error) {
	absPath := file
	if !filepath.IsAbs(file) {
		absPath = filepath.Join(a.ProjectRoot, file)
	}

	var targetFile *astutil.ParsedFile
	for i := range a.parsedFiles {
		if a.parsedFiles[i].FilePath == absPath {
			targetFile = &a.parsedFiles[i]
			break
		}
	}

	if targetFile == nil {
		return &model.ImpactResult{}, nil
	}

	pkgPath, err := a.Resolver.FileToPackage(absPath)
	if err != nil {
		return &model.ImpactResult{}, nil
	}

	// Deduplicate: line → function
	seenFuncs := make(map[string]bool)
	merged := &model.ImpactResult{}

	for _, line := range lines {
		fn := astutil.LocateFunc(targetFile.File, targetFile.Fset, line)
		if fn == nil {
			continue
		}

		receiver := astutil.ReceiverTypeName(fn)
		funcID := model.FuncID{
			Pkg:      pkgPath,
			Receiver: receiver,
			Name:     fn.Name.Name,
		}

		key := funcID.Key()
		if seenFuncs[key] {
			continue
		}
		seenFuncs[key] = true

		result := a.ImpactByFunc(funcID)
		merged.ChangedFuncs = append(merged.ChangedFuncs, result.ChangedFuncs...)
		merged.AffectedRoutes = append(merged.AffectedRoutes, result.AffectedRoutes...)
	}

	return merged, nil
}

// ImpactByChangedFuncs analyzes impact for a list of changed functions.
func (a *Analyzer) ImpactByChangedFuncs(funcs []model.FuncLocation) *model.ImpactResult {
	merged := &model.ImpactResult{}
	seen := make(map[string]bool)

	for _, fl := range funcs {
		key := fl.FuncID.Key()
		if seen[key] {
			continue
		}
		seen[key] = true

		result := a.ImpactByFunc(fl.FuncID)
		merged.ChangedFuncs = append(merged.ChangedFuncs, result.ChangedFuncs...)
		merged.AffectedRoutes = append(merged.AffectedRoutes, result.AffectedRoutes...)
	}

	return merged
}

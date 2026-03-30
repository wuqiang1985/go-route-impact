package analyzer

import (
	"fmt"
	"path/filepath"

	"github.com/pnt-team/go-route-impact-v2/internal/astutil"
	"github.com/pnt-team/go-route-impact-v2/internal/callgraph"
	"github.com/pnt-team/go-route-impact-v2/internal/config"
	"github.com/pnt-team/go-route-impact-v2/internal/extractor"
	_ "github.com/pnt-team/go-route-impact-v2/internal/extractor/iris" // register iris extractor
	"github.com/pnt-team/go-route-impact-v2/pkg/model"
)

// Analyzer is the core coordinator that combines callgraph + extractor.
type Analyzer struct {
	ProjectRoot string
	Config      *config.Config
	Resolver    *astutil.Resolver
	CallGraph   *callgraph.Graph
	Routes      []model.Route
	// routeHandlers maps handler FuncID key → []Route
	routeHandlers map[string][]model.Route
	// parsedFiles are kept for locate-by-line operations
	parsedFiles []astutil.ParsedFile
}

// New creates and initializes an Analyzer for the given project.
func New(projectRoot string, cfg *config.Config) (*Analyzer, error) {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve project root: %w", err)
	}

	resolver, err := astutil.NewResolver(absRoot)
	if err != nil {
		return nil, fmt.Errorf("create resolver: %w", err)
	}

	a := &Analyzer{
		ProjectRoot:   absRoot,
		Config:        cfg,
		Resolver:      resolver,
		routeHandlers: make(map[string][]model.Route),
	}

	if err := a.buildIndex(); err != nil {
		return nil, err
	}

	return a, nil
}

// buildIndex constructs the call graph and extracts routes.
func (a *Analyzer) buildIndex() error {
	// Step 1: Full AST parse all project files
	parsed, err := astutil.ParseProjectFull(a.ProjectRoot, a.Config.Exclude)
	if err != nil {
		return fmt.Errorf("parse project: %w", err)
	}
	a.parsedFiles = parsed

	// Step 2: Build function-level call graph
	builder := callgraph.NewBuilder(a.Resolver)
	a.CallGraph = builder.Build(parsed)

	// Step 3: Extract routes (with handler FuncID)
	framework := a.Config.Framework
	if framework == "auto" {
		framework = "iris"
	}

	ext, err := extractor.Get(framework)
	if err != nil {
		return fmt.Errorf("get extractor: %w", err)
	}

	routes, err := ext.Extract(a.ProjectRoot, a.Config.EntryPoint, a.Resolver)
	if err != nil {
		return fmt.Errorf("extract routes: %w", err)
	}
	a.Routes = routes

	// Step 4: Build route handler index
	for _, r := range routes {
		if !r.HandlerFuncID.IsZero() {
			key := r.HandlerFuncID.Key()
			a.routeHandlers[key] = append(a.routeHandlers[key], r)
		}
	}

	return nil
}

// AllRoutes returns all extracted routes.
func (a *Analyzer) AllRoutes() []model.Route {
	return a.Routes
}

// GraphStats returns call graph statistics.
func (a *Analyzer) GraphStats() (funcs, edges int) {
	return a.CallGraph.Stats()
}

// GetCallGraph returns the internal call graph.
func (a *Analyzer) GetCallGraph() *callgraph.Graph {
	return a.CallGraph
}

// GetParsedFiles returns the parsed files.
func (a *Analyzer) GetParsedFiles() []astutil.ParsedFile {
	return a.parsedFiles
}

// IsRouteHandler checks if the given FuncID is a route handler.
func (a *Analyzer) IsRouteHandler(id model.FuncID) bool {
	_, ok := a.routeHandlers[id.Key()]
	return ok
}

// RoutesForHandler returns the routes handled by the given FuncID.
func (a *Analyzer) RoutesForHandler(id model.FuncID) []model.Route {
	return a.routeHandlers[id.Key()]
}

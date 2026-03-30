package analyzer

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wuqiang1985/go-route-impact/internal/astutil"
	"github.com/wuqiang1985/go-route-impact/internal/callgraph"
	"github.com/wuqiang1985/go-route-impact/internal/config"
	"github.com/wuqiang1985/go-route-impact/internal/extractor"
	_ "github.com/wuqiang1985/go-route-impact/internal/extractor/gin"  // register gin extractor
	_ "github.com/wuqiang1985/go-route-impact/internal/extractor/iris" // register iris extractor
	"github.com/wuqiang1985/go-route-impact/pkg/model"
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

	// Check go.mod exists
	goModPath := filepath.Join(absRoot, "go.mod")
	if _, err := os.Stat(goModPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("go.mod not found in %s\n\n"+
			"Please run from the project root directory, or use --project to specify it:\n"+
			"  go-route-impact --project /path/to/your/project <command>", absRoot)
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
		framework = extractor.DetectFramework(a.ProjectRoot)
		if framework == "" {
			framework = "iris" // fallback
		}
	}

	ext, err := extractor.Get(framework)
	if err != nil {
		return fmt.Errorf("get extractor: %w", err)
	}

	// Check entry point exists before extracting
	entryPath := filepath.Join(a.ProjectRoot, a.Config.EntryPoint)
	if _, statErr := os.Stat(entryPath); os.IsNotExist(statErr) {
		return fmt.Errorf("entry point not found: %s\n\n"+
			"The default entry point is main.go in the project root.\n"+
			"If your main.go is in a different location, create a config file:\n"+
			"  go-route-impact init\n"+
			"Then edit .route-impact.yaml to set the correct entry_point, e.g.:\n"+
			"  entry_point: cmd/api/main.go", entryPath)
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

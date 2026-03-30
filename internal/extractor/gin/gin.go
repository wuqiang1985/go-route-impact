package gin

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/pnt-team/go-route-impact-v2/internal/astutil"
	"github.com/pnt-team/go-route-impact-v2/internal/extractor"
	"github.com/pnt-team/go-route-impact-v2/pkg/model"
)

func init() {
	extractor.Register(&GinExtractor{})
}

// GinExtractor extracts routes from Gin projects.
//
// Supported patterns:
//
//	r := gin.Default() / gin.New()
//	r.GET("/path", handler)
//	r.POST("/path", controller.Method)
//
//	v1 := r.Group("/api/v1")
//	v1.GET("/users", controller.List)
//
//	func RegisterRoutes(rg *gin.RouterGroup) {
//	    rg.GET("/items", ctrl.List)
//	}
type GinExtractor struct{}

func (e *GinExtractor) Name() string { return "gin" }

// Extract scans all Go files in the project to find Gin route registrations.
// Unlike Iris which has a single entry-point pattern, Gin routes can be
// registered anywhere, so we scan all files.
func (e *GinExtractor) Extract(projectRoot string, entryPoint string, resolver *astutil.Resolver) ([]model.Route, error) {
	// Phase 1: Parse entry point to find top-level router and groups.
	entryPath := filepath.Join(projectRoot, entryPoint)
	entryFile, _, err := astutil.ParseFullFile(entryPath)
	if err != nil {
		return nil, fmt.Errorf("parse entry point %s: %w", entryPoint, err)
	}

	// Track engine and group variables with their path prefixes.
	// e.g., r := gin.Default() → ginVars["r"] = ""
	//        v1 := r.Group("/api/v1") → ginVars["v1"] = "/api/v1"
	ginVars := make(map[string]string)

	entryAliases := buildImportAliases(entryFile)

	var mainFunc *ast.FuncDecl
	for _, decl := range entryFile.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == "main" {
			mainFunc = fn
			break
		}
	}

	if mainFunc == nil || mainFunc.Body == nil {
		return nil, fmt.Errorf("main function not found in %s", entryPoint)
	}

	var routes []model.Route

	// Walk main function to collect gin engine/group vars and direct routes.
	routes = append(routes, extractFromBlock(mainFunc.Body, ginVars, "", entryAliases, resolver, projectRoot, entryPoint)...)

	// Phase 2: Find route registration functions called from main.
	// e.g., routes.Register(r) or registerUserRoutes(v1)
	routes = append(routes, extractFromFuncCalls(mainFunc.Body, ginVars, entryAliases, resolver, projectRoot)...)

	return routes, nil
}

// extractFromBlock walks a block statement and extracts gin routes.
// It tracks Group() calls and direct HTTP method calls.
func extractFromBlock(body *ast.BlockStmt, ginVars map[string]string, inheritPrefix string, aliases map[string]string, resolver *astutil.Resolver, projectRoot string, file string) []model.Route {
	if body == nil {
		return nil
	}

	var routes []model.Route

	for _, stmt := range body.List {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			routes = append(routes, handleAssign(s, ginVars, inheritPrefix, aliases, resolver, projectRoot, file)...)

		case *ast.ExprStmt:
			if call, ok := s.X.(*ast.CallExpr); ok {
				routes = append(routes, handleCall(call, ginVars, aliases, resolver, projectRoot, file)...)
			}

		case *ast.BlockStmt:
			// Naked block: { v1.GET(...) }
			routes = append(routes, extractFromBlock(s, ginVars, inheritPrefix, aliases, resolver, projectRoot, file)...)
		}
	}

	return routes
}

// handleAssign processes assignment statements to track gin variables and extract routes.
func handleAssign(assign *ast.AssignStmt, ginVars map[string]string, inheritPrefix string, aliases map[string]string, resolver *astutil.Resolver, projectRoot string, file string) []model.Route {
	if len(assign.Lhs) < 1 || len(assign.Rhs) < 1 {
		return nil
	}

	ident, ok := assign.Lhs[0].(*ast.Ident)
	if !ok {
		return nil
	}

	rhs := assign.Rhs[0]
	call, ok := rhs.(*ast.CallExpr)
	if !ok {
		return nil
	}

	// Check for gin.Default() / gin.New()
	if isGinInit(call, aliases) {
		ginVars[ident.Name] = inheritPrefix
		return nil
	}

	// Check for r.Group("/prefix") or v1.Group("/prefix")
	if prefix, base, ok := isGroupCall(call, ginVars); ok {
		ginVars[ident.Name] = base + prefix
		return nil
	}

	return nil
}

// handleCall processes a function call that may be a route registration.
func handleCall(call *ast.CallExpr, ginVars map[string]string, aliases map[string]string, resolver *astutil.Resolver, projectRoot string, file string) []model.Route {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	methodName := sel.Sel.Name

	// Check for HTTP method calls: r.GET, v1.POST, etc.
	if isHTTPMethod(methodName) {
		return handleHTTPRoute(call, sel, ginVars, aliases, resolver, projectRoot, file)
	}

	return nil
}

// handleHTTPRoute extracts route info from r.GET("/path", handler) calls.
func handleHTTPRoute(call *ast.CallExpr, sel *ast.SelectorExpr, ginVars map[string]string, aliases map[string]string, resolver *astutil.Resolver, projectRoot string, file string) []model.Route {
	if len(call.Args) < 2 {
		return nil
	}

	// Identify the receiver variable
	prefix := ""
	if varIdent, ok := sel.X.(*ast.Ident); ok {
		if p, exists := ginVars[varIdent.Name]; exists {
			prefix = p
		}
	}

	routePath := extractStringLit(call.Args[0])
	if routePath == "" {
		return nil
	}

	fullPath := prefix + routePath
	method := strings.ToUpper(sel.Sel.Name)
	if method == "ANY" {
		method = "ANY"
	}

	// The handler is the last argument (Gin allows middleware before handler)
	handlerExpr := call.Args[len(call.Args)-1]
	handler := extractHandlerName(handlerExpr)
	handlerFuncID := resolveHandlerFuncID(handlerExpr, aliases, resolver, projectRoot)

	relFile, _ := filepath.Rel(projectRoot, filepath.Join(projectRoot, file))
	pkgPath, _ := resolver.FileToPackage(filepath.Join(projectRoot, file))

	return []model.Route{{
		Method:        method,
		Path:          fullPath,
		Handler:       handler,
		File:          relFile,
		Package:       pkgPath,
		HandlerFuncID: handlerFuncID,
	}}
}

// extractFromFuncCalls finds calls like registerRoutes(r) or pkg.Setup(v1)
// in the main function body, then follows them to extract routes.
func extractFromFuncCalls(body *ast.BlockStmt, ginVars map[string]string, aliases map[string]string, resolver *astutil.Resolver, projectRoot string) []model.Route {
	if body == nil {
		return nil
	}

	var routes []model.Route

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// pkg.SetupRoutes(r) or pkg.SetupRoutes(v1)
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		ident, isIdent := call.Fun.(*ast.Ident)

		var funcName, pkgAlias string
		if isSel {
			if pkgId, ok := sel.X.(*ast.Ident); ok {
				pkgAlias = pkgId.Name
				funcName = sel.Sel.Name
			}
		} else if isIdent {
			funcName = ident.Name
		}

		if funcName == "" {
			return true
		}

		// Skip known non-route functions
		if isGinInit(call, aliases) {
			return true
		}
		if _, _, ok := isGroupCall(call, ginVars); ok {
			return true
		}
		if isHTTPMethod(funcName) {
			return true // already handled
		}

		// Check if any argument is a gin variable
		argPrefix := ""
		foundGinArg := false
		for _, arg := range call.Args {
			if argIdent, ok := arg.(*ast.Ident); ok {
				if p, exists := ginVars[argIdent.Name]; exists {
					argPrefix = p
					foundGinArg = true
					break
				}
			}
		}

		if !foundGinArg {
			return true
		}

		// Resolve the function and extract routes from it
		var importPath string
		if pkgAlias != "" {
			importPath = aliases[pkgAlias]
		}

		funcRoutes := resolveGinRouteFunc(funcName, importPath, argPrefix, aliases, resolver, projectRoot)
		routes = append(routes, funcRoutes...)

		return true
	})

	return routes
}

// resolveGinRouteFunc finds a function by name in the given package and extracts routes from it.
func resolveGinRouteFunc(funcName string, importPath string, prefix string, aliases map[string]string, resolver *astutil.Resolver, projectRoot string) []model.Route {
	var searchDir string
	if importPath != "" && resolver.IsInternal(importPath) {
		searchDir = resolver.PackageToDir(importPath)
	} else {
		// Local function, search in project root
		searchDir = projectRoot
	}

	if searchDir == "" {
		return nil
	}

	pattern := filepath.Join(searchDir, "*.go")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil
	}

	for _, filePath := range matches {
		if strings.HasSuffix(filePath, "_test.go") {
			continue
		}

		f, _, err := astutil.ParseFullFile(filePath)
		if err != nil {
			continue
		}

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != funcName || fn.Body == nil {
				continue
			}

			// Determine the parameter name used for the gin engine/group
			paramVars := make(map[string]string)
			if fn.Type.Params != nil {
				for _, param := range fn.Type.Params.List {
					for _, name := range param.Names {
						// Check if param type is *gin.Engine, *gin.RouterGroup, or gin.IRouter etc.
						if isGinRouterType(param.Type) {
							paramVars[name.Name] = prefix
						}
					}
				}
			}

			if len(paramVars) == 0 {
				continue
			}

			fileAliases := buildImportAliases(f)
			relPath, _ := filepath.Rel(projectRoot, filePath)

			return extractFromBlock(fn.Body, paramVars, prefix, fileAliases, resolver, projectRoot, relPath)
		}
	}

	return nil
}

// isGinInit checks if a call is gin.Default() or gin.New().
func isGinInit(call *ast.CallExpr, aliases map[string]string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	// Check if the package is "gin" (by alias or import path)
	if !isGinPackage(pkgIdent.Name, aliases) {
		return false
	}

	return sel.Sel.Name == "Default" || sel.Sel.Name == "New"
}

// isGroupCall checks if a call is r.Group("/prefix").
// Returns (prefix, basePrefix, true) if it is.
func isGroupCall(call *ast.CallExpr, ginVars map[string]string) (string, string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Group" {
		return "", "", false
	}

	varIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return "", "", false
	}

	base, exists := ginVars[varIdent.Name]
	if !exists {
		return "", "", false
	}

	if len(call.Args) < 1 {
		return "", base, true
	}

	prefix := extractStringLit(call.Args[0])
	return prefix, base, true
}

// isHTTPMethod checks if the method name is an HTTP method in Gin.
func isHTTPMethod(name string) bool {
	switch name {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "Any",
		"Handle":
		return true
	}
	return false
}

// isGinPackage checks if the alias refers to the gin package.
func isGinPackage(alias string, aliases map[string]string) bool {
	if alias == "gin" {
		return true
	}
	if importPath, ok := aliases[alias]; ok {
		return strings.Contains(importPath, "gin-gonic/gin") || strings.HasSuffix(importPath, "/gin")
	}
	return false
}

// isGinRouterType checks if a type expression looks like a Gin router type.
// Matches: *gin.Engine, *gin.RouterGroup, gin.IRouter, gin.IRoutes
func isGinRouterType(expr ast.Expr) bool {
	// Unwrap pointer
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}

	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}

	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}

	if pkgIdent.Name != "gin" {
		return false
	}

	switch sel.Sel.Name {
	case "Engine", "RouterGroup", "IRouter", "IRoutes":
		return true
	}
	return false
}

// resolveHandlerFuncID resolves a handler expression to a FuncID.
// Handles:
//
//	controller.Method → pkg.ControllerType.Method
//	pkg.Function      → pkg.Function
//	localFunc         → current pkg function
func resolveHandlerFuncID(expr ast.Expr, aliases map[string]string, resolver *astutil.Resolver, projectRoot string) model.FuncID {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		if pkgOrVar, ok := e.X.(*ast.Ident); ok {
			methodName := e.Sel.Name

			// Check if it's a package function: pkg.Handler
			if importPath, ok := aliases[pkgOrVar.Name]; ok {
				if resolver.IsInternal(importPath) {
					return model.FuncID{
						Pkg:  importPath,
						Name: methodName,
					}
				}
			}

			// Otherwise it's varName.Method → need to resolve varName type
			// We return a partial FuncID; the callgraph builder will resolve it
			return model.FuncID{
				Receiver: pkgOrVar.Name,
				Name:     methodName,
			}
		}

	case *ast.Ident:
		// Local function
		return model.FuncID{
			Name: e.Name,
		}
	}

	return model.FuncID{}
}

func extractStringLit(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	return strings.Trim(lit.Value, `"`)
}

func extractHandlerName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		if ident, ok := e.X.(*ast.Ident); ok {
			return ident.Name + "." + e.Sel.Name
		}
		return e.Sel.Name
	case *ast.Ident:
		return e.Name
	default:
		return "<anonymous>"
	}
}

func buildImportAliases(f *ast.File) map[string]string {
	aliases := make(map[string]string)
	for _, imp := range f.Imports {
		path := strings.Trim(imp.Path.Value, `"`)
		var name string
		if imp.Name != nil {
			name = imp.Name.Name
		} else {
			parts := strings.Split(path, "/")
			name = parts[len(parts)-1]
			name = strings.ReplaceAll(name, "-", "_")
		}
		if name == "_" {
			continue
		}
		aliases[name] = path
	}
	return aliases
}

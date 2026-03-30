package gin

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/wuqiang1985/go-route-impact/internal/astutil"
	"github.com/wuqiang1985/go-route-impact/internal/extractor"
	"github.com/wuqiang1985/go-route-impact/pkg/model"
)

func init() {
	extractor.Register(&GinExtractor{})
}

// GinExtractor extracts routes from Gin projects by scanning all Go files.
//
// Supported patterns:
//
//  1. Direct: r := gin.Default(); r.GET("/path", handler)
//  2. Group:  v1 := r.Group("/api"); v1.GET("/users", handler)
//  3. Struct embed: type App struct { *gin.Engine }; a.Engine.Group(...)
//  4. Method-based: func (a *App) Router() { group := a.Engine.Group(...); group.GET(...) }
type GinExtractor struct{}

func (e *GinExtractor) Name() string { return "gin" }

func (e *GinExtractor) Extract(projectRoot string, entryPoint string, resolver *astutil.Resolver) ([]model.Route, error) {
	// Parse all project files to find Gin route registrations.
	// Gin routes can be registered anywhere (main, methods, init functions),
	// so we scan everything rather than just the entry point.
	parsed, err := astutil.ParseProjectFull(projectRoot, []string{"vendor/", "test/", "testdata/", "docs/"})
	if err != nil {
		return nil, err
	}

	var allRoutes []model.Route

	for _, pf := range parsed {
		aliases := buildImportAliases(pf.File)
		relPath, _ := filepath.Rel(projectRoot, pf.FilePath)
		pkgPath, _ := resolver.FileToPackage(pf.FilePath)

		for _, decl := range pf.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			routes := extractRoutesFromFunc(fn, relPath, pkgPath, aliases, resolver, projectRoot)
			allRoutes = append(allRoutes, routes...)
		}
	}

	return allRoutes, nil
}

// extractRoutesFromFunc extracts Gin routes from a single function/method body.
// It tracks Group() assignments and collects HTTP method registrations.
func extractRoutesFromFunc(fn *ast.FuncDecl, file string, pkgPath string, aliases map[string]string, resolver *astutil.Resolver, projectRoot string) []model.Route {
	if fn.Body == nil {
		return nil
	}

	// groupVars tracks variable → path prefix.
	// Seed with known engine-like variables that have empty prefix.
	groupVars := make(map[string]string)

	// Collect controller variable types for handler FuncID resolution.
	ctrlTypes := make(map[string]ctrlVar)

	var routes []model.Route

	// Two-pass: first collect all group and variable assignments, then collect routes.
	// Pass 1: scan all assignments
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) < 1 || len(assign.Rhs) < 1 {
			return true
		}

		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}

		rhs := assign.Rhs[0]

		// Check for gin.Default() / gin.New()
		if call, ok := rhs.(*ast.CallExpr); ok {
			if isGinInit(call, aliases) {
				groupVars[ident.Name] = ""
				return true
			}
		}

		// Check for x.Engine.Group("/prefix") or x.Group("/prefix")
		if call, ok := rhs.(*ast.CallExpr); ok {
			if prefix, ok := extractGroupPrefix(call, groupVars); ok {
				groupVars[ident.Name] = prefix
				return true
			}
		}

		// Check for &pkg.Type{} controller assignments
		actualRhs := rhs
		if unary, ok := actualRhs.(*ast.UnaryExpr); ok && unary.Op.String() == "&" {
			actualRhs = unary.X
		}
		if comp, ok := actualRhs.(*ast.CompositeLit); ok {
			if sel, ok := comp.Type.(*ast.SelectorExpr); ok {
				if pkgIdent, ok := sel.X.(*ast.Ident); ok {
					importPath := pkgIdent.Name
					if full, exists := aliases[pkgIdent.Name]; exists {
						importPath = full
					}
					ctrlTypes[ident.Name] = ctrlVar{pkg: importPath, typeName: sel.Sel.Name}
				}
			}
			// Local type: Type{}
			if localIdent, ok := comp.Type.(*ast.Ident); ok {
				ctrlTypes[ident.Name] = ctrlVar{pkg: pkgPath, typeName: localIdent.Name}
			}
		}

		return true
	})

	// Pass 2: find all HTTP method calls
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) < 2 {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		methodName := sel.Sel.Name
		if !isHTTPMethod(methodName) {
			return true
		}

		// Determine the path prefix from the receiver
		prefix, found := resolvePrefix(sel.X, groupVars)
		if !found {
			return true
		}

		routePath := extractStringLit(call.Args[0])
		if routePath == "" {
			return true
		}

		fullPath := prefix + routePath
		method := strings.ToUpper(methodName)

		// Handler is the last argument
		handlerExpr := call.Args[len(call.Args)-1]
		handler := extractHandlerName(handlerExpr)
		handlerFuncID := resolveHandlerFuncID(handlerExpr, aliases, ctrlTypes, pkgPath, resolver)

		routes = append(routes, model.Route{
			Method:        method,
			Path:          fullPath,
			Handler:       handler,
			File:          file,
			Package:       pkgPath,
			HandlerFuncID: handlerFuncID,
		})

		return true
	})

	return routes
}

// resolvePrefix resolves the path prefix from a call receiver expression.
// Handles: group (ident), a.Engine (selector), etc.
func resolvePrefix(expr ast.Expr, groupVars map[string]string) (string, bool) {
	switch x := expr.(type) {
	case *ast.Ident:
		// group.GET(...) or r.POST(...)
		if prefix, ok := groupVars[x.Name]; ok {
			return prefix, true
		}

	case *ast.SelectorExpr:
		// a.Engine.GET(...) — the Engine field on a struct that embeds *gin.Engine
		if x.Sel.Name == "Engine" {
			return "", true
		}
	}

	return "", false
}

// extractGroupPrefix extracts the prefix from a Group() call.
// Handles:
//   - group.Group("/prefix")
//   - a.Engine.Group("/prefix")
//   - a.Group("/prefix")   (embedded engine)
func extractGroupPrefix(call *ast.CallExpr, groupVars map[string]string) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Group" {
		return "", false
	}

	prefix := ""
	if len(call.Args) >= 1 {
		prefix = extractStringLit(call.Args[0])
	}

	// Check receiver
	basePrefix, found := resolvePrefix(sel.X, groupVars)
	if found {
		return basePrefix + prefix, true
	}

	// a.Engine.Group() — sel.X is a.Engine (SelectorExpr)
	if inner, ok := sel.X.(*ast.SelectorExpr); ok {
		if inner.Sel.Name == "Engine" {
			return prefix, true
		}
	}

	return "", false
}

type ctrlVar struct {
	pkg      string
	typeName string
}

// resolveHandlerFuncID resolves a handler expression to a FuncID.
func resolveHandlerFuncID(expr ast.Expr, aliases map[string]string, ctrlTypes map[string]ctrlVar, currentPkg string, resolver *astutil.Resolver) model.FuncID {
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		if varIdent, ok := e.X.(*ast.Ident); ok {
			methodName := e.Sel.Name

			// Known controller variable
			if cv, ok := ctrlTypes[varIdent.Name]; ok {
				return model.FuncID{Pkg: cv.pkg, Receiver: cv.typeName, Name: methodName}
			}

			// Package function: pkg.Handler
			if importPath, ok := aliases[varIdent.Name]; ok {
				if resolver.IsInternal(importPath) {
					return model.FuncID{Pkg: importPath, Name: methodName}
				}
			}

			// Fallback: treat as variable.Method
			return model.FuncID{Pkg: currentPkg, Receiver: varIdent.Name, Name: methodName}
		}

	case *ast.Ident:
		return model.FuncID{Pkg: currentPkg, Name: e.Name}
	}

	return model.FuncID{}
}

func isGinInit(call *ast.CallExpr, aliases map[string]string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	if !isGinPackage(pkgIdent.Name, aliases) {
		return false
	}
	return sel.Sel.Name == "Default" || sel.Sel.Name == "New"
}

func isHTTPMethod(name string) bool {
	switch name {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS", "Any", "Handle":
		return true
	}
	return false
}

func isGinPackage(alias string, aliases map[string]string) bool {
	if alias == "gin" {
		return true
	}
	if importPath, ok := aliases[alias]; ok {
		return strings.Contains(importPath, "gin-gonic/gin") || strings.HasSuffix(importPath, "/gin")
	}
	return false
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

package iris

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"github.com/wuqiang1985/go-route-impact/internal/astutil"
	"github.com/wuqiang1985/go-route-impact/internal/extractor"
	"github.com/wuqiang1985/go-route-impact/pkg/model"
)

func init() {
	extractor.Register(&IrisExtractor{})
}

// IrisExtractor extracts routes from Iris v12 projects.
// Enhanced over v1: records HandlerFuncID for each route.
type IrisExtractor struct{}

func (e *IrisExtractor) Name() string { return "iris" }

// Extract parses the entry point (main.go) and extracts all routes.
func (e *IrisExtractor) Extract(projectRoot string, entryPoint string, resolver *astutil.Resolver) ([]model.Route, error) {
	entryPath := filepath.Join(projectRoot, entryPoint)
	f, _, err := astutil.ParseFullFile(entryPath)
	if err != nil {
		return nil, fmt.Errorf("parse entry point %s: %w", entryPoint, err)
	}

	importAliases := buildImportAliases(f)

	// Phase 1: Track variable assignments for Party calls.
	partyVars := make(map[string]string)

	var mainFunc *ast.FuncDecl
	for _, decl := range f.Decls {
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

	for _, stmt := range mainFunc.Body.List {
		// Track: v2 := app.Iris.Party("/api/v2")
		if assign, ok := stmt.(*ast.AssignStmt); ok {
			if len(assign.Lhs) >= 1 && len(assign.Rhs) >= 1 {
				if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
					if prefix := extractTopLevelPartyPrefix(assign.Rhs[0]); prefix != "" {
						partyVars[ident.Name] = prefix
					}
				}
			}
			continue
		}

		// Track: v2.PartyFunc("/business", func(business router.Party) { ... })
		exprStmt, ok := stmt.(*ast.ExprStmt)
		if !ok {
			continue
		}

		call, ok := exprStmt.X.(*ast.CallExpr)
		if !ok {
			continue
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			continue
		}

		if sel.Sel.Name == "PartyFunc" {
			varIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				continue
			}
			varPrefix := partyVars[varIdent.Name]

			if len(call.Args) < 2 {
				continue
			}
			subPrefix := extractStringLit(call.Args[0])
			fullPrefix := varPrefix + subPrefix

			funcLit, ok := call.Args[1].(*ast.FuncLit)
			if !ok {
				continue
			}

			routes = append(routes, extractFromPartyFuncBody(funcLit, fullPrefix, importAliases, resolver, projectRoot)...)
		}
	}

	return routes, nil
}

func extractTopLevelPartyPrefix(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Party" {
		return ""
	}

	if len(call.Args) < 1 {
		return ""
	}

	return extractStringLit(call.Args[0])
}

func extractFromPartyFuncBody(body *ast.FuncLit, parentPrefix string, importAliases map[string]string, resolver *astutil.Resolver, projectRoot string) []model.Route {
	var routes []model.Route

	ast.Inspect(body.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if !isMvcConfigure(call) {
			return true
		}

		if len(call.Args) < 2 {
			return true
		}

		subPrefix := extractPartyPrefix(call.Args[0])
		fullPrefix := parentPrefix + subPrefix

		handlerRoutes := resolveHandlerRoutes(call.Args[1], importAliases, resolver, projectRoot, fullPrefix)
		routes = append(routes, handlerRoutes...)

		return true
	})

	return routes
}

func isMvcConfigure(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Configure" {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	return ok && ident.Name == "mvc"
}

func extractPartyPrefix(expr ast.Expr) string {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return ""
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Party" {
		return ""
	}

	if len(call.Args) < 1 {
		return ""
	}

	return extractStringLit(call.Args[0])
}

func resolveHandlerRoutes(expr ast.Expr, importAliases map[string]string, resolver *astutil.Resolver, projectRoot string, pathPrefix string) []model.Route {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return nil
	}

	pkgAlias := pkgIdent.Name
	funcName := sel.Sel.Name

	importPath, ok := importAliases[pkgAlias]
	if !ok {
		return nil
	}

	pkgDir := resolver.PackageToDir(importPath)
	if pkgDir == "" {
		return nil
	}

	return extractRoutesFromMvcFunc(pkgDir, funcName, importPath, resolver, projectRoot, pathPrefix)
}

// extractRoutesFromMvcFunc parses Go files in pkgDir to find the given function
// and extract route registrations from it. Enhanced to record HandlerFuncID.
func extractRoutesFromMvcFunc(pkgDir string, funcName string, pkgImportPath string, resolver *astutil.Resolver, projectRoot string, pathPrefix string) []model.Route {
	pattern := filepath.Join(pkgDir, "*.go")
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

		// Build import aliases for this file to resolve controller types
		fileAliases := buildImportAliases(f)

		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != funcName {
				continue
			}

			relPath, _ := filepath.Rel(projectRoot, filePath)
			pkgPath, _ := resolver.FileToPackage(filePath)

			// Find controller variable type from function body
			controllerTypes := findControllerTypes(fn.Body, fileAliases)

			return extractRoutesFromFuncBody(fn.Body, relPath, pkgPath, pathPrefix, controllerTypes, resolver)
		}
	}

	return nil
}

// controllerVar holds info about a controller variable used in route registration.
type controllerVar struct {
	varName    string
	pkgPath    string
	typeName   string
}

// findControllerTypes finds controller variable types from the function body.
// Looks for patterns like: controller := &controllers.GameInfoController{Service: service}
func findControllerTypes(body *ast.BlockStmt, aliases map[string]string) map[string]controllerVar {
	result := make(map[string]controllerVar)
	if body == nil {
		return result
	}

	for _, stmt := range body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) < 1 || len(assign.Rhs) < 1 {
			continue
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			continue
		}

		// Check for &pkg.Type{} or pkg.Type{}
		rhs := assign.Rhs[0]
		if unary, ok := rhs.(*ast.UnaryExpr); ok && unary.Op.String() == "&" {
			rhs = unary.X
		}

		if comp, ok := rhs.(*ast.CompositeLit); ok {
			if sel, ok := comp.Type.(*ast.SelectorExpr); ok {
				if pkgIdent, ok := sel.X.(*ast.Ident); ok {
					importPath := pkgIdent.Name
					if fullPath, exists := aliases[pkgIdent.Name]; exists {
						importPath = fullPath
					}
					result[ident.Name] = controllerVar{
						varName:  ident.Name,
						pkgPath:  importPath,
						typeName: sel.Sel.Name,
					}
				}
			}
		}
	}

	return result
}

// extractRoutesFromFuncBody extracts route definitions from a function body.
// Enhanced: records HandlerFuncID for each route.
func extractRoutesFromFuncBody(body *ast.BlockStmt, file string, pkgPath string, pathPrefix string, controllerTypes map[string]controllerVar, resolver *astutil.Resolver) []model.Route {
	if body == nil {
		return nil
	}

	routerVars := collectRouterVars(body)

	var routes []model.Route
	httpMethods := map[string]bool{
		"Get": true, "Post": true, "Put": true, "Delete": true,
		"Patch": true, "Head": true, "Options": true, "Any": true,
		"Handle": true,
	}

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		methodName := sel.Sel.Name
		if !httpMethods[methodName] {
			return true
		}

		if !isRouterCall(sel.X) && !isRouterVar(sel.X, routerVars) {
			return true
		}

		if len(call.Args) < 2 {
			return true
		}

		routePath := extractStringLit(call.Args[0])
		if routePath == "" {
			return true
		}

		handler := extractHandlerName(call.Args[1])
		method := strings.ToUpper(methodName)
		fullPath := pathPrefix + routePath

		// Build HandlerFuncID from the handler expression
		handlerFuncID := resolveHandlerFuncID(call.Args[1], controllerTypes, pkgPath, resolver)

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

// resolveHandlerFuncID resolves the handler expression to a FuncID.
// Handles: controller.GetEnvConfig → controllers.GameInfoController.GetEnvConfig
func resolveHandlerFuncID(expr ast.Expr, controllerTypes map[string]controllerVar, currentPkg string, resolver *astutil.Resolver) model.FuncID {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return model.FuncID{}
	}

	methodName := sel.Sel.Name

	// Check if the receiver is a known controller variable
	if ident, ok := sel.X.(*ast.Ident); ok {
		if cv, exists := controllerTypes[ident.Name]; exists {
			return model.FuncID{
				Pkg:      cv.pkgPath,
				Receiver: cv.typeName,
				Name:     methodName,
			}
		}

		// Fallback: use the variable name as a hint
		return model.FuncID{
			Pkg:      currentPkg,
			Receiver: ident.Name,
			Name:     methodName,
		}
	}

	return model.FuncID{}
}

func collectRouterVars(body *ast.BlockStmt) map[string]bool {
	vars := make(map[string]bool)
	for _, stmt := range body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) < 1 || len(assign.Rhs) < 1 {
			continue
		}
		ident, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			continue
		}
		sel, ok := assign.Rhs[0].(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Router" {
			continue
		}
		vars[ident.Name] = true
	}
	return vars
}

func isRouterVar(expr ast.Expr, routerVars map[string]bool) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && routerVars[ident.Name]
}

func isRouterCall(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if ok && sel.Sel.Name == "Router" {
		return true
	}
	ident, ok := expr.(*ast.Ident)
	if ok && (ident.Name == "router" || ident.Name == "Router") {
		return true
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

package callgraph

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/pnt-team/go-route-impact-v2/internal/astutil"
	"github.com/pnt-team/go-route-impact-v2/internal/typeinfer"
	"github.com/pnt-team/go-route-impact-v2/pkg/model"
)

// Builder constructs a function-level call graph from parsed Go files.
type Builder struct {
	resolver *astutil.Resolver
	// structFields: pkgPath → structName → fieldName → TypeInfo
	structFields map[string]map[string]map[string]typeinfer.TypeInfo
	// funcReturns: funcKey → TypeInfo (return type)
	funcReturns map[string]typeinfer.TypeInfo
	// allFuncs: funcKey → FuncLocation
	allFuncs map[string]model.FuncLocation
	// importAliases per file: filePath → alias → importPath
	fileImportAliases map[string]map[string]string
}

// NewBuilder creates a new call graph builder.
func NewBuilder(resolver *astutil.Resolver) *Builder {
	return &Builder{
		resolver:          resolver,
		structFields:      make(map[string]map[string]map[string]typeinfer.TypeInfo),
		funcReturns:       make(map[string]typeinfer.TypeInfo),
		allFuncs:          make(map[string]model.FuncLocation),
		fileImportAliases: make(map[string]map[string]string),
	}
}

// Build constructs the call graph from all parsed files.
func (b *Builder) Build(files []astutil.ParsedFile) *Graph {
	g := NewGraph()

	// Phase 1: Collect all type information (struct fields, function signatures)
	b.collectTypeInfo(files)

	// Phase 2: For each function, extract call targets and build edges
	for _, pf := range files {
		pkgPath, err := b.resolver.FileToPackage(pf.FilePath)
		if err != nil {
			continue
		}

		relPath, _ := b.resolver.RelPath(pf.FilePath)
		aliases := b.fileImportAliases[pf.FilePath]

		for _, decl := range pf.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}

			receiver := astutil.ReceiverTypeName(fn)
			callerID := model.FuncID{
				Pkg:      pkgPath,
				Receiver: receiver,
				Name:     fn.Name.Name,
			}

			startLine := pf.Fset.Position(fn.Pos()).Line
			endLine := pf.Fset.Position(fn.End()).Line

			loc := model.FuncLocation{
				FuncID:    callerID,
				File:      relPath,
				StartLine: startLine,
				EndLine:   endLine,
			}
			g.AddFunc(callerID, loc)

			// Extract all calls in this function body
			callees := b.extractCalls(fn, pkgPath, receiver, aliases, pf.Fset)
			for _, calleeID := range callees {
				if !calleeID.IsZero() {
					g.AddEdge(callerID, calleeID)
				}
			}
		}
	}

	// Phase 3: Resolve interface method calls to concrete implementations.
	// When an edge points to pkg.InterfaceName.Method but no such func exists,
	// find concrete types in the same package that implement that method.
	b.resolveInterfaceEdges(g)

	return g
}

// resolveInterfaceEdges finds edges pointing to interface methods and redirects
// them to concrete implementations. This handles the common Go pattern:
//
//	type Controller struct { Service SomeInterface }
//	func (c *Controller) Handle() { c.Service.DoSomething() }
//
// Where DoSomething is defined on a concrete struct (lowercase type).
func (b *Builder) resolveInterfaceEdges(g *Graph) {
	// Collect all edges that point to non-existent FuncIDs
	type pendingEdge struct {
		caller model.FuncID
		callee model.FuncID
	}
	var pending []pendingEdge

	for callerKey, callees := range g.Forward {
		callerLoc, ok := g.Funcs[callerKey]
		if !ok {
			continue
		}
		for _, callee := range callees {
			calleeKey := callee.Key()
			if _, exists := g.Funcs[calleeKey]; !exists && callee.Receiver != "" {
				pending = append(pending, pendingEdge{
					caller: callerLoc.FuncID,
					callee: callee,
				})
			}
		}
	}

	for _, pe := range pending {
		// Strategy 1: Same package, find concrete type with this method.
		// e.g., services.GameInfoService.GetEnvConfig → services.gameInfoService.GetEnvConfig
		resolved := b.findConcreteImplementation(g, pe.callee)
		for _, concrete := range resolved {
			g.AddEdge(pe.caller, concrete)
		}
	}
}

// findConcreteImplementation finds concrete method implementations for an interface method call.
func (b *Builder) findConcreteImplementation(g *Graph, interfaceCall model.FuncID) []model.FuncID {
	var results []model.FuncID

	// Look for any function in the same package with the same method name
	// but a different (concrete) receiver type
	for _, loc := range g.Funcs {
		if loc.FuncID.Pkg == interfaceCall.Pkg &&
			loc.FuncID.Name == interfaceCall.Name &&
			loc.FuncID.Receiver != "" &&
			loc.FuncID.Receiver != interfaceCall.Receiver {
			results = append(results, loc.FuncID)
		}
	}

	// If not found in same package, search more broadly by method name
	// (handles cross-package interface implementations)
	if len(results) == 0 && isExportedName(interfaceCall.Name) {
		for _, loc := range g.Funcs {
			if loc.FuncID.Name == interfaceCall.Name &&
				loc.FuncID.Receiver != "" &&
				loc.FuncID.Key() != interfaceCall.Key() &&
				b.resolver.IsInternal(loc.FuncID.Pkg) {
				results = append(results, loc.FuncID)
			}
		}
	}

	return results
}

// collectTypeInfo collects struct field types and function return types.
func (b *Builder) collectTypeInfo(files []astutil.ParsedFile) {
	for _, pf := range files {
		pkgPath, err := b.resolver.FileToPackage(pf.FilePath)
		if err != nil {
			continue
		}

		// Build import aliases for this file
		aliases := typeinfer.BuildImportAliases(pf.File)
		b.fileImportAliases[pf.FilePath] = aliases

		// Collect struct field types
		structFields := typeinfer.StructFieldTypes(pf.File)
		if len(structFields) > 0 {
			if b.structFields[pkgPath] == nil {
				b.structFields[pkgPath] = make(map[string]map[string]typeinfer.TypeInfo)
			}
			for structName, fields := range structFields {
				b.structFields[pkgPath][structName] = fields
			}
		}

		// Collect function return types
		for _, decl := range pf.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			receiver := astutil.ReceiverTypeName(fn)
			funcID := model.FuncID{Pkg: pkgPath, Receiver: receiver, Name: fn.Name.Name}
			retType := typeinfer.FuncReturnType(fn)
			if retType.TypeName != "" {
				// Resolve alias to full import path
				if retType.Package != "" {
					if fullPath, ok := aliases[retType.Package]; ok {
						retType.Package = fullPath
					}
				} else {
					retType.Package = pkgPath
				}
				b.funcReturns[funcID.Key()] = retType
			}

			relPath, _ := b.resolver.RelPath(pf.FilePath)
			b.allFuncs[funcID.Key()] = model.FuncLocation{
				FuncID: funcID,
				File:   relPath,
			}
		}
	}
}

// extractCalls extracts all function/method calls from a function body.
func (b *Builder) extractCalls(fn *ast.FuncDecl, pkgPath string, receiverType string, aliases map[string]string, fset *token.FileSet) []model.FuncID {
	if fn.Body == nil {
		return nil
	}

	var callees []model.FuncID
	seen := make(map[string]bool)

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var calleeID model.FuncID

		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			calleeID = b.resolveSelectorCall(fun, fn, pkgPath, receiverType, aliases)

		case *ast.Ident:
			// Local function call: localFunc()
			calleeID = model.FuncID{
				Pkg:  pkgPath,
				Name: fun.Name,
			}
		}

		if !calleeID.IsZero() {
			key := calleeID.Key()
			if !seen[key] {
				seen[key] = true
				callees = append(callees, calleeID)
			}
		}

		return true
	})

	return callees
}

// resolveSelectorCall resolves a selector expression call like x.Method() or pkg.Func().
func (b *Builder) resolveSelectorCall(sel *ast.SelectorExpr, fn *ast.FuncDecl, pkgPath string, receiverType string, aliases map[string]string) model.FuncID {
	methodName := sel.Sel.Name

	switch x := sel.X.(type) {
	case *ast.Ident:
		// Case 1: pkg.Func() — package-level function call
		if importPath, ok := aliases[x.Name]; ok {
			if b.resolver.IsInternal(importPath) {
				return model.FuncID{Pkg: importPath, Name: methodName}
			}
			return model.FuncID{} // external package, skip
		}

		// Case 2: r.Method() — receiver method call (r is a param)
		// Check if x is the receiver parameter
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			for _, field := range fn.Recv.List {
				for _, name := range field.Names {
					if name.Name == x.Name {
						// r.Method() → same package, same receiver type
						return model.FuncID{
							Pkg:      pkgPath,
							Receiver: receiverType,
							Name:     methodName,
						}
					}
				}
			}
		}

		// Case 3: localVar.Method() — method on a local variable
		// Try to infer the type of localVar
		return b.resolveLocalVarMethod(x.Name, methodName, fn, pkgPath, aliases)

	case *ast.SelectorExpr:
		// Case 4: r.Field.Method() — chained field access
		return b.resolveChainedCall(x, methodName, fn, pkgPath, receiverType, aliases)
	}

	return model.FuncID{}
}

// resolveLocalVarMethod tries to resolve a method call on a local variable.
func (b *Builder) resolveLocalVarMethod(varName string, methodName string, fn *ast.FuncDecl, pkgPath string, aliases map[string]string) model.FuncID {
	if fn.Body == nil {
		return model.FuncID{}
	}

	// Check function parameters
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			for _, name := range param.Names {
				if name.Name == varName {
					ti := typeinfer.TypeInfo{}
					switch pt := param.Type.(type) {
					case *ast.SelectorExpr:
						if pkgIdent, ok := pt.X.(*ast.Ident); ok {
							ti.Package = resolveAlias(pkgIdent.Name, aliases)
							ti.TypeName = pt.Sel.Name
						}
					case *ast.StarExpr:
						if sel, ok := pt.X.(*ast.SelectorExpr); ok {
							if pkgIdent, ok := sel.X.(*ast.Ident); ok {
								ti.Package = resolveAlias(pkgIdent.Name, aliases)
								ti.TypeName = sel.Sel.Name
							}
						}
					case *ast.Ident:
						ti.Package = pkgPath
						ti.TypeName = pt.Name
					}

					if ti.TypeName != "" {
						targetPkg := ti.Package
						if targetPkg == "" {
							targetPkg = pkgPath
						}
						return model.FuncID{
							Pkg:      targetPkg,
							Receiver: ti.TypeName,
							Name:     methodName,
						}
					}
				}
			}
		}
	}

	// Scan function body for variable assignments
	varTypes := typeinfer.VarAssignments(fn.Body, aliases)
	if ti, ok := varTypes[varName]; ok && ti.TypeName != "" {
		targetPkg := ti.Package
		if targetPkg == "" {
			targetPkg = pkgPath
		}
		return model.FuncID{
			Pkg:      targetPkg,
			Receiver: ti.TypeName,
			Name:     methodName,
		}
	}

	return model.FuncID{}
}

// resolveChainedCall resolves calls like r.Service.Method().
func (b *Builder) resolveChainedCall(sel *ast.SelectorExpr, methodName string, fn *ast.FuncDecl, pkgPath string, receiverType string, aliases map[string]string) model.FuncID {
	fieldName := sel.Sel.Name

	// Try to identify the base expression
	baseIdent, ok := sel.X.(*ast.Ident)
	if !ok {
		return model.FuncID{}
	}

	// Check if base is the receiver parameter
	baseType := ""
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		for _, field := range fn.Recv.List {
			for _, name := range field.Names {
				if name.Name == baseIdent.Name {
					baseType = receiverType
				}
			}
		}
	}

	if baseType == "" {
		// Try local var types
		varTypes := typeinfer.VarAssignments(fn.Body, aliases)
		if ti, ok := varTypes[baseIdent.Name]; ok {
			baseType = ti.TypeName
		}
	}

	if baseType == "" {
		return model.FuncID{}
	}

	// Look up the struct field type
	if pkgFields, ok := b.structFields[pkgPath]; ok {
		if fields, ok := pkgFields[baseType]; ok {
			if fieldType, ok := fields[fieldName]; ok {
				targetPkg := fieldType.Package
				if targetPkg == "" {
					targetPkg = pkgPath
				}
				return model.FuncID{
					Pkg:      targetPkg,
					Receiver: fieldType.TypeName,
					Name:     methodName,
				}
			}
		}
	}

	// Try all packages for the struct type
	for pkg, pkgFields := range b.structFields {
		if fields, ok := pkgFields[baseType]; ok {
			if fieldType, ok := fields[fieldName]; ok {
				targetPkg := fieldType.Package
				if targetPkg == "" {
					targetPkg = pkg
				}
				return model.FuncID{
					Pkg:      targetPkg,
					Receiver: fieldType.TypeName,
					Name:     methodName,
				}
			}
		}
	}

	return model.FuncID{}
}

// resolveAlias resolves a package alias to its import path.
func resolveAlias(alias string, aliases map[string]string) string {
	if path, ok := aliases[alias]; ok {
		return path
	}
	return alias
}

// isInterfaceMethod checks if the method is an interface method.
// For now we use a heuristic: if receiver type name starts with uppercase
// and no struct definition is found, it might be an interface.
func isExportedName(name string) bool {
	return len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z'
}

// fallbackMethodLookup tries to find a method by name across all known functions.
// Used as a last resort when type inference fails.
func (b *Builder) fallbackMethodLookup(methodName string) model.FuncID {
	if !isExportedName(methodName) {
		return model.FuncID{}
	}

	var candidates []model.FuncID
	for _, loc := range b.allFuncs {
		if loc.FuncID.Name == methodName && loc.FuncID.Receiver != "" {
			candidates = append(candidates, loc.FuncID)
		}
	}

	// Only use fallback if there's exactly one match (unambiguous)
	if len(candidates) == 1 {
		return candidates[0]
	}

	return model.FuncID{}
}

// resolveInterfaceMethod attempts to resolve an interface method call.
// In Go projects, interfaces are often satisfied by a single concrete type.
// We look for any type that has this method name.
func (b *Builder) resolveInterfaceMethod(interfacePkg string, interfaceName string, methodName string) []model.FuncID {
	var results []model.FuncID

	for key, loc := range b.allFuncs {
		if loc.FuncID.Name == methodName && loc.FuncID.Receiver != "" {
			_ = key
			// Check if the receiver's package could be a concrete implementation
			if strings.Contains(loc.FuncID.Pkg, interfacePkg) || b.resolver.IsInternal(loc.FuncID.Pkg) {
				results = append(results, loc.FuncID)
			}
		}
	}

	return results
}

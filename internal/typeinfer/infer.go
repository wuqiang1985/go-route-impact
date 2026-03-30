package typeinfer

import (
	"go/ast"
	"strings"
)

// TypeInfo holds inferred type information for a variable or field.
type TypeInfo struct {
	// Package is the import path of the type's package.
	Package string
	// TypeName is the type name (e.g., "gameInfoService").
	TypeName string
}

// StructFieldTypes extracts field name → type info for all struct types in a file.
// e.g., for type GameInfoController struct { Service *services.GameInfoService }
// returns {"GameInfoController": {"Service": TypeInfo{Package:"services", TypeName:"GameInfoService"}}}
func StructFieldTypes(file *ast.File) map[string]map[string]TypeInfo {
	result := make(map[string]map[string]TypeInfo)

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}

			fields := make(map[string]TypeInfo)
			for _, field := range structType.Fields.List {
				if len(field.Names) == 0 {
					continue
				}
				fieldName := field.Names[0].Name
				ti := resolveTypeExpr(field.Type)
				if ti.TypeName != "" {
					fields[fieldName] = ti
				}
			}

			if len(fields) > 0 {
				result[typeSpec.Name.Name] = fields
			}
		}
	}

	return result
}

// FuncReturnType infers the return type of a function from its signature.
// Handles patterns like: func NewGameInfoService(...) *GameInfoService
func FuncReturnType(fn *ast.FuncDecl) TypeInfo {
	if fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
		return TypeInfo{}
	}

	// Look at the first return value
	return resolveTypeExpr(fn.Type.Results.List[0].Type)
}

// VarAssignments scans a function body for variable assignments and infers types.
// Handles patterns like:
//   - service := services.NewGameInfoService(repos)   → service: services.GameInfoService (from return type)
//   - controller := &controllers.GameInfoController{} → controller: controllers.GameInfoController
func VarAssignments(body *ast.BlockStmt, importAliases map[string]string) map[string]TypeInfo {
	result := make(map[string]TypeInfo)
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

		ti := inferExprType(assign.Rhs[0], importAliases)
		if ti.TypeName != "" {
			result[ident.Name] = ti
		}
	}

	return result
}

// inferExprType infers the type of an expression.
func inferExprType(expr ast.Expr, importAliases map[string]string) TypeInfo {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		// &controllers.GameInfoController{...}
		if e.Op.String() == "&" {
			return inferExprType(e.X, importAliases)
		}

	case *ast.CompositeLit:
		// controllers.GameInfoController{...}
		return resolveTypeExpr(e.Type)

	case *ast.CallExpr:
		// services.NewGameInfoService(repos)
		// The type is the return type of the function, but we can infer from
		// the naming convention: New<Type> returns *<Type>
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			if pkgIdent, ok := sel.X.(*ast.Ident); ok {
				funcName := sel.Sel.Name
				pkgAlias := pkgIdent.Name

				// Convention: NewXxx returns Xxx or *Xxx
				if strings.HasPrefix(funcName, "New") {
					typeName := funcName[3:] // strip "New"
					// lowercase first char for the actual type
					if len(typeName) > 0 {
						return TypeInfo{
							Package:  resolveAlias(pkgAlias, importAliases),
							TypeName: strings.ToLower(typeName[:1]) + typeName[1:],
						}
					}
				}

				return TypeInfo{
					Package:  resolveAlias(pkgAlias, importAliases),
					TypeName: "", // unknown return type
				}
			}
		}
	}

	return TypeInfo{}
}

// resolveTypeExpr extracts type info from a type expression.
func resolveTypeExpr(expr ast.Expr) TypeInfo {
	if expr == nil {
		return TypeInfo{}
	}

	// Unwrap pointer
	if star, ok := expr.(*ast.StarExpr); ok {
		return resolveTypeExpr(star.X)
	}

	// pkg.Type
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		if pkgIdent, ok := sel.X.(*ast.Ident); ok {
			return TypeInfo{
				Package:  pkgIdent.Name, // alias, needs resolution later
				TypeName: sel.Sel.Name,
			}
		}
	}

	// Local type
	if ident, ok := expr.(*ast.Ident); ok {
		return TypeInfo{
			TypeName: ident.Name,
		}
	}

	return TypeInfo{}
}

// resolveAlias resolves a package alias to its import path.
func resolveAlias(alias string, importAliases map[string]string) string {
	if path, ok := importAliases[alias]; ok {
		return path
	}
	return alias
}

// BuildImportAliases builds a map from import alias/name → import path.
func BuildImportAliases(f *ast.File) map[string]string {
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

		if name == "_" || name == "." {
			continue
		}

		aliases[name] = path
	}

	return aliases
}

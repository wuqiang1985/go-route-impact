package astutil

import (
	"go/ast"
	"go/token"
)

// LocateFunc finds the FuncDecl that contains the given line number.
// Returns nil if no function contains that line.
func LocateFunc(file *ast.File, fset *token.FileSet, line int) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		startLine := fset.Position(fn.Pos()).Line
		endLine := fset.Position(fn.End()).Line

		if line >= startLine && line <= endLine {
			return fn
		}
	}
	return nil
}

// FuncRange returns the start and end lines of a FuncDecl.
func FuncRange(fn *ast.FuncDecl, fset *token.FileSet) (start, end int) {
	return fset.Position(fn.Pos()).Line, fset.Position(fn.End()).Line
}

// AllFuncRanges returns all function declarations in a file with their line ranges.
type FuncRange2 struct {
	Decl      *ast.FuncDecl
	StartLine int
	EndLine   int
}

// AllFuncRanges returns all function declarations with their line ranges.
func AllFuncRanges(file *ast.File, fset *token.FileSet) []FuncRange2 {
	var ranges []FuncRange2
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		start := fset.Position(fn.Pos()).Line
		end := fset.Position(fn.End()).Line
		ranges = append(ranges, FuncRange2{Decl: fn, StartLine: start, EndLine: end})
	}
	return ranges
}

// ReceiverTypeName extracts the receiver type name from a FuncDecl.
// Returns empty string for package-level functions.
func ReceiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}

	recv := fn.Recv.List[0].Type

	// Handle pointer receiver *T
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}

	// Handle indexed type (generic) T[X]
	if idx, ok := recv.(*ast.IndexExpr); ok {
		recv = idx.X
	}

	if ident, ok := recv.(*ast.Ident); ok {
		return ident.Name
	}

	return ""
}

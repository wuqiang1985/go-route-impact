package model

// FuncID uniquely identifies a function or method in the project.
// For methods: Pkg="services", Receiver="gameInfoService", Name="GetEnvConfig"
// For functions: Pkg="utils", Receiver="", Name="FormatDate"
type FuncID struct {
	// Pkg is the Go import path of the package.
	Pkg string `json:"pkg"`
	// Receiver is the receiver type name (empty for package-level functions).
	Receiver string `json:"receiver,omitempty"`
	// Name is the function/method name.
	Name string `json:"name"`
}

// String returns a human-readable representation like "services.gameInfoService.GetEnvConfig".
func (f FuncID) String() string {
	// Use short package name (last segment)
	shortPkg := f.Pkg
	for i := len(f.Pkg) - 1; i >= 0; i-- {
		if f.Pkg[i] == '/' {
			shortPkg = f.Pkg[i+1:]
			break
		}
	}

	if f.Receiver != "" {
		return shortPkg + "." + f.Receiver + "." + f.Name
	}
	return shortPkg + "." + f.Name
}

// Key returns a unique key for map lookups.
func (f FuncID) Key() string {
	if f.Receiver != "" {
		return f.Pkg + "." + f.Receiver + "." + f.Name
	}
	return f.Pkg + "." + f.Name
}

// IsZero returns true if the FuncID is empty.
func (f FuncID) IsZero() bool {
	return f.Pkg == "" && f.Name == ""
}

// Route represents a single HTTP route extracted from the project.
type Route struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Handler string `json:"handler"`
	// File is the Go source file where the handler is defined.
	File string `json:"file"`
	// Package is the Go package path of the handler file.
	Package string `json:"package"`
	// HandlerFuncID is the FuncID of the controller method handling this route.
	HandlerFuncID FuncID `json:"handler_func_id"`
}

// FuncLocation records where a function is defined in source code.
type FuncLocation struct {
	FuncID    FuncID `json:"func_id"`
	File      string `json:"file"`       // project-relative path
	StartLine int    `json:"start_line"` // 1-based
	EndLine   int    `json:"end_line"`   // 1-based
}

// CallChain represents the call chain from a changed function to a route.
type CallChain struct {
	// Chain is the list of FuncIDs from changed function → ... → controller method.
	Chain []FuncID `json:"chain"`
	// Route is the HTTP route at the end of the chain.
	Route Route `json:"route"`
}

// ImpactResult describes the impact of changed functions on routes.
type ImpactResult struct {
	// ChangedFuncs lists the functions that were modified.
	ChangedFuncs []FuncLocation `json:"changed_funcs"`
	// AffectedRoutes lists all routes reachable from the changed functions.
	AffectedRoutes []CallChain `json:"affected_routes"`
}

package callgraph

import (
	"github.com/pnt-team/go-route-impact-v2/pkg/model"
)

// Graph represents a function-level call graph.
type Graph struct {
	// Forward maps caller FuncID key → list of callee FuncIDs.
	Forward map[string][]model.FuncID
	// Reverse maps callee FuncID key → list of caller FuncIDs.
	Reverse map[string][]model.FuncID
	// Funcs maps FuncID key → FuncLocation.
	Funcs map[string]model.FuncLocation
}

// NewGraph creates an empty call graph.
func NewGraph() *Graph {
	return &Graph{
		Forward: make(map[string][]model.FuncID),
		Reverse: make(map[string][]model.FuncID),
		Funcs:   make(map[string]model.FuncLocation),
	}
}

// AddFunc registers a function in the graph.
func (g *Graph) AddFunc(id model.FuncID, loc model.FuncLocation) {
	g.Funcs[id.Key()] = loc
}

// AddEdge adds a caller → callee edge to the graph.
func (g *Graph) AddEdge(caller, callee model.FuncID) {
	callerKey := caller.Key()
	calleeKey := callee.Key()

	// Deduplicate
	for _, existing := range g.Forward[callerKey] {
		if existing.Key() == calleeKey {
			return
		}
	}

	g.Forward[callerKey] = append(g.Forward[callerKey], callee)
	g.Reverse[calleeKey] = append(g.Reverse[calleeKey], caller)
}

// CallersBFS performs a BFS traversal of the reverse graph starting from the given FuncID.
// It returns all reachable FuncIDs in the call chain (callers of callers...).
// stopPredicate, if non-nil, is called on each reached FuncID. If it returns true,
// that FuncID is collected but BFS does not continue past it.
func (g *Graph) CallersBFS(start model.FuncID, stopPredicate func(model.FuncID) bool) [][]model.FuncID {
	type path struct {
		chain []model.FuncID
	}

	var results [][]model.FuncID
	visited := make(map[string]bool)
	startKey := start.Key()
	visited[startKey] = true

	queue := []path{{chain: []model.FuncID{start}}}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		lastFuncID := current.chain[len(current.chain)-1]
		lastKey := lastFuncID.Key()

		callers := g.Reverse[lastKey]
		if len(callers) == 0 {
			// Terminal node (no more callers), include if chain > 1
			if len(current.chain) > 1 {
				results = append(results, current.chain)
			}
			continue
		}

		for _, caller := range callers {
			callerKey := caller.Key()
			if visited[callerKey] {
				continue
			}
			visited[callerKey] = true

			newChain := make([]model.FuncID, len(current.chain)+1)
			copy(newChain, current.chain)
			newChain[len(current.chain)] = caller

			if stopPredicate != nil && stopPredicate(caller) {
				// Reached a stop node (e.g., controller method), collect the chain
				results = append(results, newChain)
			} else {
				queue = append(queue, path{chain: newChain})
			}
		}
	}

	return results
}

// Callers returns direct callers of the given FuncID.
func (g *Graph) Callers(id model.FuncID) []model.FuncID {
	return g.Reverse[id.Key()]
}

// Callees returns direct callees of the given FuncID.
func (g *Graph) Callees(id model.FuncID) []model.FuncID {
	return g.Forward[id.Key()]
}

// LookupFunc returns the FuncLocation for a FuncID, if known.
func (g *Graph) LookupFunc(id model.FuncID) (model.FuncLocation, bool) {
	loc, ok := g.Funcs[id.Key()]
	return loc, ok
}

// FindFunc searches for a FuncID by partial match (short name format).
// shortName can be: "services.gameInfoService.GetEnvConfig" or "GetEnvConfig"
func (g *Graph) FindFunc(shortName string) []model.FuncID {
	var results []model.FuncID

	for _, loc := range g.Funcs {
		if loc.FuncID.String() == shortName || loc.FuncID.Key() == shortName || loc.FuncID.Name == shortName {
			results = append(results, loc.FuncID)
		}
	}

	return results
}

// Stats returns statistics about the graph.
func (g *Graph) Stats() (funcs, edges int) {
	funcs = len(g.Funcs)
	for _, callees := range g.Forward {
		edges += len(callees)
	}
	return
}

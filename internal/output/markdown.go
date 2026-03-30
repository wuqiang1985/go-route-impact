package output

import (
	"fmt"
	"io"

	"github.com/wuqiang1985/go-route-impact/pkg/model"
)

// PrintRoutesMarkdown writes routes as a Markdown table.
func PrintRoutesMarkdown(w io.Writer, routes []model.Route) {
	fmt.Fprintln(w, "| Method | Path | Handler |")
	fmt.Fprintln(w, "|--------|------|---------|")

	for _, r := range routes {
		fmt.Fprintf(w, "| %s | %s | %s |\n", r.Method, r.Path, r.Handler)
	}

	fmt.Fprintf(w, "\nTotal: %d routes\n", len(routes))
}

// PrintImpactMarkdown writes impact result as Markdown.
func PrintImpactMarkdown(w io.Writer, result *model.ImpactResult) {
	if len(result.ChangedFuncs) > 0 {
		fmt.Fprintf(w, "## Changed Functions (%d)\n\n", len(result.ChangedFuncs))
		for _, f := range result.ChangedFuncs {
			fmt.Fprintf(w, "- `%s`", f.FuncID.String())
			if f.File != "" {
				fmt.Fprintf(w, " (%s:%d-%d)", f.File, f.StartLine, f.EndLine)
			}
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w)
	}

	if len(result.AffectedRoutes) == 0 {
		fmt.Fprintln(w, "No routes affected.")
		return
	}

	// Deduplicate routes
	routeSet := make(map[string]bool)
	var uniqueRoutes []model.Route
	for _, cc := range result.AffectedRoutes {
		key := cc.Route.Method + " " + cc.Route.Path
		if !routeSet[key] {
			routeSet[key] = true
			uniqueRoutes = append(uniqueRoutes, cc.Route)
		}
	}

	fmt.Fprintf(w, "## Affected Routes (%d)\n\n", len(uniqueRoutes))

	// Call chains
	fmt.Fprintln(w, "### Call Chains")
	for _, cc := range result.AffectedRoutes {
		parts := make([]string, len(cc.Chain))
		for i, f := range cc.Chain {
			parts[i] = "`" + f.String() + "`"
		}
		fmt.Fprintf(w, "- %s → `%s %s`\n",
			chainToString(cc.Chain), cc.Route.Method, cc.Route.Path)
	}
	fmt.Fprintln(w)

	// Routes table
	PrintRoutesMarkdown(w, uniqueRoutes)
}

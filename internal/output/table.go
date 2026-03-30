package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/pnt-team/go-route-impact-v2/pkg/model"
)

var (
	headerColor  = color.New(color.FgCyan, color.Bold)
	methodGet    = color.New(color.FgGreen)
	methodPost   = color.New(color.FgYellow)
	methodPut    = color.New(color.FgBlue)
	methodDelete = color.New(color.FgRed)
	chainColor   = color.New(color.FgHiBlack)
	funcColor    = color.New(color.FgMagenta)
	routeColor   = color.New(color.FgCyan)
)

// PrintRouteTable prints routes in a table format.
func PrintRouteTable(w io.Writer, routes []model.Route) {
	if len(routes) == 0 {
		fmt.Fprintln(w, "No routes found.")
		return
	}

	methodW, pathW, handlerW := 6, 4, 7
	for _, r := range routes {
		if len(r.Method) > methodW {
			methodW = len(r.Method)
		}
		if len(r.Path) > pathW {
			pathW = len(r.Path)
		}
		if len(r.Handler) > handlerW {
			handlerW = len(r.Handler)
		}
	}

	if pathW > 60 {
		pathW = 60
	}

	sep := fmt.Sprintf("├%s┼%s┼%s┤",
		strings.Repeat("─", methodW+2),
		strings.Repeat("─", pathW+2),
		strings.Repeat("─", handlerW+2))

	topBorder := fmt.Sprintf("┌%s┬%s┬%s┐",
		strings.Repeat("─", methodW+2),
		strings.Repeat("─", pathW+2),
		strings.Repeat("─", handlerW+2))

	bottomBorder := fmt.Sprintf("└%s┴%s┴%s┘",
		strings.Repeat("─", methodW+2),
		strings.Repeat("─", pathW+2),
		strings.Repeat("─", handlerW+2))

	fmt.Fprintln(w, topBorder)
	fmt.Fprintf(w, "│ %-*s │ %-*s │ %-*s │\n",
		methodW, "METHOD", pathW, "PATH", handlerW, "HANDLER")
	fmt.Fprintln(w, sep)

	for _, r := range routes {
		method := padRight(r.Method, methodW)
		path := padRight(truncate(r.Path, pathW), pathW)
		handler := padRight(r.Handler, handlerW)

		fmt.Fprintf(w, "│ %s │ %s │ %s │\n",
			colorMethod(method), path, handler)
	}

	fmt.Fprintln(w, bottomBorder)
	fmt.Fprintf(w, "\nTotal: %d routes\n", len(routes))
}

// PrintImpactResult prints the function-level impact analysis result.
func PrintImpactResult(w io.Writer, result *model.ImpactResult) {
	if len(result.ChangedFuncs) > 0 {
		headerColor.Fprintf(w, "\nChanged functions (%d):\n", len(result.ChangedFuncs))
		for _, f := range result.ChangedFuncs {
			loc := ""
			if f.File != "" {
				loc = fmt.Sprintf(" (%s:%d-%d)", f.File, f.StartLine, f.EndLine)
			}
			funcColor.Fprintf(w, "  • %s", f.FuncID.String())
			chainColor.Fprintf(w, "%s\n", loc)
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

	headerColor.Fprintf(w, "📋 Function Impact (%d function(s) → %d route(s))\n\n",
		len(result.ChangedFuncs), len(uniqueRoutes))

	// Print call chains
	fmt.Fprintln(w, "Call Chain(s):")
	printed := make(map[string]bool)
	for _, cc := range result.AffectedRoutes {
		chainKey := chainToString(cc.Chain)
		routeStr := cc.Route.Method + " " + cc.Route.Path
		fullKey := chainKey + " → " + routeStr
		if printed[fullKey] {
			continue
		}
		printed[fullKey] = true

		// Print chain
		for i, fid := range cc.Chain {
			indent := strings.Repeat("  ", i)
			prefix := "  "
			if i > 0 {
				prefix = indent + "← "
			}
			funcColor.Fprintf(w, "%s%s\n", prefix, fid.String())
		}
		// Print route at the end
		indent := strings.Repeat("  ", len(cc.Chain))
		routeColor.Fprintf(w, "%s← [ROUTE] %s %s\n", indent, cc.Route.Method, cc.Route.Path)
		fmt.Fprintln(w)
	}

	// Print affected routes table
	fmt.Fprintf(w, "Affected Routes (%d):\n", len(uniqueRoutes))
	PrintRouteTable(w, uniqueRoutes)
}

// PrintCallGraph prints the call graph for a function.
func PrintCallGraph(w io.Writer, funcID model.FuncID, result *model.ImpactResult) {
	funcColor.Fprintf(w, "%s (TARGET)\n", funcID.String())

	if len(result.AffectedRoutes) == 0 {
		chainColor.Fprintln(w, "  (no routes reachable)")
		return
	}

	printed := make(map[string]bool)
	for _, cc := range result.AffectedRoutes {
		chainKey := chainToString(cc.Chain) + cc.Route.Method + cc.Route.Path
		if printed[chainKey] {
			continue
		}
		printed[chainKey] = true

		// Print callers (skip the first which is the target itself)
		for i := 1; i < len(cc.Chain); i++ {
			indent := strings.Repeat("  ", i)
			fmt.Fprintf(w, "%s← %s\n", indent, cc.Chain[i].String())
		}
		indent := strings.Repeat("  ", len(cc.Chain))
		routeColor.Fprintf(w, "%s← [ROUTE] %s %s\n", indent, cc.Route.Method, cc.Route.Path)
	}
}

func chainToString(chain []model.FuncID) string {
	parts := make([]string, len(chain))
	for i, f := range chain {
		parts[i] = f.String()
	}
	return strings.Join(parts, " → ")
}

func colorMethod(method string) string {
	trimmed := strings.TrimSpace(method)
	switch trimmed {
	case "GET":
		return methodGet.Sprint(method)
	case "POST":
		return methodPost.Sprint(method)
	case "PUT":
		return methodPut.Sprint(method)
	case "DELETE":
		return methodDelete.Sprint(method)
	default:
		return method
	}
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

package output

import (
	"encoding/json"
	"io"

	"github.com/wuqiang1985/go-route-impact/pkg/model"
)

// PrintRoutesJSON writes routes as JSON.
func PrintRoutesJSON(w io.Writer, routes []model.Route) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(routes)
}

// PrintImpactJSON writes impact result as JSON.
func PrintImpactJSON(w io.Writer, result *model.ImpactResult) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

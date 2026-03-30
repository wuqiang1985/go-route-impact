package extractor

import (
	"github.com/wuqiang1985/go-route-impact/internal/astutil"
	"github.com/wuqiang1985/go-route-impact/pkg/model"
)

// RouteExtractor extracts HTTP routes from a Go web project.
type RouteExtractor interface {
	// Name returns the framework name (e.g., "iris", "gin").
	Name() string
	// Extract scans the project and returns all discovered routes.
	// Each route includes the HandlerFuncID linking to the controller method.
	Extract(projectRoot string, entryPoint string, resolver *astutil.Resolver) ([]model.Route, error)
}

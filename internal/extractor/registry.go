package extractor

import "fmt"

var registry = map[string]RouteExtractor{}

// Register adds a RouteExtractor to the global registry.
func Register(e RouteExtractor) {
	registry[e.Name()] = e
}

// Get returns the extractor for the given framework name.
func Get(name string) (RouteExtractor, error) {
	e, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown framework: %s (available: %v)", name, availableNames())
	}
	return e, nil
}

func availableNames() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	return names
}

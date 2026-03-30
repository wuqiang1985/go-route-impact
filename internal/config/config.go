package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const DefaultConfigFile = ".route-impact.yaml"

// Config represents the tool configuration.
type Config struct {
	Framework  string   `yaml:"framework"`
	EntryPoint string   `yaml:"entry_point"`
	Exclude    []string `yaml:"exclude"`
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Framework:  "auto",
		EntryPoint: "main.go",
		Exclude: []string{
			"vendor/",
			"test/",
			"testdata/",
			"docs/",
		},
	}
}

// Load reads the config from a YAML file.
// If the file doesn't exist, returns the default config.
// projectRoot is used to find .route-impact.yaml when path is empty.
func Load(path string, projectRoot string) (*Config, error) {
	if path == "" {
		// Try project root first, then current directory
		if projectRoot != "" {
			candidate := filepath.Join(projectRoot, DefaultConfigFile)
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
			}
		}
		if path == "" {
			path = DefaultConfigFile
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		return nil, err
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Save writes the config to a YAML file.
func Save(path string, cfg *Config) error {
	if path == "" {
		path = DefaultConfigFile
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

package talos

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Contexts map[string]Context `yaml:"contexts"`
}

type Context struct {
	Endpoints []string `yaml:"endpoints"`
	Nodes     []string `yaml:"nodes"`
}

func ParseConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read talosconfig file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse talosconfig: %w", err)
	}

	return &config, nil
}

func (c *Config) GetFirstControlPlaneNode() (string, error) {
	// Look for the default context (usually "talos")
	for _, context := range c.Contexts {
		if len(context.Endpoints) > 0 {
			return context.Endpoints[0], nil
		}
	}
	return "", fmt.Errorf("no endpoints found in talosconfig")
}

func (c *Config) GetAllNodes() ([]string, error) {
	// Look for the default context (usually "talos")
	for _, context := range c.Contexts {
		if len(context.Nodes) > 0 {
			return context.Nodes, nil
		}
	}
	return nil, fmt.Errorf("no nodes found in talosconfig")
}
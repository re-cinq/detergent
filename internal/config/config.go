package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Agent struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

type Gate struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

type Station struct {
	Name    string   `yaml:"name"`
	Command string   `yaml:"command,omitempty"`
	Args    []string `yaml:"args,omitempty"`
	Prompt  string   `yaml:"prompt"`
}

type Settings struct {
	Watches     string `yaml:"watches"`
	AutoRebase  bool   `yaml:"auto_rebase"`
	AutoResolve bool   `yaml:"auto_resolve"`
}

type Config struct {
	Agent    Agent     `yaml:"agent"`
	Settings Settings  `yaml:"settings"`
	Gates    []Gate    `yaml:"gates"`
	Stations []Station `yaml:"stations"`
}

// ResolvedStation holds the fully resolved command/args for a station.
type ResolvedStation struct {
	Name    string
	Command string
	Args    []string
	Prompt  string
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Settings.Watches == "" {
		return nil, fmt.Errorf("config: settings.watches is required")
	}

	return &cfg, nil
}

// ResolveStation returns the fully resolved command and args for a station,
// falling back to the top-level agent defaults.
func (c *Config) ResolveStation(s Station) ResolvedStation {
	cmd := s.Command
	if cmd == "" {
		cmd = c.Agent.Command
	}

	args := s.Args
	if args == nil {
		args = c.Agent.Args
	}

	return ResolvedStation{
		Name:    s.Name,
		Command: cmd,
		Args:    args,
		Prompt:  s.Prompt,
	}
}

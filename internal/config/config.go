package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Agent       AgentConfig  `yaml:"agent"`
	Settings    Settings     `yaml:"settings"`
	Concerns    []Concern    `yaml:"concerns"`
	Permissions *Permissions `yaml:"permissions,omitempty"`
}

// Permissions mirrors the Claude Code .claude/settings.json permissions block.
// When set, detergent writes this into each worktree before invoking the agent.
type Permissions struct {
	Allow []string `yaml:"allow" json:"allow"`
	Deny  []string `yaml:"deny,omitempty" json:"deny,omitempty"`
}

type AgentConfig struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

type Settings struct {
	PollInterval Duration `yaml:"poll_interval"`
	BranchPrefix string   `yaml:"branch_prefix"`
}

// Duration wraps time.Duration for YAML unmarshaling from strings like "10s".
type Duration time.Duration

func (d *Duration) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(parsed)
	return nil
}

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

type Concern struct {
	Name    string `yaml:"name"`
	Watches string `yaml:"watches"`
	Prompt  string `yaml:"prompt"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	return parse(data)
}

func parse(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	if cfg.Settings.BranchPrefix == "" {
		cfg.Settings.BranchPrefix = "detergent/"
	}
	if cfg.Settings.PollInterval == 0 {
		cfg.Settings.PollInterval = Duration(30 * time.Second)
	}

	return &cfg, nil
}

func Validate(cfg *Config) []error {
	var errs []error

	if cfg.Agent.Command == "" {
		errs = append(errs, fmt.Errorf("agent.command is required"))
	}

	if len(cfg.Concerns) == 0 {
		errs = append(errs, fmt.Errorf("at least one concern is required"))
	}

	names := make(map[string]bool)
	for i, c := range cfg.Concerns {
		if c.Name == "" {
			errs = append(errs, fmt.Errorf("concerns[%d]: name is required", i))
		} else if names[c.Name] {
			errs = append(errs, fmt.Errorf("concerns[%d]: duplicate name %q", i, c.Name))
		} else {
			names[c.Name] = true
		}

		if c.Watches == "" {
			errs = append(errs, fmt.Errorf("concerns[%d] (%s): watches is required", i, c.Name))
		}

		if c.Prompt == "" {
			errs = append(errs, fmt.Errorf("concerns[%d] (%s): prompt is required", i, c.Name))
		}
	}

	if cycleErr := detectCycles(cfg.Concerns); cycleErr != nil {
		errs = append(errs, cycleErr)
	}

	return errs
}

func detectCycles(concerns []Concern) error {
	// Build adjacency: concern name -> what it watches (if that's also a concern)
	nameSet := make(map[string]bool)
	for _, c := range concerns {
		nameSet[c.Name] = true
	}

	// Graph edges: watches -> name (concern depends on what it watches)
	// For cycle detection we need: name -> []downstream
	// Actually: if A watches B, then A depends on B. Edge: A -> B.
	adj := make(map[string][]string)
	for _, c := range concerns {
		if nameSet[c.Watches] {
			adj[c.Name] = append(adj[c.Name], c.Watches)
		}
	}

	// DFS-based cycle detection
	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // done
	)
	color := make(map[string]int)

	var visit func(node string) error
	visit = func(node string) error {
		color[node] = gray
		for _, dep := range adj[node] {
			if color[dep] == gray {
				return fmt.Errorf("cycle detected: %s -> %s", node, dep)
			}
			if color[dep] == white {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		color[node] = black
		return nil
	}

	for _, c := range concerns {
		if color[c.Name] == white {
			if err := visit(c.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

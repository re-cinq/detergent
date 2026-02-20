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
	Gates       []Gate       `yaml:"gates,omitempty"`
	Permissions *Permissions `yaml:"permissions,omitempty"`
	Preamble    string       `yaml:"preamble,omitempty"`
}

// Gate defines a pre-commit quality gate (linter, formatter, type checker, etc.).
type Gate struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

// Permissions mirrors the Claude Code .claude/settings.json permissions block.
// When set, line writes this into each worktree before invoking the agent.
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
	Watches      string   `yaml:"watches"`
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
	Name     string   `yaml:"name"`
	Watches  string   `yaml:"watches"`
	Prompt   string   `yaml:"prompt"`
	Command  string   `yaml:"command,omitempty"`
	Args     []string `yaml:"args,omitempty"`
	Preamble string   `yaml:"preamble,omitempty"`
}

// DefaultPreamble is the preamble prepended to every concern prompt when no
// custom preamble is configured.
const DefaultPreamble = "You are running non-interactively. Do not ask questions or wait for confirmation.\nIf something is unclear, make your best judgement and proceed.\nDo not run git commit — your changes will be committed automatically."

// ResolvePreamble returns the effective preamble for a concern.
// Per-concern preamble takes priority, then global config preamble, then DefaultPreamble.
func (cfg *Config) ResolvePreamble(c Concern) string {
	if c.Preamble != "" {
		return c.Preamble
	}
	if cfg.Preamble != "" {
		return cfg.Preamble
	}
	return DefaultPreamble
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
		cfg.Settings.BranchPrefix = "line/"
	}
	if cfg.Settings.PollInterval == 0 {
		cfg.Settings.PollInterval = Duration(30 * time.Second)
	}
	if cfg.Settings.Watches == "" {
		cfg.Settings.Watches = "main"
	}

	// Auto-populate Watches for concerns defined as an ordered list.
	// First concern watches settings.watches; each subsequent concern
	// watches the previous one. Explicit watches values take precedence.
	for i := range cfg.Concerns {
		if cfg.Concerns[i].Watches == "" {
			if i == 0 {
				cfg.Concerns[i].Watches = cfg.Settings.Watches
			} else {
				cfg.Concerns[i].Watches = cfg.Concerns[i-1].Name
			}
		}
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

		if c.Prompt == "" {
			errs = append(errs, fmt.Errorf("concerns[%d] (%s): prompt is required", i, c.Name))
		}
	}

	if cycleErr := detectCycles(cfg.Concerns); cycleErr != nil {
		errs = append(errs, cycleErr)
	}

	errs = append(errs, ValidateGates(cfg.Gates)...)

	return errs
}

// ValidateGates checks that all gates have non-empty names and run commands,
// and that gate names are unique.
func ValidateGates(gates []Gate) []error {
	var errs []error
	names := make(map[string]bool)
	for i, g := range gates {
		if g.Name == "" {
			errs = append(errs, fmt.Errorf("gates[%d]: name is required", i))
		} else if names[g.Name] {
			errs = append(errs, fmt.Errorf("gates[%d]: duplicate name %q", i, g.Name))
		} else {
			names[g.Name] = true
		}
		if g.Run == "" {
			errs = append(errs, fmt.Errorf("gates[%d]: run is required", i))
		}
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

// HasConcern returns true if a concern with the given name exists in the config.
func (cfg *Config) HasConcern(name string) bool {
	for _, c := range cfg.Concerns {
		if c.Name == name {
			return true
		}
	}
	return false
}

// ValidateConcernName returns an error if the concern name does not exist in the config.
func (cfg *Config) ValidateConcernName(name string) error {
	if !cfg.HasConcern(name) {
		return fmt.Errorf("unknown concern %q", name)
	}
	return nil
}

// BuildNameSet returns a set of all concern names in the config.
func (cfg *Config) BuildNameSet() map[string]bool {
	nameSet := make(map[string]bool, len(cfg.Concerns))
	for _, c := range cfg.Concerns {
		nameSet[c.Name] = true
	}
	return nameSet
}

// BuildDownstreamMap builds an adjacency map: watched -> []watchers.
// For each concern, if it watches another concern in the chain, that creates
// an edge: watched -> watcher.
func (cfg *Config) BuildDownstreamMap() map[string][]string {
	nameSet := cfg.BuildNameSet()
	downstream := make(map[string][]string)
	for _, c := range cfg.Concerns {
		if nameSet[c.Watches] {
			downstream[c.Watches] = append(downstream[c.Watches], c.Name)
		}
	}
	return downstream
}

// FindRoots returns the names of concerns that watch external branches
// (not other concerns in the chain).
func (cfg *Config) FindRoots() []string {
	nameSet := cfg.BuildNameSet()
	var roots []string
	for _, c := range cfg.Concerns {
		if !nameSet[c.Watches] {
			roots = append(roots, c.Name)
		}
	}
	return roots
}

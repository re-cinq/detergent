package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Agent       AgentConfig  `yaml:"agent"`
	Settings    Settings     `yaml:"settings"`
	Stations    []Station    `yaml:"stations"`
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
	BranchPrefix string `yaml:"branch_prefix"`
	Watches      string `yaml:"watches"`
}

type Station struct {
	Name     string   `yaml:"name"`
	Watches  string   `yaml:"watches"`
	Prompt   string   `yaml:"prompt"`
	Command  string   `yaml:"command,omitempty"`
	Args     []string `yaml:"args,omitempty"`
	Preamble string   `yaml:"preamble,omitempty"`
}

// DefaultPreamble is the preamble prepended to every station prompt when no
// custom preamble is configured.
const DefaultPreamble = "You are running non-interactively. Do not ask questions or wait for confirmation.\nIf something is unclear, make your best judgement and proceed.\nDo not run git commit — your changes will be committed automatically."

// ResolvePreamble returns the effective preamble for a station.
// Per-station preamble takes priority, then global config preamble, then DefaultPreamble.
func (cfg *Config) ResolvePreamble(c Station) string {
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
	if cfg.Settings.Watches == "" {
		cfg.Settings.Watches = "main"
	}

	// Auto-populate Watches for stations defined as an ordered list.
	// First station watches settings.watches; each subsequent station
	// watches the previous one. Explicit watches values take precedence.
	for i := range cfg.Stations {
		if cfg.Stations[i].Watches == "" {
			if i == 0 {
				cfg.Stations[i].Watches = cfg.Settings.Watches
			} else {
				cfg.Stations[i].Watches = cfg.Stations[i-1].Name
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

	if len(cfg.Stations) == 0 {
		errs = append(errs, fmt.Errorf("at least one station is required"))
	}

	names := make(map[string]bool)
	for i, c := range cfg.Stations {
		if c.Name == "" {
			errs = append(errs, fmt.Errorf("stations[%d]: name is required", i))
		} else if names[c.Name] {
			errs = append(errs, fmt.Errorf("stations[%d]: duplicate name %q", i, c.Name))
		} else {
			names[c.Name] = true
		}

		if c.Prompt == "" {
			errs = append(errs, fmt.Errorf("stations[%d] (%s): prompt is required", i, c.Name))
		}
	}

	if cycleErr := detectCycles(cfg.Stations); cycleErr != nil {
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

func detectCycles(stations []Station) error {
	// Build adjacency: station name -> what it watches (if that's also a station)
	nameSet := make(map[string]bool)
	for _, c := range stations {
		nameSet[c.Name] = true
	}

	// Graph edges: watches -> name (station depends on what it watches)
	// For cycle detection we need: name -> []downstream
	// Actually: if A watches B, then A depends on B. Edge: A -> B.
	adj := make(map[string][]string)
	for _, c := range stations {
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

	for _, c := range stations {
		if color[c.Name] == white {
			if err := visit(c.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

// HasStation returns true if a station with the given name exists in the config.
func (cfg *Config) HasStation(name string) bool {
	for _, c := range cfg.Stations {
		if c.Name == name {
			return true
		}
	}
	return false
}

// ValidateStationName returns an error if the station name does not exist in the config.
func (cfg *Config) ValidateStationName(name string) error {
	if !cfg.HasStation(name) {
		return fmt.Errorf("unknown station %q", name)
	}
	return nil
}

// BuildNameSet returns a set of all station names in the config.
func (cfg *Config) BuildNameSet() map[string]bool {
	nameSet := make(map[string]bool, len(cfg.Stations))
	for _, c := range cfg.Stations {
		nameSet[c.Name] = true
	}
	return nameSet
}

// BuildDownstreamMap builds an adjacency map: watched -> []watchers.
// For each station, if it watches another station in the line, that creates
// an edge: watched -> watcher.
func (cfg *Config) BuildDownstreamMap() map[string][]string {
	nameSet := cfg.BuildNameSet()
	downstream := make(map[string][]string)
	for _, c := range cfg.Stations {
		if nameSet[c.Watches] {
			downstream[c.Watches] = append(downstream[c.Watches], c.Name)
		}
	}
	return downstream
}

// FindRoots returns the names of stations that watch external branches
// (not other stations in the line).
func (cfg *Config) FindRoots() []string {
	nameSet := cfg.BuildNameSet()
	var roots []string
	for _, c := range cfg.Stations {
		if !nameSet[c.Watches] {
			roots = append(roots, c.Name)
		}
	}
	return roots
}

package config

import "fmt"

// Validate checks a loaded Config for semantic errors beyond what Load catches.
// Returns a list of human/agent-readable error strings, one per issue.
func Validate(cfg *Config) []string {
	var errs []string

	seen := make(map[string]bool)
	for i, s := range cfg.Stations {
		if s.Name == "" {
			errs = append(errs, fmt.Sprintf("stations[%d].name: required field is empty", i))
		} else if seen[s.Name] {
			errs = append(errs, fmt.Sprintf("stations[%d].name: duplicate station name %q", i, s.Name))
		} else {
			seen[s.Name] = true
		}

		if s.Prompt == "" {
			errs = append(errs, fmt.Sprintf("stations[%d].prompt: required field is empty", i))
		}

		if s.Command == "" && cfg.Agent.Command == "" {
			errs = append(errs, fmt.Sprintf("stations[%d]: no resolvable command (set station command or agent.command)", i))
		}
	}

	for i, g := range cfg.Gates {
		if g.Name == "" {
			errs = append(errs, fmt.Sprintf("gates[%d].name: required field is empty", i))
		}
		if g.Run == "" {
			errs = append(errs, fmt.Sprintf("gates[%d].run: required field is empty", i))
		}
	}

	if cfg.Settings.AutoResolve && !cfg.Settings.AutoRebase {
		errs = append(errs, "settings.auto_resolve: has no effect without auto_rebase: true")
	}

	return errs
}

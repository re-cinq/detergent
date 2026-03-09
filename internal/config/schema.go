package config

import "encoding/json"

// Schema returns a JSON Schema describing line.yaml as indented JSON.
func Schema() []byte {
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"title":                "line.yaml",
		"description":          "Configuration for assembly-line — runs gates (pre-commit checks) and stations (post-commit agent tasks) on Git commits.",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"settings", "stations"},
		"properties": map[string]any{
			"agent": map[string]any{
				"description": "Default agent command and arguments inherited by all stations. A station that does not set its own command/args uses these values.",
				"type":        "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "Default executable to run for each station (e.g. \"claude\"). Overridden by station-level command.",
					},
					"args": map[string]any{
						"type":        "array",
						"description": "Default arguments passed to the agent command. The station prompt is appended as the final argument. Overridden by station-level args.",
						"items":       map[string]any{"type": "string"},
					},
				},
			},
			"settings": map[string]any{
				"description": "Global settings for the assembly line.",
				"type":        "object",
				"required":    []string{"watches"},
				"additionalProperties": false,
				"properties": map[string]any{
					"watches": map[string]any{
						"type":        "string",
						"description": "The Git branch to watch for new commits (e.g. \"main\" or \"master\"). When a commit lands on this branch, the line is triggered.",
					},
					"auto_rebase": map[string]any{
						"type":        "boolean",
						"default":     false,
						"description": "Enable the PostToolUse auto-rebase hook. When true, automatically rebases onto the terminal station branch when changes are ready.",
					},
					"auto_resolve": map[string]any{
						"type":        "boolean",
						"default":     false,
						"description": "When true and auto_rebase is true, rebase conflicts are left for agent resolution instead of aborting. The hook reports conflicted files with resolution instructions.",
					},
				},
			},
			"gates": map[string]any{
				"description": "Ordered list of pre-commit checks. Each gate runs as a Git pre-commit hook; if any gate fails, the commit is rejected.",
				"type":        "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"name", "run"},
					"additionalProperties": false,
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Human-readable name for this gate (e.g. \"lint\", \"typecheck\").",
						},
						"run": map[string]any{
							"type":        "string",
							"description": "Shell command to execute. Exit 0 means pass; non-zero means the commit is blocked.",
						},
					},
				},
			},
			"stations": map[string]any{
				"description": "Ordered list of post-commit agent tasks. Each station runs on its own Git branch, in sequence. A station's command is resolved by checking station-level command first, then falling back to agent.command.",
				"type":        "array",
				"items": map[string]any{
					"type":     "object",
					"required": []string{"name", "prompt"},
					"additionalProperties": false,
					"properties": map[string]any{
						"name": map[string]any{
							"type":        "string",
							"description": "Unique name for this station. Maps directly to a Git branch (line/stn/<name>). Must not duplicate another station name.",
						},
						"command": map[string]any{
							"type":        "string",
							"description": "Executable to run for this station, overriding agent.command. If omitted, agent.command is used.",
						},
						"args": map[string]any{
							"type":        "array",
							"description": "Arguments for this station's command, overriding agent.args. The prompt is appended as the final argument. If omitted, agent.args is used.",
							"items":       map[string]any{"type": "string"},
						},
						"prompt": map[string]any{
							"type":        "string",
							"description": "The prompt text passed to the agent command as its final argument. Describes what this station should do.",
						},
					},
				},
			},
		},
	}

	out, _ := json.MarshalIndent(schema, "", "  ")
	return out
}

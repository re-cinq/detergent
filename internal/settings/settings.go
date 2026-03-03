package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeSettings marshals settings to JSON and writes it to settingsPath.
func writeSettings(settingsPath string, settings map[string]any) error {
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, append(out, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing settings: %w", err)
	}
	return nil
}

// RemoveStatusline removes the statusLine key from .claude/settings.json.
func RemoveStatusline(repoDir string) error {
	settings, settingsPath, err := readSettings(repoDir)
	if err != nil {
		return err
	}
	if _, ok := settings["statusLine"]; !ok {
		return nil
	}
	delete(settings, "statusLine")
	return writeSettings(settingsPath, settings)
}

// readSettings reads and parses .claude/settings.json, returning the map and path.
// Returns an empty map if the file doesn't exist.
func readSettings(repoDir string) (map[string]any, string, error) {
	settingsPath := filepath.Join(repoDir, ".claude", "settings.json")
	settings := make(map[string]any)

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, settingsPath, nil
		}
		return nil, settingsPath, fmt.Errorf("reading settings: %w", err)
	}

	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, settingsPath, fmt.Errorf("parsing settings: %w", err)
	}
	return settings, settingsPath, nil
}

// autoRebaseHookEvents lists the Claude Code hook events that trigger auto-rebase.
var autoRebaseHookEvents = []string{"PostToolUse", "Stop"}

const autoRebaseCommand = "line auto-rebase-hook"

// hasAutoRebaseEntry returns true if the hook array already contains our entry.
func hasAutoRebaseEntry(entries []any) bool {
	for _, existing := range entries {
		if e, ok := existing.(map[string]any); ok {
			if hooks, ok := e["hooks"].([]any); ok {
				for _, h := range hooks {
					if hm, ok := h.(map[string]any); ok {
						if cmd, _ := hm["command"].(string); cmd == autoRebaseCommand {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// filterAutoRebaseEntry removes our entry from a hook array, returning the filtered slice.
func filterAutoRebaseEntry(entries []any) []any {
	var filtered []any
	for _, existing := range entries {
		keep := true
		if e, ok := existing.(map[string]any); ok {
			if hooks, ok := e["hooks"].([]any); ok {
				for _, h := range hooks {
					if hm, ok := h.(map[string]any); ok {
						if cmd, _ := hm["command"].(string); cmd == autoRebaseCommand {
							keep = false
						}
					}
				}
			}
		}
		if keep {
			filtered = append(filtered, existing)
		}
	}
	return filtered
}

// ConfigureAutoRebaseHook adds auto-rebase hooks to .claude/settings.json
// for PostToolUse and Stop events.
func ConfigureAutoRebaseHook(repoDir string) error {
	settingsDir := filepath.Join(repoDir, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return fmt.Errorf("creating .claude dir: %w", err)
	}

	settings, settingsPath, err := readSettings(repoDir)
	if err != nil {
		return err
	}

	hooksMap, _ := settings["hooks"].(map[string]any)
	if hooksMap == nil {
		hooksMap = make(map[string]any)
	}

	hook := map[string]any{
		"type":    "command",
		"command": autoRebaseCommand,
		"timeout": 30,
	}
	entry := map[string]any{
		"matcher": "",
		"hooks":   []any{hook},
	}

	for _, event := range autoRebaseHookEvents {
		eventHooks, _ := hooksMap[event].([]any)
		if hasAutoRebaseEntry(eventHooks) {
			continue
		}
		eventHooks = append(eventHooks, entry)
		hooksMap[event] = eventHooks
	}

	settings["hooks"] = hooksMap
	return writeSettings(settingsPath, settings)
}

// RemoveAutoRebaseHook removes the assembly-line auto-rebase hook entries
// from .claude/settings.json, preserving other hooks.
func RemoveAutoRebaseHook(repoDir string) error {
	settings, settingsPath, err := readSettings(repoDir)
	if err != nil {
		return err
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}

	changed := false
	for _, event := range autoRebaseHookEvents {
		eventHooks, ok := hooksMap[event].([]any)
		if !ok {
			continue
		}
		filtered := filterAutoRebaseEntry(eventHooks)
		if len(filtered) == len(eventHooks) {
			continue
		}
		changed = true
		if len(filtered) == 0 {
			delete(hooksMap, event)
		} else {
			hooksMap[event] = filtered
		}
	}

	if !changed {
		return nil
	}

	if len(hooksMap) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooksMap
	}

	return writeSettings(settingsPath, settings)
}

// ConfigureAgentDoneHook installs a Stop hook in the given directory that
// touches a done marker file when the agent's turn ends. This lets the
// runner detect completion without parsing TUI output.
func ConfigureAgentDoneHook(dir, markerFile string) error {
	settingsDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return fmt.Errorf("creating .claude dir: %w", err)
	}

	settings, settingsPath, err := readSettings(dir)
	if err != nil {
		return err
	}

	markerPath := filepath.Join(dir, markerFile)
	hook := map[string]any{
		"type":    "command",
		"command": "touch " + markerPath,
		"timeout": 5,
	}
	entry := map[string]any{
		"matcher": "",
		"hooks":   []any{hook},
	}

	hooksMap, _ := settings["hooks"].(map[string]any)
	if hooksMap == nil {
		hooksMap = make(map[string]any)
	}
	hooksMap["Stop"] = []any{entry}
	settings["hooks"] = hooksMap

	return writeSettings(settingsPath, settings)
}

// RemoveAgentDoneHooks removes any Stop hook entries installed by
// ConfigureAgentDoneHook from .claude/settings.json. Claude Code syncs
// worktree settings back to the main repo, so the done marker touch
// command can leak to the main repo's settings after a line run.
func RemoveAgentDoneHooks(repoDir string) error {
	settings, settingsPath, err := readSettings(repoDir)
	if err != nil {
		return err
	}

	hooksMap, ok := settings["hooks"].(map[string]any)
	if !ok {
		return nil
	}

	stopHooks, ok := hooksMap["Stop"].([]any)
	if !ok {
		return nil
	}

	var filtered []any
	for _, existing := range stopHooks {
		keep := true
		if e, ok := existing.(map[string]any); ok {
			if hooks, ok := e["hooks"].([]any); ok {
				for _, h := range hooks {
					if hm, ok := h.(map[string]any); ok {
						if cmd, _ := hm["command"].(string); strings.Contains(cmd, ".line-agent-done") {
							keep = false
						}
					}
				}
			}
		}
		if keep {
			filtered = append(filtered, existing)
		}
	}

	if len(filtered) == len(stopHooks) {
		return nil
	}

	if len(filtered) == 0 {
		delete(hooksMap, "Stop")
	} else {
		hooksMap["Stop"] = filtered
	}

	if len(hooksMap) == 0 {
		delete(settings, "hooks")
	} else {
		settings["hooks"] = hooksMap
	}

	return writeSettings(settingsPath, settings)
}

// ConfigureStatusline sets the Claude Code statusline to use line statusline.
func ConfigureStatusline(repoDir string) error {
	settingsDir := filepath.Join(repoDir, ".claude")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		return fmt.Errorf("creating .claude dir: %w", err)
	}

	settings, settingsPath, err := readSettings(repoDir)
	if err != nil {
		return err
	}

	settings["statusLine"] = map[string]any{
		"type":    "command",
		"command": "line statusline",
	}

	return writeSettings(settingsPath, settings)
}

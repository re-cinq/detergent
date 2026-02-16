package cli

import "github.com/re-cinq/detergent/internal/engine"

// ANSI escape codes for terminal colors
const (
	ansiGreen       = "\033[32m"
	ansiCyan        = "\033[36m"
	ansiYellow      = "\033[33m"
	ansiRed         = "\033[31m"
	ansiDim         = "\033[2m"
	ansiBoldMagenta = "\033[1;35m"
	ansiReset       = "\033[0m"
)

// stateDisplay returns the symbol and color for a given state.
func stateDisplay(state, lastResult string) (symbol, color string) {
	switch state {
	case engine.StateChangeDetected:
		return "◎", ansiYellow
	case engine.StateAgentRunning, engine.StateCommitting:
		return "⟳", ansiYellow
	case "running": // legacy
		return "⟳", ansiYellow
	case engine.StateFailed:
		return "✗", ansiRed
	case engine.StateSkipped:
		return "⊘", ansiDim
	case "pending":
		return "◯", ansiYellow
	case engine.StateIdle:
		if lastResult != "" {
			return "✓", ansiGreen
		}
		return "·", ansiDim
	case "unknown":
		return "·", ansiDim
	default:
		return "◯", ansiReset
	}
}

package app

import (
	"strings"

	"github.com/andyrewlee/amux/internal/data"
)

func (a *App) defaultAssistantName() string {
	return data.DefaultAssistant
}

func (a *App) assistantNames() []string {
	if a != nil && a.config != nil {
		names := a.config.AssistantNames()
		if len(names) > 0 {
			return names
		}
	}
	return []string{a.defaultAssistantName()}
}

// assistantPickerOptions is the new-tab picker roster: the configured
// assistants plus the "terminal" pseudo-assistant, which opens a plain shell
// tab. Returns a fresh slice so appending never writes into config state.
func (a *App) assistantPickerOptions() []string {
	names := a.assistantNames()
	out := make([]string, 0, len(names)+1)
	out = append(out, names...)
	return append(out, data.TerminalAssistant)
}

func (a *App) isKnownAssistant(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	if name == data.TerminalAssistant {
		// Not a configured assistant: it is handled by the center pane as a
		// shell tab, but every launch path gates on this predicate.
		return true
	}
	if a == nil || a.config == nil || len(a.config.Assistants) == 0 {
		return true
	}
	return a.config.IsAssistantKnown(name)
}

package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAssistantConfigHidden covers the hidden field's merge semantics: an
// absent key defaults to false (visible), an explicit true excludes the
// assistant from VisibleAssistantNames while AssistantNames/IsAssistantKnown
// keep reporting it (hiding is a picker filter, never a deactivation), and an
// explicit false stays visible.
func TestAssistantConfigHidden(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	configPath := filepath.Join(home, ".amux", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	content := `{
  "assistants": {
    "codex": { "hidden": true },
    "droid": { "hidden": false },
    "myagent": { "command": "myagent" }
  }
}`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig() error = %v", err)
	}

	tests := []struct {
		name       string
		assistant  string
		wantHidden bool
	}{
		{"explicit hidden true", "codex", true},
		{"explicit hidden false stays visible", "droid", false},
		{"absent hidden key on custom assistant defaults false", "myagent", false},
		{"unmentioned built-in defaults false", "claude", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cfg.Assistants[tt.assistant]
			if !ok {
				t.Fatalf("expected assistant %q to exist", tt.assistant)
			}
			if got.Hidden != tt.wantHidden {
				t.Fatalf("Assistants[%q].Hidden = %v, want %v", tt.assistant, got.Hidden, tt.wantHidden)
			}
			// Hidden must never remove the assistant from the full roster or
			// the known-assistant check: restore and validation depend on it.
			if !cfg.IsAssistantKnown(tt.assistant) {
				t.Fatalf("IsAssistantKnown(%q) = false, want true even when hidden", tt.assistant)
			}
		})
	}

	names := cfg.AssistantNames()
	for _, want := range []string{"codex", "droid", "myagent", "claude"} {
		if !containsName(names, want) {
			t.Fatalf("AssistantNames() = %v, missing %q (hidden must not shrink the full roster)", names, want)
		}
	}

	visible := cfg.VisibleAssistantNames()
	if containsName(visible, "codex") {
		t.Fatalf("VisibleAssistantNames() = %v, want codex excluded", visible)
	}
	for _, want := range []string{"droid", "myagent", "claude"} {
		if !containsName(visible, want) {
			t.Fatalf("VisibleAssistantNames() = %v, missing %q", visible, want)
		}
	}
}

// TestVisibleAssistantNamesNilConfig mirrors AssistantNames' nil-safety: a nil
// *Config must not panic and reports no visible names.
func TestVisibleAssistantNamesNilConfig(t *testing.T) {
	var c *Config
	if got := c.VisibleAssistantNames(); got != nil {
		t.Fatalf("VisibleAssistantNames() on nil config = %v, want nil", got)
	}
}

func containsName(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}

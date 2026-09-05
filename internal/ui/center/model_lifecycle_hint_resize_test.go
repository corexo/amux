package center

import (
	"testing"

	"github.com/andyrewlee/amux/internal/vterm"
)

// TestSetShowKeymapHintsResizesTerminals pins the bottom-truncation fix: the
// hint bar takes its rows from the terminal area, so toggling it must re-apply
// the terminal sizes. Without that the vterm keeps its taller pre-toggle height
// while View pads to the shorter metrics height and silently drops the bottom
// rows — losing exactly the part of an agent's output that sits at the bottom.
func TestSetShowKeymapHintsResizesTerminals(t *testing.T) {
	m := newTestModel()
	term := vterm.New(80, 20)
	tab := &Tab{ID: TabID("t"), Assistant: "claude", Name: "claude", Running: true, Terminal: term}
	addWorkspaceWithTabs(t, m, "ws", tab)
	m.SetShowKeymapHints(false)
	m.SetSize(90, 30)

	if got, want := term.Height, m.terminalMetrics().Height; got != want {
		t.Fatalf("baseline: vterm height %d, metrics %d", got, want)
	}

	m.SetShowKeymapHints(true)

	metrics := m.terminalMetrics()
	if len(m.helpLines(m.contentWidth())) == 0 {
		t.Fatal("expected the hint bar to occupy rows for this fixture")
	}
	if term.Height != metrics.Height {
		t.Fatalf("vterm height %d does not follow metrics height %d after enabling hints; "+
			"View would truncate the bottom %d row(s)", term.Height, metrics.Height, term.Height-metrics.Height)
	}

	m.SetShowKeymapHints(false)
	if metrics := m.terminalMetrics(); term.Height != metrics.Height {
		t.Fatalf("vterm height %d does not follow metrics height %d after disabling hints", term.Height, metrics.Height)
	}
}

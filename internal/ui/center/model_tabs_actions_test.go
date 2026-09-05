package center

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andyrewlee/amux/internal/data"
	"github.com/andyrewlee/amux/internal/messages"
	"github.com/andyrewlee/amux/internal/ui/diff"
)

// newActionsModel wires a model to an active workspace with the supplied tabs so
// the workspace-scoped helpers (getTabs/getActiveTabIdx/...) resolve correctly.
// workspaceID() returns "" until m.workspace is set, which would silently route
// every lookup to the empty-string bucket, so always go through this helper.
func newActionsModel(t *testing.T, tabs ...*Tab) (*Model, *data.Workspace, string) {
	t.Helper()
	m := newTestModel()
	ws := newTestWorkspace("ws", "/repo/ws")
	wsID := string(ws.ID())
	m.tabs.ByWorkspace[wsID] = tabs
	m.tabs.ActiveByWorkspace[wsID] = 0
	m.workspace = ws
	return m, ws, wsID
}

func chatTab(ws *data.Workspace, id string) *Tab {
	return &Tab{
		ID:        TabID(id),
		Name:      id,
		Assistant: "claude",
		Workspace: ws,
		Running:   true,
	}
}

// diffTab builds a non-chat tab (a diff viewer, keyed off DiffViewer != nil per
// isChatTab) so tests can assert diff tabs keep closing synchronously without
// the agent-tab confirmation guard.
func diffTab(ws *data.Workspace, id string) *Tab {
	return &Tab{
		ID:         TabID(id),
		Name:       id,
		Assistant:  "diff",
		Workspace:  ws,
		DiffViewer: &diff.Model{},
	}
}

// drainBatch runs a (possibly batched) command and returns every concrete
// message it produces, flattening one level of tea.BatchMsg.
func drainBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, sub := range batch {
			if sub == nil {
				continue
			}
			out = append(out, sub())
		}
		return out
	}
	return []tea.Msg{msg}
}

// ----- nextTab / prevTab / NextTab / PrevTab -----

func TestNextPrevTab_CycleCircularly(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	m, _, wsID := newActionsModel(t,
		chatTab(ws, "a"), chatTab(ws, "b"), chatTab(ws, "c"))

	m.nextTab()
	if got := m.tabs.ActiveByWorkspace[wsID]; got != 1 {
		t.Fatalf("nextTab from 0 -> %d, want 1", got)
	}
	m.nextTab()
	m.nextTab()
	if got := m.tabs.ActiveByWorkspace[wsID]; got != 0 {
		t.Fatalf("nextTab should wrap to 0, got %d", got)
	}
	m.prevTab()
	if got := m.tabs.ActiveByWorkspace[wsID]; got != 2 {
		t.Fatalf("prevTab from 0 should wrap to 2, got %d", got)
	}
}

func TestNextPrevTab_EmptyListLeavesIndexUnchanged(t *testing.T) {
	m, _, wsID := newActionsModel(t)
	m.tabs.ActiveByWorkspace[wsID] = 0

	m.nextTab()
	m.prevTab()
	if got := m.tabs.ActiveByWorkspace[wsID]; got != 0 {
		t.Fatalf("expected index to stay 0 with no tabs, got %d", got)
	}
}

func TestNextTab_PublicWrapperMovesAndReturnsSelectionCmd(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	m, _, wsID := newActionsModel(t, chatTab(ws, "a"), chatTab(ws, "b"))

	cmd := m.NextTab()
	if got := m.tabs.ActiveByWorkspace[wsID]; got != 1 {
		t.Fatalf("NextTab should advance active index to 1, got %d", got)
	}
	assertSelectionChanged(t, cmd, wsID, 1)
}

func TestPrevTab_PublicWrapperMovesAndReturnsSelectionCmd(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	m, _, wsID := newActionsModel(t, chatTab(ws, "a"), chatTab(ws, "b"))

	cmd := m.PrevTab()
	if got := m.tabs.ActiveByWorkspace[wsID]; got != 1 {
		t.Fatalf("PrevTab from 0 should wrap to 1, got %d", got)
	}
	assertSelectionChanged(t, cmd, wsID, 1)
}

// ----- SelectTab -----

func TestSelectTab_ValidIndexSelectsAndEmitsCmd(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	m, _, wsID := newActionsModel(t, chatTab(ws, "a"), chatTab(ws, "b"), chatTab(ws, "c"))

	cmd := m.SelectTab(2)
	if got := m.tabs.ActiveByWorkspace[wsID]; got != 2 {
		t.Fatalf("SelectTab(2) active index = %d, want 2", got)
	}
	assertSelectionChanged(t, cmd, wsID, 2)
}

func TestSelectTab_OutOfRangeIsNoOp(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	m, _, wsID := newActionsModel(t, chatTab(ws, "a"), chatTab(ws, "b"))

	for _, idx := range []int{-1, 2, 50} {
		if cmd := m.SelectTab(idx); cmd != nil {
			t.Fatalf("SelectTab(%d) out of range expected nil cmd", idx)
		}
	}
	if got := m.tabs.ActiveByWorkspace[wsID]; got != 0 {
		t.Fatalf("expected active index unchanged at 0, got %d", got)
	}
}

func TestSelectTab_EmptyListIsNoOp(t *testing.T) {
	m, _, _ := newActionsModel(t)
	if cmd := m.SelectTab(0); cmd != nil {
		t.Fatalf("SelectTab on empty list expected nil cmd")
	}
}

// ----- tabSelectionCommand -----

func TestTabSelectionCommand_NilWhenNoWorkspace(t *testing.T) {
	m := newTestModel() // no workspace set -> workspaceID() == ""
	if cmd := m.tabSelectionCommand(); cmd != nil {
		t.Fatalf("expected nil selection cmd without an active workspace")
	}
}

func TestTabSelectionCommand_EmitsSelectionChanged(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	m, _, wsID := newActionsModel(t, chatTab(ws, "a"), chatTab(ws, "b"))
	m.tabs.ActiveByWorkspace[wsID] = 1

	assertSelectionChanged(t, m.tabSelectionCommand(), wsID, 1)
}

func assertSelectionChanged(t *testing.T, cmd tea.Cmd, wsID string, wantIdx int) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected non-nil selection cmd")
	}
	var found bool
	for _, msg := range drainBatch(cmd) {
		if sel, ok := msg.(messages.TabSelectionChanged); ok {
			found = true
			if sel.WorkspaceID != wsID || sel.ActiveIndex != wantIdx {
				t.Fatalf("TabSelectionChanged = %+v, want ws=%q idx=%d", sel, wsID, wantIdx)
			}
		}
	}
	if !found {
		t.Fatalf("expected messages.TabSelectionChanged in command output")
	}
}

// ----- reattachActiveTabIfDetached / ReattachActiveTabIfDetached -----

func TestReattachActiveTabIfDetached_NoTabsIsNil(t *testing.T) {
	m, _, _ := newActionsModel(t)
	if cmd := m.reattachActiveTabIfDetached(); cmd != nil {
		t.Fatalf("expected nil with no tabs")
	}
	if cmd := m.ReattachActiveTabIfDetached(); cmd != nil {
		t.Fatalf("expected nil from public wrapper with no tabs")
	}
}

func TestReattachActiveTabIfDetached_NotDetachedIsNil(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := chatTab(ws, "tab-0") // attached chat tab
	m, _, _ := newActionsModel(t, tab)

	if cmd := m.reattachActiveTabIfDetached(); cmd != nil {
		t.Fatalf("expected nil reattach cmd for attached tab")
	}
}

func TestReattachActiveTabIfDetached_ClosedTabIsNil(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := chatTab(ws, "tab-0")
	tab.Detached = true
	tab.markClosed()
	m, _, _ := newActionsModel(t, tab)

	if cmd := m.reattachActiveTabIfDetached(); cmd != nil {
		t.Fatalf("expected nil reattach cmd for closed tab")
	}
}

func TestReattachActiveTabIfDetached_ReattachInFlightIsNil(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := chatTab(ws, "tab-0")
	tab.Detached = true
	tab.reattachInFlight = true
	m, _, _ := newActionsModel(t, tab)

	if cmd := m.reattachActiveTabIfDetached(); cmd != nil {
		t.Fatalf("expected nil reattach cmd while a reattach is already in flight")
	}
}

func TestReattachActiveTabIfDetached_NonChatTabIsNil(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := &Tab{
		ID:        TabID("tab-0"),
		Assistant: "vim", // not a registered chat assistant
		Workspace: ws,
		Detached:  true,
	}
	m, _, _ := newActionsModel(t, tab)

	if cmd := m.reattachActiveTabIfDetached(); cmd != nil {
		t.Fatalf("expected nil reattach cmd for a detached non-chat tab")
	}
}

package center

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andyrewlee/amux/internal/messages"
	"github.com/andyrewlee/amux/internal/vterm"
)

// ----- closeCurrentTab / closeTabAt -----

func TestCloseTabAt_OutOfRangeIsNoOp(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := chatTab(ws, "tab-0")
	m, _, wsID := newActionsModel(t, tab)

	for _, idx := range []int{-1, 1, 99} {
		if cmd := m.closeTabAt(idx); cmd != nil {
			t.Fatalf("closeTabAt(%d) on single tab expected nil cmd, got non-nil", idx)
		}
	}
	if got := len(m.tabs.ByWorkspace[wsID]); got != 1 {
		t.Fatalf("expected tab list untouched, got %d tabs", got)
	}
	if tab.isClosed() {
		t.Fatalf("expected out-of-range close to leave tab open")
	}
}

func TestCloseTabAt_EmptyListIsNoOp(t *testing.T) {
	m, _, _ := newActionsModel(t)
	if cmd := m.closeTabAt(0); cmd != nil {
		t.Fatalf("closeTabAt on empty list expected nil cmd")
	}
}

func TestCloseTabAt_ChatTabRequestsConfirmationWithoutClosing(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := chatTab(ws, "tab-0")
	m, _, wsID := newActionsModel(t, tab)

	cmd := m.closeTabAt(0)
	if cmd == nil {
		t.Fatalf("expected a confirmation cmd for a chat tab")
	}

	var dlg messages.ShowCloseTabDialog
	var found bool
	for _, msg := range drainBatch(cmd) {
		if d, ok := msg.(messages.ShowCloseTabDialog); ok {
			dlg, found = d, true
		}
	}
	if !found {
		t.Fatalf("expected messages.ShowCloseTabDialog, got none")
	}
	if dlg.WorkspaceID != wsID || dlg.TabID != "tab-0" || dlg.TabName != "tab-0" {
		t.Fatalf("ShowCloseTabDialog = %+v, want ws=%q tab=%q name=%q", dlg, wsID, "tab-0", "tab-0")
	}
	if len(m.tabs.ByWorkspace[wsID]) != 1 {
		t.Fatalf("expected the tab list untouched before confirmation")
	}
	if tab.isClosed() {
		t.Fatalf("expected the tab to remain open pending confirmation")
	}
}

func TestCloseTabAt_DiffTabClosesImmediately(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := diffTab(ws, "tab-diff")
	m, _, wsID := newActionsModel(t, tab)

	cmd := m.closeTabAt(0)
	if cmd == nil {
		t.Fatalf("expected a close cmd for a diff tab")
	}
	if len(m.tabs.ByWorkspace[wsID]) != 0 {
		t.Fatalf("expected the diff tab to close immediately, no confirmation")
	}
	if !tab.isClosed() {
		t.Fatalf("expected the diff tab to be marked closed")
	}
}

// ----- CloseTabByID (the confirmed-close path) -----

func TestCloseTabByID_RemovesTabAndReportsIndex(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	first := chatTab(ws, "tab-0")
	second := chatTab(ws, "tab-1")
	m, _, wsID := newActionsModel(t, first, second)
	m.tabs.ActiveByWorkspace[wsID] = 1

	cmd := m.CloseTabByID(wsID, "tab-0")
	if cmd == nil {
		t.Fatalf("expected close cmd")
	}

	remaining := m.tabs.ByWorkspace[wsID]
	if len(remaining) != 1 || remaining[0].ID != "tab-1" {
		t.Fatalf("expected only tab-1 to remain, got %+v", remaining)
	}
	if !first.isClosed() {
		t.Fatalf("expected closed tab to be marked closed")
	}
	// Removing a tab before the active index shifts the active index left.
	if got := m.tabs.ActiveByWorkspace[wsID]; got != 0 {
		t.Fatalf("active index should shift to 0 after removing index 0, got %d", got)
	}

	msgs := drainBatch(cmd)
	var gotClosed bool
	for _, msg := range msgs {
		if closed, ok := msg.(messages.TabClosed); ok {
			gotClosed = true
			if closed.Index != 0 {
				t.Fatalf("TabClosed.Index = %d, want 0", closed.Index)
			}
		}
	}
	if !gotClosed {
		t.Fatalf("expected messages.TabClosed in command output")
	}
}

func TestCloseTabByID_ClosingActiveLastTabClampsIndex(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	first := chatTab(ws, "tab-0")
	second := chatTab(ws, "tab-1")
	m, _, wsID := newActionsModel(t, first, second)
	m.tabs.ActiveByWorkspace[wsID] = 1

	if cmd := m.CloseTabByID(wsID, "tab-1"); cmd == nil {
		t.Fatalf("expected close cmd")
	}
	// Active was the last tab; closing it should clamp the active index down.
	if got := m.tabs.ActiveByWorkspace[wsID]; got != 0 {
		t.Fatalf("active index should clamp to 0, got %d", got)
	}
	if len(m.tabs.ByWorkspace[wsID]) != 1 {
		t.Fatalf("expected one remaining tab")
	}
}

func TestCloseTabByID_BatchesKillWhenSessionPresent(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := chatTab(ws, "tab-0")
	tab.SessionName = "amux-some-session"
	m, _, wsID := newActionsModel(t, tab)

	cmd := m.CloseTabByID(wsID, "tab-0")
	if cmd == nil {
		t.Fatalf("expected close cmd")
	}
	// With a session name the result is a Batch (close notification + async kill).
	if _, ok := cmd().(tea.BatchMsg); !ok {
		t.Fatalf("expected tea.BatchMsg when a tmux session must be killed")
	}
}

func TestCloseTabByID_UnknownTabIsNoOp(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := chatTab(ws, "tab-0")
	m, _, wsID := newActionsModel(t, tab)

	if cmd := m.CloseTabByID(wsID, "does-not-exist"); cmd != nil {
		t.Fatalf("expected nil cmd for an unknown tab ID")
	}
	if cmd := m.CloseTabByID("unknown-workspace", "tab-0"); cmd != nil {
		t.Fatalf("expected nil cmd for an unknown workspace ID")
	}
	if len(m.tabs.ByWorkspace[wsID]) != 1 {
		t.Fatalf("expected the tab list untouched")
	}
}

func TestCloseTabByID_AlreadyClosedTabIsNoOp(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := chatTab(ws, "tab-0")
	tab.markClosed()
	m, _, wsID := newActionsModel(t, tab)

	if cmd := m.CloseTabByID(wsID, "tab-0"); cmd != nil {
		t.Fatalf("expected nil cmd for an already-closed tab")
	}
}

func TestCloseCurrentTab_DelegatesToActiveIndex(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	first := chatTab(ws, "tab-0")
	second := chatTab(ws, "tab-1")
	m, _, wsID := newActionsModel(t, first, second)
	m.tabs.ActiveByWorkspace[wsID] = 1

	// Chat tabs are guarded: closeCurrentTab must ask about the *active* tab
	// (tab-1), not tab-0, and must not close anything itself.
	cmd := m.closeCurrentTab()
	if cmd == nil {
		t.Fatalf("expected a cmd for the active tab")
	}
	var dlg messages.ShowCloseTabDialog
	var found bool
	for _, msg := range drainBatch(cmd) {
		if d, ok := msg.(messages.ShowCloseTabDialog); ok {
			dlg, found = d, true
		}
	}
	if !found {
		t.Fatalf("expected messages.ShowCloseTabDialog naming the active tab")
	}
	if dlg.TabID != "tab-1" {
		t.Fatalf("ShowCloseTabDialog.TabID = %q, want tab-1", dlg.TabID)
	}
	if len(m.tabs.ByWorkspace[wsID]) != 2 {
		t.Fatalf("expected both tabs to remain until confirmation")
	}
}

func TestCloseCurrentTab_EmptyListIsNoOp(t *testing.T) {
	m, _, _ := newActionsModel(t)
	if cmd := m.closeCurrentTab(); cmd != nil {
		t.Fatalf("closeCurrentTab on empty list expected nil cmd")
	}
}

func TestCloseCurrentTab_OutOfRangeActiveIsNoOp(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := chatTab(ws, "tab-0")
	m, _, wsID := newActionsModel(t, tab)
	m.tabs.ActiveByWorkspace[wsID] = 5 // beyond range

	if cmd := m.closeCurrentTab(); cmd != nil {
		t.Fatalf("closeCurrentTab with out-of-range active index expected nil cmd")
	}
	if len(m.tabs.ByWorkspace[wsID]) != 1 {
		t.Fatalf("expected tab list untouched")
	}
}

// CloseActiveTab is a thin public wrapper over closeCurrentTab; a chat tab
// still routes through the confirmation guard rather than closing directly.
func TestCloseActiveTab_PublicWrapper(t *testing.T) {
	ws := newTestWorkspace("ws", "/repo/ws")
	tab := chatTab(ws, "tab-0")
	m, _, wsID := newActionsModel(t, tab)

	cmd := m.CloseActiveTab()
	if cmd == nil {
		t.Fatalf("expected CloseActiveTab cmd")
	}
	msgs := drainBatch(cmd)
	if len(msgs) != 1 {
		t.Fatalf("expected exactly one message, got %d", len(msgs))
	}
	if _, ok := msgs[0].(messages.ShowCloseTabDialog); !ok {
		t.Fatalf("expected CloseActiveTab to request confirmation for a chat tab, got %T", msgs[0])
	}
	if len(m.tabs.ByWorkspace[wsID]) != 1 {
		t.Fatalf("expected the tab to remain open pending confirmation")
	}
}

// ----- tab bar mouse close -----

func TestHandleTabBarClick_CloseHitOnChatTabShowsConfirmation(t *testing.T) {
	m := newTestModel()
	ws := newTestWorkspace("ws", "/repo/ws")
	m.SetWorkspace(ws)
	wsID := string(ws.ID())
	tab := &Tab{
		ID:        TabID("tab-0"),
		Name:      "claude",
		Assistant: "claude",
		Workspace: ws,
		Running:   true,
		Terminal:  vterm.New(80, 24),
	}
	m.tabs.ByWorkspace[wsID] = []*Tab{tab}
	m.tabs.ActiveByWorkspace[wsID] = 0
	m.SetSize(100, 40)
	m.SetOffset(0)
	m.Focus()

	// Render to populate tab hits.
	_ = m.View()

	var closeHit *tabHit
	for i := range m.tabHits {
		if m.tabHits[i].kind == tabHitClose && m.tabHits[i].index == 0 {
			closeHit = &m.tabHits[i]
			break
		}
	}
	if closeHit == nil {
		t.Fatalf("expected a close hit region for tab 0")
	}

	const (
		borderTop   = 1
		borderLeft  = 1
		paddingLeft = 1
	)
	click := tea.MouseClickMsg{
		X:      m.offsetX + borderLeft + paddingLeft + closeHit.region.X,
		Y:      borderTop,
		Button: tea.MouseLeft,
	}
	_, cmd := m.Update(click)
	if cmd == nil {
		t.Fatalf("expected a command from clicking the close region on a chat tab")
	}

	var dlg messages.ShowCloseTabDialog
	var found bool
	for _, msg := range drainBatch(cmd) {
		if d, ok := msg.(messages.ShowCloseTabDialog); ok {
			dlg, found = d, true
		}
	}
	if !found {
		t.Fatalf("expected messages.ShowCloseTabDialog from a close-region click on a chat tab")
	}
	if dlg.WorkspaceID != wsID || dlg.TabID != "tab-0" {
		t.Fatalf("ShowCloseTabDialog = %+v, want ws=%q tab=%q", dlg, wsID, "tab-0")
	}
	if len(m.tabs.ByWorkspace[wsID]) != 1 {
		t.Fatalf("expected the tab to remain open pending confirmation")
	}
}

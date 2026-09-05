package center

import (
	"testing"

	"github.com/andyrewlee/amux/internal/data"
	appPty "github.com/andyrewlee/amux/internal/pty"
	"github.com/andyrewlee/amux/internal/tmux"
)

// captureResumeSeam installs a createAgentWithTagsFn that records the resume
// flag from every call (without touching tmux) and returns a stub agent.
func captureResumeSeam(t *testing.T, resumes *[]bool) {
	t.Helper()
	restoreReattachSeams(t)
	createAgentWithTagsFn = func(
		manager *appPty.AgentManager,
		ws *data.Workspace,
		agentType appPty.AgentType,
		sessionName string,
		rows, cols uint16,
		tags tmux.SessionTags,
		resume bool,
	) (*appPty.Agent, error) {
		*resumes = append(*resumes, resume)
		return &appPty.Agent{Session: sessionName}, nil
	}
}

// TestReattachToSession_RestoreWithMissingSessionRelaunchesWithResume covers
// the scenario this ticket exists for: a persisted tab is hydrated
// (isRestore=true) but its tmux session is gone (e.g. a machine/WSL restart
// wiped the tmux server), so reattachToSession must relaunch fresh with
// resume=true instead of the old dead-end Stopped failure.
func TestReattachToSession_RestoreWithMissingSessionRelaunchesWithResume(t *testing.T) {
	m := newTestModel()
	setKnownViewport(m)
	ws := newTestWorkspace("ws", "/repo/ws")

	var resumes []bool
	captureResumeSeam(t, &resumes)
	sessionStateForFn = func(string, tmux.Options) (tmux.SessionState, error) {
		return tmux.SessionState{Exists: false}, nil
	}
	killCalled := false
	killSessionFn = func(string, tmux.Options) error {
		killCalled = true
		return nil
	}

	msg := m.reattachToSession(ws, TabID("tab-restore"), "claude", "sess-gone", 1, true)()
	result, ok := msg.(ptyTabReattachResult)
	if !ok {
		t.Fatalf("expected ptyTabReattachResult, got %T (%+v)", msg, msg)
	}
	if result.Agent == nil || result.Agent.Session != "sess-gone" {
		t.Fatalf("expected a fresh agent for sess-gone, got %+v", result.Agent)
	}
	if len(resumes) != 1 || !resumes[0] {
		t.Fatalf("expected createAgentWithTagsFn called once with resume=true, got %v", resumes)
	}
	if killCalled {
		t.Fatal("restore path must never kill a session; the session is already gone")
	}
}

// TestReattachToSession_NonRestoreWithMissingSessionStaysStoppedWithoutLaunch
// is the required non-regression: a non-restore reattach (the user manually
// retriggers a reattach, or a live-tmux-discovery merge) whose session is
// missing keeps today's behavior byte-for-byte - Stopped, no relaunch -
// because resume only ever applies to startup hydration.
func TestReattachToSession_NonRestoreWithMissingSessionStaysStoppedWithoutLaunch(t *testing.T) {
	m := newTestModel()
	setKnownViewport(m)
	ws := newTestWorkspace("ws", "/repo/ws")

	var resumes []bool
	captureResumeSeam(t, &resumes)
	sessionStateForFn = func(string, tmux.Options) (tmux.SessionState, error) {
		return tmux.SessionState{Exists: false}, nil
	}

	msg := m.reattachToSession(ws, TabID("tab-nonrestore"), "claude", "sess-gone", 1, false)()
	failed, ok := msg.(ptyTabReattachFailed)
	if !ok {
		t.Fatalf("expected ptyTabReattachFailed, got %T (%+v)", msg, msg)
	}
	if !failed.Stopped {
		t.Fatal("expected Stopped=true, matching today's dead-session outcome")
	}
	if len(resumes) != 0 {
		t.Fatalf("expected createAgentWithTagsFn not called at all, got %d call(s): %v", len(resumes), resumes)
	}
}

// TestReattachToSession_ExistingLiveSessionNeverResumes covers a session that
// still exists with a live pane: it is just reattached (tmux's
// attach-if-exists semantics), never relaunched, so resume must be false
// regardless of isRestore.
func TestReattachToSession_ExistingLiveSessionNeverResumes(t *testing.T) {
	m := newTestModel()
	setKnownViewport(m)
	ws := newTestWorkspace("ws", "/repo/ws")

	var resumes []bool
	captureResumeSeam(t, &resumes)
	probe := eligibleReattachProbe()
	sessionStateForFn = func(string, tmux.Options) (tmux.SessionState, error) {
		return tmux.SessionState{Exists: true, HasLivePane: true}, nil
	}
	probeSessionFn = func(string, tmux.Options) (tmux.SessionProbe, error) {
		return probe, nil
	}
	resizePaneToSizeFn = func(string, int, int, tmux.Options) error { return nil }
	capturePaneFullDataFn = func(string, tmux.Options) ([]byte, error) { return nil, nil }
	capturePaneHistoryDataFn = func(string, tmux.Options) ([]byte, error) { return nil, nil }
	capturePaneFn = func(string, tmux.Options) ([]byte, error) { return nil, nil }

	msg := m.reattachToSession(ws, TabID("tab-live"), "claude", "sess-live", 1, true)()
	if _, ok := msg.(ptyTabReattachResult); !ok {
		t.Fatalf("expected ptyTabReattachResult, got %T (%+v)", msg, msg)
	}
	if len(resumes) != 1 || resumes[0] {
		t.Fatalf("expected createAgentWithTagsFn called once with resume=false, got %v", resumes)
	}
}

// TestReattachToSession_RestoreWithDeadPaneStaysStoppedWithoutKill covers the
// middle case: the session still exists but has no live pane. Even on
// restore, this must keep today's Stopped outcome and must never kill the
// session - only the explicit user-triggered restart (RestartActiveTab) tears
// one down.
func TestReattachToSession_RestoreWithDeadPaneStaysStoppedWithoutKill(t *testing.T) {
	m := newTestModel()
	setKnownViewport(m)
	ws := newTestWorkspace("ws", "/repo/ws")

	var resumes []bool
	captureResumeSeam(t, &resumes)
	sessionStateForFn = func(string, tmux.Options) (tmux.SessionState, error) {
		return tmux.SessionState{Exists: true, HasLivePane: false}, nil
	}
	killCalled := false
	killSessionFn = func(string, tmux.Options) error {
		killCalled = true
		return nil
	}

	msg := m.reattachToSession(ws, TabID("tab-dead-pane"), "claude", "sess-dead", 1, true)()
	failed, ok := msg.(ptyTabReattachFailed)
	if !ok {
		t.Fatalf("expected ptyTabReattachFailed, got %T (%+v)", msg, msg)
	}
	if !failed.Stopped {
		t.Fatal("expected Stopped=true for a session with no live pane")
	}
	if len(resumes) != 0 {
		t.Fatalf("expected createAgentWithTagsFn not called at all, got %d call(s): %v", len(resumes), resumes)
	}
	if killCalled {
		t.Fatal("restore path must never kill a session; only RestartActiveTab does")
	}
}

// TestCreateAgentTabWithSession_NewTabNeverResumes proves a user-pressed new
// tab always launches fresh: resume is a startup-restore-only concept, and the
// new-tab path (createAgentTabWithSession) has no isRestore signal at all.
func TestCreateAgentTabWithSession_NewTabNeverResumes(t *testing.T) {
	m := newTestModel()
	ws := newTestWorkspace("ws", "/repo/ws")

	var resumes []bool
	captureResumeSeam(t, &resumes)
	probeSessionFn = func(string, tmux.Options) (tmux.SessionProbe, error) {
		return tmux.SessionProbe{}, nil
	}
	resizePaneToSizeFn = func(string, int, int, tmux.Options) error { return nil }
	capturePaneHistoryDataFn = func(string, tmux.Options) ([]byte, error) { return nil, nil }
	capturePaneFullDataFn = func(string, tmux.Options) ([]byte, error) { return nil, nil }
	capturePaneFn = func(string, tmux.Options) ([]byte, error) { return nil, nil }

	msg := m.createAgentTab("claude", ws)()
	if _, ok := msg.(ptyTabCreateResult); !ok {
		t.Fatalf("expected ptyTabCreateResult, got %T (%+v)", msg, msg)
	}
	if len(resumes) != 1 || resumes[0] {
		t.Fatalf("expected createAgentWithTagsFn called once with resume=false, got %v", resumes)
	}
}

// TestAddTabsFromWorkspace_DiscoveredTabNeverResumes covers the other
// AddTabsFromWorkspace caller: tabs discovered from already-live tmux sessions
// (isRestore=false, app_tmux_discover.go) must never resume, even in the
// (normally unreachable) case where the session vanished between discovery and
// reattach - the state.Exists gate makes this safe either way, but isRestore
// must still reach reattachToSession as false from this call site.
func TestAddTabsFromWorkspace_DiscoveredTabNeverResumes(t *testing.T) {
	m := newTestModel()
	setKnownViewport(m)
	ws := newTestWorkspace("ws", "/repo/ws")

	var resumes []bool
	captureResumeSeam(t, &resumes)
	sessionStateForFn = func(string, tmux.Options) (tmux.SessionState, error) {
		return tmux.SessionState{Exists: false}, nil
	}

	cmd := m.AddTabsFromWorkspace(ws, []data.TabInfo{
		{Assistant: "claude", Status: "running", SessionName: "sess-gone"},
	}, false)
	if cmd == nil {
		t.Fatal("expected a reattach cmd for a running tab")
	}
	msg := cmd()
	failed, ok := msg.(ptyTabReattachFailed)
	if !ok {
		t.Fatalf("expected ptyTabReattachFailed, got %T (%+v)", msg, msg)
	}
	if !failed.Stopped {
		t.Fatal("expected Stopped=true, matching today's dead-session outcome")
	}
	if len(resumes) != 0 {
		t.Fatalf("expected createAgentWithTagsFn not called at all, got %d call(s): %v", len(resumes), resumes)
	}
}

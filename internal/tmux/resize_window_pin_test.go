package tmux

import (
	"os/exec"
	"strings"
	"testing"
)

// windowOptions returns the session's window-option block, so a test can assert
// on what `resize-window` left behind.
func windowOptions(t *testing.T, opts Options, session string) string {
	t.Helper()
	args := tmuxArgs(opts, "show-options", "-t", session, "-w")
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("show-options -w: %v (%s)", err, out)
	}
	return string(out)
}

// TestResizePaneToSize_LeavesAutomaticSizing pins the fix for a stuck pane:
// tmux's `resize-window -x/-y` also sets `window-size manual` on the window and
// never lifts it, so the window would ignore every later client size — the amux
// window gets maximized, the pane keeps its old geometry, and tmux pads the
// surplus client area with dots. ResizePaneToSize must therefore apply the
// requested size AND clear the pin, so the attach that follows resizes the
// window to the real client again.
func TestResizePaneToSize_LeavesAutomaticSizing(t *testing.T) {
	skipIfNoTmux(t)
	opts := testServer(t)

	createSession(t, opts, "resize-pin", "sleep 300")

	if err := ResizePaneToSize("resize-pin", 60, 20, opts); err != nil {
		t.Fatalf("ResizePaneToSize: %v", err)
	}

	// The snapshot size must still be in effect — clearing the pin may not
	// resize the window behind the caller's back.
	if got := sessionWindowSize(t, opts, "resize-pin"); got != "60x20" {
		t.Fatalf("window size = %s, want 60x20", got)
	}
	if opts := windowOptions(t, opts, "resize-pin"); strings.Contains(opts, "window-size") {
		t.Fatalf("window-size must not stay pinned after a snapshot resize, got:\n%s", opts)
	}
}

// sessionWindowSize reports the tmux window geometry as "WxH".
func sessionWindowSize(t *testing.T, opts Options, session string) string {
	t.Helper()
	args := tmuxArgs(opts, "display-message", "-p", "-t", session, "#{window_width}x#{window_height}")
	out, err := exec.Command("tmux", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("display-message: %v (%s)", err, out)
	}
	return strings.TrimSpace(string(out))
}

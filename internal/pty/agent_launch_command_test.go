package pty

import (
	"testing"

	"github.com/andyrewlee/amux/internal/config"
)

// TestAgentLaunchCommand pins the exact shell command CreateAgentWithTags hands
// to tmux, independent of a running tmux server: resume with configured args
// wraps the assistant command in a timed, fail-soft POSIX-sh guard that only
// falls back to a fresh launch when the resume attempt dies almost
// immediately (see resumeExitGraceWindowSeconds), while resume with no
// configured args, and resume=false, both produce byte-for-byte today's plain
// launch command.
func TestAgentLaunchCommand(t *testing.T) {
	const loginShell = "exec '/bin/bash' -l"
	const nonResumeWant = "claude; stty sane; printf '\\033[?1049l\\033[?25h\\033[0m\\033c'; " +
		"echo 'Agent exited. Dropping to shell...'; export TERM=xterm-256color; " + loginShell

	tests := []struct {
		name   string
		cfg    config.AssistantConfig
		resume bool
		want   string
	}{
		{
			name:   "resume with configured args uses a timed fail-soft guard",
			cfg:    config.AssistantConfig{Command: "claude", ResumeArgs: "--continue"},
			resume: true,
			want: `s=$(date +%s); claude --continue; rc=$?; e=$(date +%s); if [ "$rc" -ne 0 ] && [ $((e-s)) -lt 5 ]; then claude; fi` +
				"; stty sane; printf '\\033[?1049l\\033[?25h\\033[0m\\033c'; " +
				"echo 'Agent exited. Dropping to shell...'; export TERM=xterm-256color; " + loginShell,
		},
		{
			name:   "resume true with no configured resume args equals a fresh launch",
			cfg:    config.AssistantConfig{Command: "claude"},
			resume: true,
			want:   nonResumeWant,
		},
		{
			name:   "resume false never applies resume args even when configured",
			cfg:    config.AssistantConfig{Command: "claude", ResumeArgs: "--continue"},
			resume: false,
			want:   nonResumeWant,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentLaunchCommand(tt.cfg, loginShell, tt.resume)
			if got != tt.want {
				t.Fatalf("agentLaunchCommand() = %q, want %q", got, tt.want)
			}
		})
	}
}

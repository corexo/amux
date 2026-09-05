package center

import "testing"

// TestSanitizeAITitle pins the contract the tab bar depends on: whatever a
// helper command prints becomes at most aiTitleMaxLen printable, space-free
// runes, and "-" (or noise-only output) is rejected so the tab keeps its name.
func TestSanitizeAITitle(t *testing.T) {
	tests := map[string]struct{ in, want string }{
		"plain":            {"authfix", "authfix"},
		"trailing newline": {"authfix\n", "authfix"},
		"caps at 10 runes": {"refactor-the-parser", "refactor-t"},
		"strips quotes":    {"\"authfix\"\n", "authfix"},
		"first line only":  {"\n\nauthfix\nsome explanation", "authfix"},
		"drops spaces":     {"auth fix now", "authfixnow"},
		"dash is no idea":  {"-\n", ""},
		"empty":            {"", ""},
		"multibyte capped": {"überprüfungslauf", "überprüfun"},
		// JSON is what the default local model answers; a reasoning model's
		// monologue around the object must not become the title.
		"json title":          {"{\n  \"title\": \"docker\"\n}\n", "docker"},
		"json renamed key":    {"{\"task\": \"authfix\"}", "authfix"},
		"json dash":           {"{\"title\": \"-\"}", ""},
		"json after monolog":  {"We are given a transcript...\n{\"title\": \"pagination\"}", "pagination"},
		"json ambiguous keys": {"{\"answer\": \"-\", \"reason\": \"unclear\"}", ""},
		"json empty object":   {"{ }", ""},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := sanitizeAITitle(tc.in); got != tc.want {
				t.Fatalf("sanitizeAITitle(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if n := len([]rune(sanitizeAITitle(tc.in))); n > aiTitleMaxLen {
				t.Fatalf("sanitizeAITitle(%q) returned %d runes, want <= %d", tc.in, n, aiTitleMaxLen)
			}
		})
	}
}

// TestAITitleCommandDisabled confirms the off switch: an explicit "off" means
// no helper command, so no tick is ever armed.
func TestAITitleCommandDisabled(t *testing.T) {
	t.Setenv(aiTitleEnv, "off")
	if got := aiTitleCommand(); got != "" {
		t.Fatalf("aiTitleCommand() = %q, want empty when disabled", got)
	}

	t.Setenv(aiTitleEnv, "my-summarizer --stdin")
	if got := aiTitleCommand(); got != "my-summarizer --stdin" {
		t.Fatalf("aiTitleCommand() = %q, want the configured command", got)
	}
}

// TestRunAITitleUsesStdinTranscript exercises the exec path end-to-end with a
// shell stand-in for the AI: the transcript must reach the command on stdin,
// and its answer must come back sanitized.
func TestRunAITitleUsesStdinTranscript(t *testing.T) {
	got := runAITitle("grep -q 'fix the auth bug' && echo 'authbug' || echo -", "user: fix the auth bug\n")
	if got != "authbug" {
		t.Fatalf("runAITitle() = %q, want %q", got, "authbug")
	}
	if got := runAITitle("exit 1", "whatever"); got != "" {
		t.Fatalf("runAITitle() on failing command = %q, want empty", got)
	}
}

// TestAITitleEligible pins which tabs the sweep may retitle: only tabs still
// carrying the auto-generated name. An AI- or user-set name must survive,
// which is also what keeps a persisted title stable across restarts.
func TestAITitleEligible(t *testing.T) {
	tests := map[string]struct {
		tab  *Tab
		want bool
	}{
		"assistant name":       {&Tab{Name: "claude", Assistant: "claude"}, true},
		"numbered variant":     {&Tab{Name: "pi 2", Assistant: "pi"}, true},
		"empty name":           {&Tab{Assistant: "omp"}, true},
		"terminal placeholder": {&Tab{Name: "Terminal"}, true},
		"ai generated name":    {&Tab{Name: "authbug", Assistant: "claude"}, false},
		"user renamed":         {&Tab{Name: "pi refactor", Assistant: "pi"}, false},
		"nil tab":              {nil, false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := aiTitleEligible(tc.tab); got != tc.want {
				t.Fatalf("aiTitleEligible(%+v) = %v, want %v", tc.tab, got, tc.want)
			}
		})
	}
}

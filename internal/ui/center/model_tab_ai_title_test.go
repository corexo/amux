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

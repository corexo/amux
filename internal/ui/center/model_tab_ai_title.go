package center

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/andyrewlee/amux/internal/logging"
	"github.com/andyrewlee/amux/internal/tmux"
	"github.com/andyrewlee/amux/internal/ui/common"
)

// AI tab titles: a short (<= aiTitleMaxLen chars) label derived from what the
// agent in the tab is actually working on, instead of the static "claude" /
// "codex-2" name.
//
// ponytail: amux has no LLM client and gains none here. The summary comes from
// a helper command that reads the pane transcript on stdin and prints one line.
// AMUX_TITLE_CMD picks it ("off"/"none" disables); by default a local ollama
// model does the work (no tokens leave the machine) and the claude CLI is only
// the fallback, so the feature stays silent when neither exists.
const (
	aiTitleEnv          = "AMUX_TITLE_CMD"
	aiTitleLocalCmd     = "ollama run qwen3:4b --think=false --format json"
	aiTitleRemoteCmd    = "claude -p --model haiku"
	aiTitleMaxLen       = 10
	aiTitleDelay        = 20 * time.Second
	aiTitleSweepEvery   = 15 * time.Second
	aiTitleMaxAttempts  = 3
	aiTitleCaptureLines = 60
	aiTitleTimeout      = 45 * time.Second
	// The instructions follow the transcript: small local models weight the
	// tail of the prompt far more heavily, and a JSON answer keeps a reasoning
	// model's monologue out of the title (see sanitizeAITitle).
	aiTitlePromptHead = "Terminal transcript of a coding-agent session:\n"
	aiTitlePromptTail = "\n\nName the task in ONE lowercase word, max 10 letters. " +
		"Reply as JSON: {\"title\":\"...\"} — use {\"title\":\"-\"} if unclear.\n"
)

// aiTitleSweepTick drives the periodic scan for still-unnamed tabs. Titles are
// armed by this sweep rather than at tab creation, because tabs also arrive
// via session restore, tmux adoption and reattach — the sweep covers every one
// of those paths from a single place.
type aiTitleSweepTick struct{}

// aiTitleTick asks for a title attempt for one tab.
type aiTitleTick struct {
	WorkspaceID string
	TabID       TabID
}

// aiTitleResult carries the helper's answer back to the UI goroutine. An empty
// Title means "nothing usable yet" and schedules another attempt.
type aiTitleResult struct {
	WorkspaceID string
	TabID       TabID
	Title       string
}

// aiTitleCommand returns the configured helper command, or "" when AI titles
// are disabled or unavailable.
func aiTitleCommand() string {
	cmd := strings.TrimSpace(os.Getenv(aiTitleEnv))
	switch strings.ToLower(cmd) {
	case "off", "none", "0":
		return ""
	case "":
		if _, err := exec.LookPath("ollama"); err == nil {
			return aiTitleLocalCmd
		}
		if _, err := exec.LookPath("claude"); err == nil {
			return aiTitleRemoteCmd
		}
		return ""
	}
	return cmd
}

// scheduleAITitleSweep arms the next sweep. It always returns a command so the
// center's Init has one shape regardless of the environment; the sweep handler
// is where a disabled helper stops the loop.
func (m *Model) scheduleAITitleSweep() tea.Cmd {
	return common.SafeTick(aiTitleSweepEvery, func(time.Time) tea.Msg {
		return aiTitleSweepTick{}
	})
}

// updateAITitleSweep requests one title attempt per eligible tab and re-arms
// the sweep. Eligible means: still carrying its auto-generated name, at least
// aiTitleDelay old (so the agent has printed something), has a tmux session,
// and has not been attempted yet.
func (m *Model) updateAITitleSweep() tea.Cmd {
	if aiTitleCommand() == "" {
		return nil
	}
	cmds := []tea.Cmd{m.scheduleAITitleSweep()}
	now := time.Now()
	for wsID, tabs := range m.tabs.ByWorkspace {
		for _, tab := range tabs {
			if !aiTitleEligible(tab) {
				continue
			}
			tab.mu.Lock()
			attempts, createdAt, session := tab.aiTitleAttempts, tab.createdAt, tab.SessionName
			tab.mu.Unlock()
			if attempts > 0 || session == "" {
				continue
			}
			if createdAt > 0 && now.Sub(time.Unix(createdAt, 0)) < aiTitleDelay {
				continue
			}
			wsID, tabID := wsID, tab.ID
			cmds = append(cmds, func() tea.Msg {
				return aiTitleTick{WorkspaceID: wsID, TabID: tabID}
			})
		}
	}
	return common.SafeBatch(cmds...)
}

// aiTitleEligible reports whether a tab still carries the auto-generated name
// ("claude", "pi 2", "Terminal") and may therefore be retitled. A name that
// does not match that shape was set by the AI (or by the user) and is left
// alone, which also keeps a persisted AI title stable across restarts.
func aiTitleEligible(tab *Tab) bool {
	if tab == nil || tab.isClosed() {
		return false
	}
	name := strings.TrimSpace(tab.Name)
	assistant := strings.TrimSpace(tab.Assistant)
	if name == "" || name == "Terminal" {
		return true
	}
	if assistant == "" || name == assistant {
		return name == assistant
	}
	suffix, ok := strings.CutPrefix(name, assistant+" ")
	if !ok {
		return false
	}
	_, err := strconv.Atoi(suffix)
	return err == nil
}

// scheduleAITitle re-arms a single tab's attempt after an inconclusive answer.
func (m *Model) scheduleAITitle(wsID string, tabID TabID) tea.Cmd {
	if aiTitleCommand() == "" {
		return nil
	}
	return common.SafeTick(aiTitleDelay, func(time.Time) tea.Msg {
		return aiTitleTick{WorkspaceID: wsID, TabID: tabID}
	})
}

// updateAITitleTick captures the pane transcript and hands it to the helper
// command off the UI goroutine. Attempts are counted here (not on the result)
// so a hung or failing helper can never loop forever.
func (m *Model) updateAITitleTick(msg aiTitleTick) tea.Cmd {
	cmdStr := aiTitleCommand()
	if cmdStr == "" {
		return nil
	}
	tab := m.getTabByID(msg.WorkspaceID, msg.TabID)
	if tab == nil || tab.isClosed() {
		return nil
	}
	tab.mu.Lock()
	session := tab.SessionName
	attempt := tab.aiTitleAttempts
	tab.aiTitleAttempts++
	tab.mu.Unlock()
	if session == "" || attempt >= aiTitleMaxAttempts {
		return nil
	}

	wsID, tabID, opts := msg.WorkspaceID, msg.TabID, m.tmuxOpts
	return func() tea.Msg {
		transcript, ok := tmux.CapturePaneTail(session, aiTitleCaptureLines, opts)
		if !ok || strings.TrimSpace(transcript) == "" {
			return aiTitleResult{WorkspaceID: wsID, TabID: tabID}
		}
		return aiTitleResult{WorkspaceID: wsID, TabID: tabID, Title: runAITitle(cmdStr, transcript)}
	}
}

// runAITitle runs the helper command with the transcript on stdin and returns
// the sanitized title, or "" on any failure (the tab keeps its current name).
func runAITitle(cmdStr, transcript string) string {
	ctx, cancel := context.WithTimeout(context.Background(), aiTitleTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Stdin = strings.NewReader(aiTitlePromptHead + transcript + aiTitlePromptTail)
	out, err := cmd.Output()
	if err != nil {
		logging.Warn("AI tab title command %q failed: %v", cmdStr, err)
		return ""
	}
	return sanitizeAITitle(string(out))
}

// sanitizeAITitle reduces a helper's answer to a tab label: the JSON "title"
// field when the helper answered JSON (the default local model does, which is
// what keeps a reasoning model's monologue out of the title), otherwise the
// first non-empty line. Quotes, control characters and whitespace are dropped
// and the result is capped at aiTitleMaxLen runes. "" means unusable ("-" is
// the helper's "no idea yet").
func sanitizeAITitle(s string) string {
	line, isJSON := jsonAITitle(s)
	if !isJSON {
		for _, candidate := range strings.Split(s, "\n") {
			if line = strings.TrimSpace(candidate); line != "" {
				break
			}
		}
	}
	line = strings.Trim(line, "\"'`*")

	var title []rune
	for _, r := range line {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			continue
		}
		title = append(title, r)
		if len(title) == aiTitleMaxLen {
			break
		}
	}
	if out := string(title); out != "-" {
		return out
	}
	return ""
}

// jsonAITitle pulls the title out of a JSON answer. It matches fields textually
// rather than decoding, because a model that hits its token budget emits a
// valid-looking but unterminated object ({"title": "docker" with no closing
// brace) that no JSON decoder accepts. It also tolerates a renamed key (small
// models rename "title" freely) and prose wrapped around the object. The bool
// reports whether the answer was JSON at all: when it was, its raw text must
// never be used as a fallback title.
func jsonAITitle(s string) (string, bool) {
	if !strings.Contains(s, "{") {
		return "", false
	}
	if match := aiTitleJSONTitleField.FindStringSubmatch(s); match != nil {
		return match[1], true
	}
	// A renamed key only counts when it is the object's single string field —
	// otherwise (e.g. {"answer":"-","reason":"unclear"}) the pick would be
	// arbitrary.
	if fields := aiTitleJSONStringField.FindAllStringSubmatch(s, 2); len(fields) == 1 {
		return fields[0][1], true
	}
	return "", true
}

var (
	aiTitleJSONTitleField  = regexp.MustCompile(`"title"\s*:\s*"([^"]*)"`)
	aiTitleJSONStringField = regexp.MustCompile(`"[^"]*"\s*:\s*"([^"]*)"`)
)

// updateAITitleResult applies a title, or re-arms the tick when the helper had
// nothing usable yet.
func (m *Model) updateAITitleResult(msg aiTitleResult) tea.Cmd {
	tab := m.getTabByID(msg.WorkspaceID, msg.TabID)
	if tab == nil || tab.isClosed() {
		return nil
	}
	if msg.Title == "" {
		return m.scheduleAITitle(msg.WorkspaceID, msg.TabID)
	}
	tab.mu.Lock()
	tab.Name = msg.Title
	tab.mu.Unlock()
	m.noteTabsChanged()
	return nil
}

# Configuration: assistants

amux ships a built-in roster of AI coding agents, but you are not limited to it.
The user config file lets you **override a built-in's launch command** or **add
a brand-new assistant** (for example a company-internal CLI or a tool amux does
not know about yet). This document describes that `assistants` config.

## Where the config lives

amux reads a single user config file at:

```
~/.amux/config.json
```

The file is optional. A missing file, malformed JSON, or a broken `assistants`
section falls back to the built-in defaults; a valid `assistants` section is
merged on top of them. (This is per-user global config, distinct from the
per-project `.amux/workspaces.json` described in the README.)

## The `assistants` map

The config schema has an `assistants` object. Each **key** is the assistant
name; each **value** overrides that assistant's launch settings:

```json
{
  "assistants": {
    "mytool": { "command": "mytool --interactive", "interrupt_count": 2, "interrupt_delay_ms": 100 }
  }
}
```

The value fields (all optional) are:

| JSON key             | Type   | Meaning                                                              |
|----------------------|--------|---------------------------------------------------------------------|
| `command`            | string | Shell command amux runs to launch the assistant.                    |
| `interrupt_count`    | number | Number of Ctrl-C signals amux sends to interrupt the agent.         |
| `interrupt_delay_ms` | number | Delay, in milliseconds, between those Ctrl-C signals.               |
| `resume_args`        | string | Args appended to `command` to resume a prior conversation on restore (see [Resuming a prior conversation on restore](#resuming-a-prior-conversation-on-restore)). |
| `hidden`             | bool   | Removes this assistant from the `+new` picker (see [Hiding an assistant from the picker](#hiding-an-assistant-from-the-picker)). |

Defaults applied when a value is kept: `interrupt_count` falls back to `1` if it
is missing or not positive, and `interrupt_delay_ms` falls back to `0` if it is
missing or negative.

Assistant names must start with a letter or number and may contain only letters,
numbers, dots, dashes, or underscores (max 100 characters). Names are matched
case-insensitively (they are lowercased). An entry whose name fails validation
is ignored.

## Adding a custom assistant

Give the new key a **non-empty `command`**. That is the only requirement — a new
name with a command becomes a real, usable assistant:

```json
{
  "assistants": {
    "mytool": { "command": "mytool --interactive" }
  }
}
```

After this, `mytool`:

- **appears in the assistant picker** (the agent-selection dialog), listed after
  the built-in agents;
- is **treated as a chat agent**, exactly like the built-ins.

A custom entry **without** a `command` is dropped (there would be nothing to
launch), so always include one for a new name.

### Caveat: custom assistants have no brand color

The built-in agents each render with a dedicated brand color. A custom
(non-built-in) assistant does **not** get one — it falls back to the default
primary color. This is purely cosmetic; the assistant is fully functional
otherwise.

## Overriding a built-in's command

The same map overrides the built-in agents. For a built-in you only need to set
the field(s) you want to change — the command keeps its built-in default unless
you provide one. For example, to launch `claude` through a wrapper while keeping
its interrupt behavior:

```json
{
  "assistants": {
    "claude": { "command": "my-claude-wrapper" }
  }
}
```

## Resuming a prior conversation on restore

Some assistants can resume a previous conversation instead of starting blank.
The optional `resume_args` field names the flag(s) amux appends to `command`
to do that:

```json
{
  "assistants": {
    "codex": { "command": "codex", "resume_args": "resume --last" }
  }
}
```

This only ever applies when amux restores a persisted tab and finds its tmux
session gone — the ordinary shape of a machine or WSL restart, where the tmux
server (and every session in it) is gone but the tab list is still on disk.
Reattaching to a tab whose session is still alive, opening a new tab, and an
explicit restart all keep launching the assistant fresh, exactly as before.

The resume attempt is fail-soft: amux runs `<command> <resume_args>`, and only
falls back to a plain `<command>` launch if that exits almost immediately —
there was nothing to resume yet (e.g. a tab that was never actually used). A
resumed session that runs for a while and then exits normally, including the
user quitting it with Ctrl-C, drops to the shell prompt instead of silently
spawning another agent.

Built-in defaults: `claude`, `pi`, `omp`, `antigravity`, and `opencode` ship a
`resume_args` value (`--continue` or the assistant's equivalent); the
remaining built-ins (`codex`, `droid`, `cursor`, `fx`, `grok`, `amp`, `cline`)
ship none and always launch fresh. An override that omits `resume_args` keeps
the built-in default; setting it to `""` explicitly disables resume for that
assistant.

A custom (non-built-in) assistant gets resume behavior purely from its own
config entry — there is no built-in default to fall back to:

```json
{
  "assistants": {
    "gemini": { "command": "gemini", "resume_args": "--resume latest" }
  }
}
```

The built-in roster (default names) is: `claude`, `codex`, `opencode`, `droid`,
`cursor`, `pi`, `omp`, `antigravity`, `fx`, `grok`, `amp`, `cline`.

## Hiding an assistant from the picker

If you have an assistant configured (built-in or custom) whose CLI you don't
actually have installed, set `hidden` to keep it out of the `+new` agent
picker without deleting its config entry:

```json
{
  "assistants": {
    "codex": { "hidden": true }
  }
}
```

`hidden` only removes the entry from the `+new` picker. It does **not**
deactivate the assistant: an existing tab already running that assistant
keeps working exactly as before, and restoring a tab after a restart still
works even if its assistant is hidden. If hiding would leave the picker with
no real assistant at all, amux falls back to showing the full (unfiltered)
roster rather than an empty picker. Defaults to `false` (visible) when
omitted.

## AI tab titles (`AMUX_TITLE_CMD`)

A new agent tab starts with a static name (`claude`, `codex-2`). About 20
seconds in, amux captures the last 60 lines of the tab's tmux pane, pipes them
to a helper command on stdin, and uses the first line it prints as the tab
title — capped at **10 characters**, whitespace and quotes stripped. A helper
that answers `-` (or fails) leaves the name alone and is retried up to 3 times.

| `AMUX_TITLE_CMD`      | Behavior                                               |
|-----------------------|--------------------------------------------------------|
| unset                 | `claude -p` when the `claude` CLI is on PATH, else off  |
| `off` / `none` / `0`  | disabled                                               |
| any command           | run via `sh -c`, transcript on stdin, title on stdout  |

```sh
AMUX_TITLE_CMD="codex exec -" amux     # use a different agent CLI
AMUX_TITLE_CMD=off amux                # no AI calls at all
```

Titles are persisted with the tab, so a reattached tab keeps its generated
name and no further AI calls are made for it.

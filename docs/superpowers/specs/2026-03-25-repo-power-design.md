# Repo Power Design

Date: 2026-03-25
Status: Draft approved in-session, written for implementation planning

## Summary

GistClaw needs a first-party repo toolbelt so agents can inspect, edit, test, build, and reason about a software repository without depending on MCP for core coding behavior. The repo-power slice adds typed built-in tools under the existing `internal/tools` seam, keeps authority bounded to `WorkspaceRoot`, and routes all side effects through the current approval, journaling, and replay pipeline.

The goal is to make GistClaw feel as practically capable as Deer Flow and OpenClaw on repo tasks, while staying stricter on safety, replayability, and operator trust.

## Goals

- Give agents strong built-in repo tools for everyday coding work.
- Keep the runtime kernel unchanged; tools remain extensions plugged into the current seam.
- Enforce `WorkspaceRoot` as the hard authority boundary for the first-party repo toolbelt.
- Make approvals depend on effect class, not hand-wavy tool naming.
- Preserve replay, auditability, and readable receipts for all repo actions.
- Keep MCP as an overlay, not the primary repo-power dependency.

## Non-Goals

- No arbitrary host-path access in this slice.
- No `gh`-first GitHub workflow in the core toolbelt.
- No broad marketplace or plugin-runtime expansion.
- No compatibility layer for legacy OpenClaw/Deer Flow tool contracts.
- No silent full-file overwrite for existing files.
- No prompt-only safety model.

## Architecture

The repo-power toolbelt lives entirely inside the existing `internal/tools` seam. The runtime continues to pass tool specs into providers and execute returned tool calls through the registry. The runtime does not learn file, git, or shell semantics directly.

Built-in repo tools are registered alongside the existing first-party research tools and MCP-loaded tools. The tool registry remains the single entry point for tool discovery and invocation.

The authority boundary is `WorkspaceRoot` only. Every path-bearing tool must normalize and validate requested paths against the run’s workspace root before any read or write occurs. Commands executed through `shell_exec`, `run_tests`, or `run_build` run with a working directory inside `WorkspaceRoot`.

## Tool Catalog

### Read tools

- `list_dir(path)`
- `read_file(path, start_line?, end_line?, max_bytes?)`
- `grep_search(query, path?, glob?, max_matches?)`

### Edit tools

- `apply_patch(patch)`
- `write_new_file(path, content)`
- `delete_path(path)`
- `move_path(from, to)`

### Repo inspection tools

- `git_status()`
- `git_diff(target?)`
- `git_show(target)`
- `git_log(limit?)`

### Execution tools

- `shell_exec(command, cwd?, timeout_sec?)`
- `run_tests(target?)`
- `run_build(target?)`

## Why Typed Tools Instead Of Shell-Only

The repo-power slice deliberately avoids collapsing everything into raw shell. A pure shell model gives the LLM too much surface area and produces weak approvals, weak receipts, and weak auditability. Typed tools provide stronger affordances:

- better prompts and planning for the model
- clearer approvals for operators
- cleaner receipts for replay and debugging
- room for tool-specific safety checks

`shell_exec` is included from day one, but it is an escape hatch, not the primary interface for common repo actions.

## Authority Model

All first-party repo tools are restricted to `WorkspaceRoot`.

Rules:

- Every requested path is resolved against `WorkspaceRoot`.
- Escapes via `..`, absolute path tricks, symlinks, or path normalization failures are rejected.
- `cwd` for `shell_exec`, `run_tests`, and `run_build` must resolve inside `WorkspaceRoot`.
- Existing-file edits may only target files under `WorkspaceRoot`.
- New-file creation may only create files under `WorkspaceRoot`.

Future host-wide tooling, if ever needed, should be separate higher-risk tools such as `host_read` or `host_shell`, not a looser version of the same repo toolbelt.

## Edit Model

The first slice is patch-first.

- Existing file edits go through `apply_patch`.
- New files are created through `write_new_file`.
- `delete_path` and `move_path` are explicit tools with stronger approval requirements.
- Whole-file overwrite for existing files is intentionally excluded.

This keeps edits legible, composable, and approval-friendly.

## Effect Classes

Policy is keyed by effect class rather than tool name. The initial effect classes are:

- `read`
- `exec_read`
- `exec_write`
- `patch`
- `create`
- `delete`
- `move`

Examples:

- `list_dir`, `read_file`, `grep_search`, and read-only git tools are `read`
- `run_tests` and most default `run_build` invocations are `exec_read`
- mutating `shell_exec` is `exec_write`
- `apply_patch` is `patch`
- `write_new_file` is `create`
- `delete_path` is `delete`
- `move_path` is `move`

## Approval Policy

Recommended baseline:

- allow without approval:
  - `read`
  - read-only git tools
  - `grep_search`
  - `run_tests`
  - `run_build`
  - read-only `shell_exec`
- require approval:
  - `patch`
  - `create`
  - `delete`
  - `move`
  - mutating `shell_exec`

The system should not trust the tool name alone. `shell_exec` must be risk-tiered by policy or execution mode, not assumed safe because it was called from a “test” context.

## Receipts And Replay

Repo tools must preserve readable receipts and strong replay evidence.

Receipts should capture:

- tool name
- normalized target path(s) when applicable
- resolved working directory for exec tools
- command string for exec tools
- exit code
- timeout state
- stdout/stderr summaries with bounded truncation
- whether output was truncated

Replay remains evidence-based rather than intent-based. For example, an exec receipt should include the actual observed command result, not only that a command was attempted.

## Guardrails

- Path normalization and symlink validation are hard checks in tool code.
- Reads and command outputs are size-bounded and explicitly truncated.
- `apply_patch` rejects targets outside root and malformed patches.
- `write_new_file` rejects overwriting existing files.
- `delete_path` and `move_path` reject root escape and invalid target states.
- `run_tests` and `run_build` are thin wrappers over the shared exec runner with known defaults, not arbitrary shell aliases.

## Implementation Shape

Planned code layout:

- `internal/tools/path_guard.go`
- `internal/tools/effects.go`
- `internal/tools/fs_list.go`
- `internal/tools/fs_read.go`
- `internal/tools/fs_write.go`
- `internal/tools/fs_move.go`
- `internal/tools/fs_delete.go`
- `internal/tools/patch.go`
- `internal/tools/git_status.go`
- `internal/tools/git_diff.go`
- `internal/tools/git_show.go`
- `internal/tools/git_log.go`
- `internal/tools/exec.go`
- `internal/tools/repo_tools.go`
- `internal/tools/repo_tools_test.go`

Likely supporting updates:

- `internal/tools/policy.go`
- `internal/app/bootstrap.go` only if registry wiring needs to be restructured

## Delivery Order

### Milestone A

- path guard
- effect classification
- `list_dir`
- `read_file`
- `grep_search`

### Milestone B

- `apply_patch`
- `write_new_file`
- `delete_path`
- `move_path`

### Milestone C

- shared exec runner
- `shell_exec`
- `run_tests`
- `run_build`

### Milestone D

- `git_status`
- `git_diff`
- `git_show`
- `git_log`
- policy matrix hardening
- documentation updates

## Testing Strategy

The repo-power slice requires test-first coverage at four levels:

### Path guard tests

- reject `..` escapes
- reject absolute paths outside root
- reject symlink escapes
- accept valid in-root paths

### Tool contract tests

- read tools return bounded output
- patch/create/delete/move tools enforce path and state rules
- exec tools capture exit code, stdout, stderr, and timeout
- git tools return structured outputs without mutating the repo

### Policy tests

- each effect class maps to the expected approval outcome
- mutating exec is treated differently from read-only exec

### Runtime integration tests

- provider receives the built-in repo tool specs
- runtime executes the tool and journals the result
- approvals still gate mutating effects
- receipts are persisted with usable summaries

## Open Follow-Ups

- Add `gh` or GitHub MCP as a later overlay, not in the first repo-power slice.
- Evaluate whether `run_tests` and `run_build` should learn repo-specific defaults from config after the core toolbelt is stable.
- Consider later host-level escape-hatch tools only as distinct high-risk tools.

## Recommendation

Implement the repo-power slice as a first-party built-in toolbelt with strict `WorkspaceRoot` enforcement and patch-first editing. This earns the “agent can code” baseline without weakening the kernel’s safety and replay model.

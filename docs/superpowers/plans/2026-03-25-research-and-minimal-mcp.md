# Research And Minimal MCP Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add first-party research tools and a minimal static-config MCP tool source to GistClaw without changing the session kernel.

**Architecture:** Tool execution stays in the existing runtime/tool registry seam. First-party research tools (`web_search`, `web_fetch`) and MCP-discovered tools are all normal `internal/tools.Tool` implementations, journaled and governed by the same policy path. MCP stays startup-loaded from config with explicit allowlists and risk metadata; no marketplace, dynamic install/update UX, or separate plugin runtime.

**Tech Stack:** Go 1.24+, stdlib `net/http`, current SQLite journal/projection stack, YAML config, JSON-RPC over stdio for MCP, `go test`.

---

## Scope Check

Keep this as one plan because research and MCP share the same foundation:

- provider tool-call round-trip
- runtime tool execution
- registry/bootstrap/config wiring
- policy and doctor readiness

If MCP transport work starts to sprawl, split only the transport layer into a follow-up plan. Do not split research away from the tool-execution foundation.

## File Structure

**Create:**

- `internal/tools/register.go` — central built-in tool registration plus MCP tool loading entrypoint.
- `internal/tools/web_search.go` — first-party `web_search` tool backed by one concrete search backend.
- `internal/tools/web_fetch.go` — first-party `web_fetch` tool with bounded HTTP fetch and readable text extraction.
- `internal/tools/http_client.go` — shared outbound HTTP client, timeouts, byte limits, scheme checks.
- `internal/tools/mcp_client.go` — stdio JSON-RPC lifecycle for `initialize`, `tools/list`, and `tools/call`.
- `internal/tools/mcp_tool.go` — `Tool` wrapper for one configured MCP tool.
- `internal/tools/mcp_test.go` — MCP loader, allowlist, and invocation coverage.
- `teams/default/researcher.soul.yaml` — default research specialist prompt.

**Modify:**

- `internal/runtime/runs.go` — pass tool specs to providers, execute tool calls, journal tool outcomes, feed tool results back into subsequent provider turns.
- `internal/conversations/service.go` — add projections for tool-call and approval events into `tool_calls` and `approvals`.
- `internal/model/types.go` — add typed payload structs or event-facing helper types for tool calls/results if needed.
- `internal/providers/anthropic/provider.go` — serialize prior tool calls and tool results back into Anthropic message format.
- `internal/providers/openai/provider.go` — serialize prior tool calls and tool results back into OpenAI chat/responses format.
- `internal/tools/policy.go` — classify built-in research tools as low risk and deny unconfigured MCP tools by default.
- `internal/app/config.go` — add `research` and `mcp` config blocks plus validation/defaults.
- `internal/app/bootstrap.go` — register built-in tools, load MCP tools at startup, fail fast on bad config.
- `internal/app/bootstrap_test.go` — startup wiring tests for research and MCP tool loading.
- `internal/app/config_test.go` — config validation/default coverage for research and MCP.
- `internal/app/team_validation.go` — keep team validation aligned if the default team adds a researcher agent.
- `cmd/gistclaw/doctor.go` — add optional checks for research backend readiness and configured MCP servers.
- `cmd/gistclaw/doctor_test.go` — doctor output coverage for research and MCP checks.
- `teams/default/team.yaml` — add `researcher` as a specialist worker.

**Test:**

- `internal/runtime/runs_test.go`
- `internal/runtime/starter_workflow_test.go`
- `internal/tools/tools_test.go`
- `internal/tools/mcp_test.go`
- `internal/app/config_test.go`
- `internal/app/bootstrap_test.go`
- `cmd/gistclaw/doctor_test.go`

## Locked Product Decisions For This Slice

- `web_search` and `web_fetch` are built-in tools, not MCP dependencies.
- MCP is a tool source, not a second runtime.
- Only explicitly configured MCP tools may load.
- Every MCP tool needs static local metadata: alias, risk, and enabled state.
- Milestone 1 only allows read-oriented MCP tools. Write-capable MCP tools are deferred.
- The runtime kernel stays unchanged. No new session or collaboration primitive is introduced here.

## Config Shape

Use a narrow config surface that can grow later without reopening the kernel:

```yaml
research:
  provider: tavily
  api_key: tvly-...
  max_results: 5
  timeout_sec: 10

mcp:
  servers:
    - id: github
      transport: stdio
      command: ["uvx", "mcp-server-github"]
      env:
        GITHUB_TOKEN: ${GITHUB_TOKEN}
      tools:
        - name: search_repositories
          alias: github_search_repositories
          risk: low
          enabled: true
        - name: get_file_contents
          alias: github_get_file_contents
          risk: low
          enabled: true
```

Rules:

- reject unknown `research.provider`
- require `research.api_key` for providers that need one
- reject MCP servers with empty `id`, empty `command`, or duplicate tool aliases
- reject MCP tools without explicit `alias` and `risk`
- reject MCP tools marked `risk: medium|high` in this slice

### Task 1: Build The Real Tool-Call Foundation

**Files:**

- Modify: `internal/runtime/runs.go`
- Modify: `internal/conversations/service.go`
- Modify: `internal/providers/anthropic/provider.go`
- Modify: `internal/providers/openai/provider.go`
- Modify: `internal/model/types.go`
- Test: `internal/runtime/runs_test.go`
- Test: `internal/runtime/starter_workflow_test.go`

- [ ] **Step 1: Write failing runtime tests for tool execution**

Add table-driven tests that prove all of the following:

- provider receives non-empty `ToolSpecs`
- when provider returns `ToolCalls`, the runtime looks up and invokes the tool
- the next provider turn sees prior tool results
- preview-only runs still do not apply workspace mutations

Use a stub tool like:

```go
type recordingTool struct {
	name   string
	result string
	calls  []model.ToolCall
}
```

- [ ] **Step 2: Run the targeted runtime tests and verify failure**

Run: `go test ./internal/runtime -run 'TestRunExecutesToolCalls|TestStarterWorkflow_PreviewOnly'`

Expected: FAIL because `runs.go` currently ignores `GenerateResult.ToolCalls`.

- [ ] **Step 3: Add journal event shapes for tool calls and approvals**

Project the following event families through `ConversationStore.AppendEvent`:

- `tool_call_recorded`
- `approval_requested`
- `approval_resolved`

Use payloads shaped like:

```go
type toolCallRecordedPayload struct {
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	InputJSON  json.RawMessage `json:"input_json"`
	OutputJSON json.RawMessage `json:"output_json"`
	Decision   string          `json:"decision"`
	ApprovalID string          `json:"approval_id"`
}
```

Do not write directly to `tool_calls` from `runs.go`.

- [ ] **Step 4: Teach the runtime loop to execute low-risk tools**

In `internal/runtime/runs.go`:

- pass `ToolSpecs: r.tools.List()` into `provider.Generate`
- for each returned `ToolCallRequest`, load the registered tool
- decide policy through `tools.Policy`
- on `allow`, invoke the tool and append `tool_call_recorded`
- on `deny`, append `tool_call_recorded` with denied decision and a tool error payload
- on `ask`, append `approval_requested`, mark the run interrupted or `needs_approval`, and stop the turn cleanly

Keep the first implementation boring. One tool-call round per provider turn is acceptable if that keeps the provider adapters simple.

- [ ] **Step 5: Feed prior tool results back into provider context**

Extend the provider event-to-message conversion so prior tool calls/results survive the next `Generate` call:

- Anthropic: emit prior `tool_use` and matching `tool_result`
- OpenAI: emit assistant tool-call messages plus tool-result messages

Do not flatten tool results into plain text assistant prose. Preserve tool identity.

- [ ] **Step 6: Re-run runtime tests**

Run: `go test ./internal/runtime -run 'TestRunExecutesToolCalls|TestStarterWorkflow_PreviewOnly'`

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/runtime/runs.go internal/conversations/service.go internal/providers/anthropic/provider.go internal/providers/openai/provider.go internal/model/types.go internal/runtime/runs_test.go internal/runtime/starter_workflow_test.go
git commit -m "feat: add journal-backed runtime tool execution"
```

### Task 2: Add First-Party Research Tools

**Files:**

- Create: `internal/tools/http_client.go`
- Create: `internal/tools/web_search.go`
- Create: `internal/tools/web_fetch.go`
- Modify: `internal/tools/policy.go`
- Test: `internal/tools/tools_test.go`

- [ ] **Step 1: Write failing tool tests**

Add tests for:

- `web_search` rejects empty query
- `web_search` returns normalized result objects with title, URL, snippet
- `web_fetch` rejects unsupported schemes
- `web_fetch` truncates oversized bodies
- `web_fetch` extracts readable text from HTML

Use `httptest.Server` for fetch tests. For search tests, stub the backend interface instead of hitting the network.

- [ ] **Step 2: Run the targeted tool tests and verify failure**

Run: `go test ./internal/tools -run 'TestWebSearch|TestWebFetch'`

Expected: FAIL because the tool implementations do not exist.

- [ ] **Step 3: Implement a shared outbound HTTP client**

Create one reusable helper with:

- request timeout
- max response bytes
- user-agent string
- allowed schemes `http` and `https`

Example signature:

```go
func newBoundedHTTPClient(timeout time.Duration) *http.Client
```

- [ ] **Step 4: Implement `web_search` with a narrow backend interface**

Keep the first backend explicit and simple:

```go
type SearchBackend interface {
	Search(ctx context.Context, query string, maxResults int) ([]SearchResult, error)
}
```

Use one concrete backend in this slice. `tavily` is the practical default because it has a clean HTTP API and avoids scraping instability. Do not add multiple first-party search providers yet.

- [ ] **Step 5: Implement `web_fetch`**

Requirements:

- fetch one URL
- cap bytes read
- preserve status code and content type in output
- extract readable text from HTML enough for model use
- treat redirects and fetch errors as structured tool errors, not panics

- [ ] **Step 6: Mark research tools low-risk**

Update `tools.Policy` so:

- `web_search` is `RiskLow`
- `web_fetch` is `RiskLow`

No approval flow for these tools in this slice.

- [ ] **Step 7: Re-run tool tests**

Run: `go test ./internal/tools -run 'TestWebSearch|TestWebFetch'`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/tools/http_client.go internal/tools/web_search.go internal/tools/web_fetch.go internal/tools/policy.go internal/tools/tools_test.go
git commit -m "feat: add first-party research tools"
```

### Task 3: Add Minimal Static-Config MCP Loading

**Files:**

- Create: `internal/tools/mcp_client.go`
- Create: `internal/tools/mcp_tool.go`
- Create: `internal/tools/mcp_test.go`
- Modify: `internal/app/config.go`
- Modify: `internal/app/bootstrap.go`
- Modify: `internal/tools/register.go`
- Test: `internal/app/config_test.go`
- Test: `internal/app/bootstrap_test.go`

- [ ] **Step 1: Write failing config and MCP loader tests**

Add tests for:

- unknown MCP transport rejected
- missing command rejected
- duplicate aliases rejected
- medium/high-risk MCP tools rejected
- bootstrap loads configured low-risk MCP tools into the registry

- [ ] **Step 2: Run the targeted MCP tests and verify failure**

Run: `go test ./internal/app ./internal/tools -run 'TestMCP|TestConfig'`

Expected: FAIL because MCP config and loader do not exist.

- [ ] **Step 3: Add explicit MCP config structs**

Add narrow config models:

```go
type MCPConfig struct {
	Servers []MCPServerConfig `yaml:"servers"`
}

type MCPServerConfig struct {
	ID        string            `yaml:"id"`
	Transport string            `yaml:"transport"`
	Command   []string          `yaml:"command"`
	Env       map[string]string `yaml:"env"`
	Tools     []MCPToolConfig   `yaml:"tools"`
}
```

Do not add dynamic discovery config, install sources, or marketplace fields.

- [ ] **Step 4: Implement a stdio MCP client**

Support only what the slice needs:

- process start/stop
- `initialize`
- `tools/list`
- `tools/call`

Assume stdio transport only in Milestone 1. Reject `sse` and `http` until later.

- [ ] **Step 5: Wrap configured MCP tools as normal registry tools**

Rules:

- only tools listed in local config may register
- registry name is the local alias, not raw remote name
- each wrapper exposes `ToolSpec` using configured risk and description
- each wrapper returns structured JSON output

- [ ] **Step 6: Add bootstrap registration**

In `internal/app/bootstrap.go`, replace ad-hoc empty registry wiring with one registration path:

```go
reg, err := tools.BuildRegistry(cfg)
```

Bootstrap should fail fast if configured MCP servers cannot initialize.

- [ ] **Step 7: Re-run MCP and config tests**

Run: `go test ./internal/app ./internal/tools -run 'TestMCP|TestConfig|TestBootstrap'`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/tools/mcp_client.go internal/tools/mcp_tool.go internal/tools/mcp_test.go internal/tools/register.go internal/app/config.go internal/app/bootstrap.go internal/app/config_test.go internal/app/bootstrap_test.go
git commit -m "feat: load configured mcp tools into registry"
```

### Task 4: Wire Doctor And Safe Defaults

**Files:**

- Modify: `cmd/gistclaw/doctor.go`
- Modify: `cmd/gistclaw/doctor_test.go`
- Modify: `internal/app/config.go`
- Modify: `internal/tools/policy.go`

- [ ] **Step 1: Write failing doctor tests**

Cover:

- configured research backend without API key shows `FAIL`
- configured MCP server with missing binary shows `WARN` or `FAIL` consistently
- no configured research or MCP keeps checks skipped

- [ ] **Step 2: Run the targeted doctor tests and verify failure**

Run: `go test ./cmd/gistclaw -run 'TestDoctor'`

Expected: FAIL because doctor does not check research or MCP yet.

- [ ] **Step 3: Extend `doctor` with optional research and MCP readiness checks**

Add:

- `research` check when `research.provider` is configured
- `mcp:<server-id>` check for each configured server

Suggested behavior:

- config shape problems: `FAIL`
- missing binary or handshake issue: `WARN`
- healthy configuration and initialization: `PASS`

- [ ] **Step 4: Default-deny unconfigured MCP tool surfaces**

Even if an MCP server returns extra tools from `tools/list`, they must not become visible unless explicitly configured. Test that behavior.

- [ ] **Step 5: Re-run doctor tests**

Run: `go test ./cmd/gistclaw -run 'TestDoctor'`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add cmd/gistclaw/doctor.go cmd/gistclaw/doctor_test.go internal/app/config.go internal/tools/policy.go
git commit -m "feat: add research and mcp doctor checks"
```

### Task 5: Add The Default Researcher Worker

**Files:**

- Create: `teams/default/researcher.soul.yaml`
- Modify: `teams/default/team.yaml`
- Modify: `internal/app/team_validation.go`
- Test: `internal/app/team_validation_test.go`

- [ ] **Step 1: Write failing team validation tests**

Add a table-driven case proving the default team remains valid after introducing a `researcher` agent and that missing `researcher.soul.yaml` fails validation cleanly.

- [ ] **Step 2: Run the targeted team validation tests and verify failure**

Run: `go test ./internal/app -run 'TestValidateTeamDir|TestTeamValidation'`

Expected: FAIL until the new soul file and test cases land.

- [ ] **Step 3: Add the researcher worker**

Update `teams/default/team.yaml` so `assistant` may spawn and message `researcher`, and add a dedicated soul file with these traits:

- read-heavy
- citation-friendly
- does not mutate workspace
- summarizes findings back to the coordinator

- [ ] **Step 4: Re-run team validation tests**

Run: `go test ./internal/app -run 'TestValidateTeamDir|TestTeamValidation'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add teams/default/team.yaml teams/default/researcher.soul.yaml internal/app/team_validation.go internal/app/team_validation_test.go
git commit -m "feat: add default researcher worker"
```

### Task 6: End-To-End Verification

**Files:**

- Modify as needed based on failures from prior tasks.

- [ ] **Step 1: Run focused package tests**

Run:

```bash
go test ./internal/runtime ./internal/tools ./internal/app ./cmd/gistclaw
```

Expected: PASS.

- [ ] **Step 2: Run the full test suite**

Run:

```bash
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Run coverage**

Run:

```bash
go test -cover ./...
```

Expected: overall coverage stays at or above 70%.

- [ ] **Step 4: Run vet**

Run:

```bash
go vet ./...
```

Expected: PASS.

- [ ] **Step 5: Manual smoke check**

Run:

```bash
go build -o bin/gistclaw ./cmd/gistclaw
./bin/gistclaw doctor
```

Expected:

- research check is skipped unless configured
- configured MCP servers appear as distinct doctor lines
- no bootstrap failure when MCP is absent

- [ ] **Step 6: Commit final polish**

```bash
git add .
git commit -m "feat: add research and minimal mcp tool support"
```

## Deliberately Deferred

- MCP over `sse` or `http`
- write-capable MCP tools
- marketplace, install, update, or plugin UX
- multiple first-party web-search providers
- full “deep research” orchestration state machine
- Codex or Claude Code executors

## Implementation Notes

- The current runtime does not execute `GenerateResult.ToolCalls`; this is the real blocker and must land before research or MCP is useful.
- The current providers advertise tool schemas but do not round-trip prior tool results into subsequent requests; fix that in the same slice.
- Do not bypass `ConversationStore.AppendEvent` for new tool-call state.
- Keep the first MCP transport boring and restart-safe. Startup-loaded stdio is enough to prove the seam.

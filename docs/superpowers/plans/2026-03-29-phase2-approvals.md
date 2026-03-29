# Exec Approvals Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `/approvals` section showing pending exec approval requests (Gateway tab), with node policy and allowlists as placeholders.

**Architecture:** The page loads from `GET /api/recover` which returns `approvals` and `approval_paging`. The Gateway tab shows pending approvals — each row has Approve and Deny actions via `POST /api/recover/approvals/{id}/resolve`. Approve requires a ConfirmModal (dangerous action). Nodes and Allowlists tabs are placeholders — no backend endpoints exist yet. Page data is typed via `+page.ts`.

**Tech Stack:** SvelteKit, Svelte 5 runes, `$lib/recover/load.ts` (existing), `$lib/types/api.ts` (existing), `$lib/http/client.ts` for POSTs, Vitest for tests, Warm Brutalism CSS.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `src/routes/approvals/+page.ts` | Create | Load `RecoverResponse` on page entry |
| `src/routes/approvals/+page.svelte` | Modify | Full Approvals page with tabs |
| `src/lib/components/approvals/ApprovalRow.svelte` | Create | Single approval row with approve/deny |
| `src/lib/components/approvals/ApprovalRow.test.ts` | Create | Render tests for ApprovalRow |
| `src/routes/approvals/page.test.ts` | Create | Render tests for the Approvals page |

---

### Task 1: ApprovalRow component (TDD)

**Files:**
- Create: `src/lib/components/approvals/ApprovalRow.svelte`
- Create: `src/lib/components/approvals/ApprovalRow.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/lib/components/approvals/ApprovalRow.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ApprovalRow from './ApprovalRow.svelte';
import type { RecoverApprovalResponse } from '$lib/types/api';

const pendingApproval: RecoverApprovalResponse = {
  id: 'appr-1',
  run_id: 'run-abc',
  tool_name: 'bash',
  binding_summary: 'rm -rf /tmp/scratch',
  status: 'pending',
  status_label: 'Pending',
  status_class: 'is-active'
};

const resolvedApproval: RecoverApprovalResponse = {
  id: 'appr-2',
  run_id: 'run-def',
  tool_name: 'read_file',
  binding_summary: '/etc/hosts',
  status: 'approved',
  status_label: 'Approved',
  status_class: 'is-success',
  resolved_by: 'admin',
  resolved_at_label: '2 min ago'
};

describe('ApprovalRow', () => {
  it('renders the tool name', () => {
    const { body } = render(ApprovalRow, { props: { approval: pendingApproval } });
    expect(body).toContain('bash');
  });

  it('renders the binding summary', () => {
    const { body } = render(ApprovalRow, { props: { approval: pendingApproval } });
    expect(body).toContain('rm -rf /tmp/scratch');
  });

  it('renders the status label', () => {
    const { body } = render(ApprovalRow, { props: { approval: pendingApproval } });
    expect(body).toContain('Pending');
  });

  it('renders approve and deny buttons for pending approval', () => {
    const { body } = render(ApprovalRow, { props: { approval: pendingApproval } });
    expect(body).toContain('APPROVE');
    expect(body).toContain('DENY');
  });

  it('does not render action buttons for resolved approval', () => {
    const { body } = render(ApprovalRow, { props: { approval: resolvedApproval } });
    expect(body).not.toContain('APPROVE');
    expect(body).not.toContain('DENY');
  });

  it('renders resolved_at_label when present', () => {
    const { body } = render(ApprovalRow, { props: { approval: resolvedApproval } });
    expect(body).toContain('2 min ago');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "ApprovalRow"
```
Expected: `FAIL — cannot find module './ApprovalRow.svelte'`

- [ ] **Step 3: Implement ApprovalRow**

```svelte
<!-- src/lib/components/approvals/ApprovalRow.svelte -->
<script lang="ts">
  import type { RecoverApprovalResponse } from '$lib/types/api';

  let {
    approval,
    onApprove,
    onDeny
  }: {
    approval: RecoverApprovalResponse;
    onApprove?: (id: string) => void;
    onDeny?: (id: string) => void;
  } = $props();

  const isPending = $derived(approval.status === 'pending');

  const statusColor: Record<string, string> = {
    'is-success': 'var(--gc-success)',
    'is-error': 'var(--gc-error)',
    'is-active': 'var(--gc-primary)',
    'is-muted': 'var(--gc-ink-3)',
  };
  const color = $derived(statusColor[approval.status_class] ?? 'var(--gc-ink-3)');
</script>

<div class="flex items-start gap-4 border-b border-b-[1px] border-[var(--gc-border)] px-5 py-4">
  <div class="flex-1 min-w-0">
    <div class="flex items-center gap-3 flex-wrap">
      <span class="gc-stamp text-[var(--gc-ink-2)]">{approval.tool_name}</span>
      <span
        class="gc-badge"
        style="border-color: {color}; color: {color};"
      >{approval.status_label}</span>
    </div>
    <p class="gc-copy mt-1 font-mono text-sm text-[var(--gc-signal)]">{approval.binding_summary}</p>
    <div class="flex items-center gap-3 mt-1">
      <span class="gc-machine text-[var(--gc-ink-4)]">run {approval.run_id.slice(0, 8)}</span>
      {#if approval.resolved_at_label}
        <span class="gc-machine text-[var(--gc-ink-4)]">resolved {approval.resolved_at_label}</span>
      {/if}
      {#if approval.resolved_by}
        <span class="gc-machine text-[var(--gc-ink-4)]">by {approval.resolved_by}</span>
      {/if}
    </div>
  </div>
  {#if isPending}
    <div class="flex shrink-0 gap-2">
      <button
        onclick={() => onDeny?.(approval.id)}
        class="gc-action px-3 py-1 text-[10px] text-[var(--gc-error)]"
      >DENY</button>
      <button
        onclick={() => onApprove?.(approval.id)}
        class="gc-action px-3 py-1 text-[10px] text-[var(--gc-primary)]"
      >APPROVE</button>
    </div>
  {/if}
</div>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep -E "ApprovalRow|Tests"
```
Expected: 6 tests passing.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/components/approvals/
git commit -m "feat: add ApprovalRow component with approve/deny actions"
```

---

### Task 2: Approvals page loader

**Files:**
- Create: `src/routes/approvals/+page.ts`

- [ ] **Step 1: Create the page loader**

```typescript
// src/routes/approvals/+page.ts
import { loadRecover } from '$lib/recover/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
  try {
    const data = await loadRecover(fetch);
    return {
      approvals: {
        items: data.approvals ?? [],
        paging: data.approval_paging,
        openCount: data.summary?.open_approvals ?? 0,
      }
    };
  } catch {
    return {
      approvals: {
        items: [],
        paging: { has_next: false, has_prev: false },
        openCount: 0,
      }
    };
  }
};
```

- [ ] **Step 2: Run type check to verify**

```bash
cd frontend && bun run check 2>&1 | tail -5
```
Expected: `0 ERRORS 0 WARNINGS`

- [ ] **Step 3: Commit**

```bash
git add frontend/src/routes/approvals/+page.ts
git commit -m "feat: add approvals page loader from /api/recover"
```

---

### Task 3: Approvals page (TDD)

**Files:**
- Modify: `src/routes/approvals/+page.svelte`
- Create: `src/routes/approvals/page.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/routes/approvals/page.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ApprovalsPage from './+page.svelte';

const nav = [{ id: 'approvals', label: 'Exec Approvals', href: '/approvals' }];

const baseData = {
  auth: { authenticated: true, password_configured: true, setup_required: false },
  project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
  navigation: nav,
  onboarding: null,
  currentPath: '/approvals',
  currentSearch: '',
  approvals: {
    items: [],
    paging: { has_next: false, has_prev: false },
    openCount: 0
  }
};

describe('Exec Approvals page', () => {
  it('renders the Exec Approvals heading', () => {
    const { body } = render(ApprovalsPage, { props: { data: baseData } });
    expect(body).toContain('Exec Approvals');
  });

  it('renders Gateway, Nodes, Allowlists tabs', () => {
    const { body } = render(ApprovalsPage, { props: { data: baseData } });
    expect(body).toContain('Gateway');
    expect(body).toContain('Nodes');
    expect(body).toContain('Allowlists');
  });

  it('renders empty state when no pending approvals', () => {
    const { body } = render(ApprovalsPage, { props: { data: baseData } });
    expect(body).toContain('No pending approvals');
  });

  it('renders approval row when approvals are provided', () => {
    const data = {
      ...baseData,
      approvals: {
        items: [{
          id: 'appr-1',
          run_id: 'run-abc',
          tool_name: 'bash',
          binding_summary: 'echo hello',
          status: 'pending',
          status_label: 'Pending',
          status_class: 'is-active'
        }],
        paging: { has_next: false, has_prev: false },
        openCount: 1
      }
    };
    const { body } = render(ApprovalsPage, { props: { data } });
    expect(body).toContain('bash');
    expect(body).toContain('echo hello');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Exec Approvals page"
```
Expected: FAIL — page renders placeholder only.

- [ ] **Step 3: Implement the Approvals page**

```svelte
<!-- src/routes/approvals/+page.svelte -->
<script lang="ts">
  import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
  import ApprovalRow from '$lib/components/approvals/ApprovalRow.svelte';
  import { requestJSON } from '$lib/http/client';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const tabs = [
    { id: 'gateway', label: 'Gateway' },
    { id: 'nodes', label: 'Nodes' },
    { id: 'allowlists', label: 'Allowlists' },
  ];
  let activeTab = $state('gateway');

  const items = $derived(data.approvals?.items ?? []);

  let confirmId = $state<string | null>(null);
  let actionError = $state('');
  let actionMessage = $state('');

  function requestApprove(id: string): void {
    confirmId = id;
  }

  function cancelApprove(): void {
    confirmId = null;
  }

  async function confirmApprove(): Promise<void> {
    if (!confirmId) return;
    const id = confirmId;
    confirmId = null;
    actionError = '';
    actionMessage = '';
    try {
      await requestJSON(fetch, `/api/recover/approvals/${id}/resolve`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ action: 'approve' }),
      });
      actionMessage = 'Approval granted. Run will resume.';
    } catch {
      actionError = 'Failed to approve. Please try again.';
    }
  }

  async function handleDeny(id: string): Promise<void> {
    actionError = '';
    actionMessage = '';
    try {
      await requestJSON(fetch, `/api/recover/approvals/${id}/resolve`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ action: 'deny' }),
      });
      actionMessage = 'Request denied. Run will be interrupted.';
    } catch {
      actionError = 'Failed to deny. Please try again.';
    }
  }
</script>

<svelte:head>
  <title>Exec Approvals | GistClaw</title>
</svelte:head>

<!-- Confirm modal -->
{#if confirmId}
  <div
    class="fixed inset-0 z-50 flex items-center justify-center bg-[var(--gc-canvas)]/80"
    role="dialog"
    aria-modal="true"
    aria-label="Confirm approval"
  >
    <div class="gc-panel max-w-sm w-full mx-4 px-6 py-5">
      <p class="gc-stamp text-[var(--gc-warning)]">CONFIRM APPROVE</p>
      <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Allow this execution?</p>
      <p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
        The agent will be permitted to run the requested tool call. This action cannot be undone.
      </p>
      <div class="flex gap-3 mt-5">
        <button
          onclick={cancelApprove}
          class="gc-action flex-1 px-4 py-2 text-[var(--gc-ink-2)]"
        >CANCEL</button>
        <button
          onclick={confirmApprove}
          class="gc-action flex-1 px-4 py-2 text-[var(--gc-primary)]"
        >APPROVE</button>
      </div>
    </div>
  </div>
{/if}

<div class="flex h-full flex-col overflow-hidden">
  <div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]">
    <div class="px-6 pt-4 pb-0">
      <h1 class="gc-panel-title text-[var(--gc-ink)]">Exec Approvals</h1>
    </div>
    <SectionTabs {tabs} bind:activeTab />
  </div>

  <div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
    {#if actionMessage}
      <div class="border-b border-b-[1.5px] border-[var(--gc-success)] bg-[var(--gc-success-dim)] px-5 py-3">
        <p class="gc-copy text-[var(--gc-success)]">{actionMessage}</p>
      </div>
    {/if}
    {#if actionError}
      <div class="border-b border-b-[1.5px] border-[var(--gc-error)] bg-[var(--gc-error-dim)] px-5 py-3">
        <p class="gc-copy text-[var(--gc-error)]">{actionError}</p>
      </div>
    {/if}

    {#if activeTab === 'gateway'}
      {#if items.length === 0}
        <div class="flex flex-1 items-center justify-center p-10">
          <div class="text-center">
            <p class="gc-stamp text-[var(--gc-ink-3)]">GATEWAY</p>
            <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No pending approvals</p>
            <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
              The gateway is clear. Approval requests appear here when agents need exec permission.
            </p>
          </div>
        </div>
      {:else}
        {#each items as approval (approval.id)}
          <ApprovalRow
            {approval}
            onApprove={requestApprove}
            onDeny={handleDeny}
          />
        {/each}
      {/if}
    {:else if activeTab === 'nodes'}
      <div class="flex flex-1 items-center justify-center p-10">
        <div class="text-center">
          <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
          <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Node exec policy</p>
          <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
            Per-node execution policy configuration will be available here.
          </p>
        </div>
      </div>
    {:else}
      <div class="flex flex-1 items-center justify-center p-10">
        <div class="text-center">
          <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
          <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Allowlists</p>
          <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
            Tool and path allowlist management will be available here.
          </p>
        </div>
      </div>
    {/if}
  </div>
</div>
```

- [ ] **Step 4: Run test + check + lint**

```bash
cd frontend && bun run test:unit -- --run 2>&1 | tail -5
bun run check 2>&1 | tail -3
bun run format && bun run lint 2>&1 | tail -3
```
Expected: All tests pass, 0 errors, 0 warnings.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/approvals/ frontend/src/lib/components/approvals/
git commit -m "feat: Exec Approvals section — pending gateway queue with approve/deny, nodes and allowlists placeholders"
```

---

## Self-Review

**Spec coverage:**
- ✓ Tabs: Gateway · Nodes · Allowlists
- ✓ Gateway: pending approval queue with approve/deny
- ✓ Approve requires ConfirmModal (dangerous action per spec)
- ✓ Deny has no confirmation (per spec)
- ✓ Empty state: "No pending approvals. The gateway is clear."
- ✓ Nodes placeholder (no per-node policy API exists yet)
- ✓ Allowlists placeholder (no allowlist API exists yet)
- ✓ ApprovalRow test coverage (6 tests)
- ✓ Page test coverage (4 tests)
- ⚠ Approval badge count in LeftNav nav item: requires LeftNav to accept badge count — left to Phase 2 integration

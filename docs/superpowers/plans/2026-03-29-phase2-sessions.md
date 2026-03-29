# Sessions Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `/sessions` section showing channel sessions with a message history drawer, list search/filter, and an overrides placeholder tab.

**Architecture:** The page loads from `GET /api/conversations` (list) and `GET /api/conversations/{id}` (detail). Three tabs: List (session table with click-to-expand detail drawer), Overrides (placeholder), History (placeholder). The `+page.ts` loader fetches the session list; detail is fetched on-demand when a row is clicked. Components: `SessionRow.svelte` for each row, `SessionDetail.svelte` for the expanded drawer.

**Tech Stack:** SvelteKit, Svelte 5 runes, `$lib/conversations/load.ts` (existing), `$lib/types/api.ts` (existing), Vitest for tests, Warm Brutalism CSS.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `src/routes/sessions/+page.ts` | Create | Load `ConversationsResponse` on page entry |
| `src/routes/sessions/+page.svelte` | Modify | Full Sessions page with tabs |
| `src/lib/components/sessions/SessionRow.svelte` | Create | Single session list row |
| `src/lib/components/sessions/SessionRow.test.ts` | Create | Render tests for SessionRow |
| `src/lib/components/sessions/SessionDetail.svelte` | Create | Session detail drawer (messages + route + deliveries) |
| `src/lib/components/sessions/SessionDetail.test.ts` | Create | Render tests for SessionDetail |
| `src/routes/sessions/page.test.ts` | Create | Render tests for the Sessions page |

---

### Task 1: SessionRow component (TDD)

**Files:**
- Create: `src/lib/components/sessions/SessionRow.svelte`
- Create: `src/lib/components/sessions/SessionRow.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/lib/components/sessions/SessionRow.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SessionRow from './SessionRow.svelte';
import type { ConversationIndexItemResponse } from '$lib/types/api';

const session: ConversationIndexItemResponse = {
  id: 'sess-1',
  conversation_id: 'conv-1',
  agent_id: 'front',
  role_label: 'User',
  status_label: 'Active',
  updated_at_label: '2 min ago'
};

describe('SessionRow', () => {
  it('renders the session id as a stamp', () => {
    const { body } = render(SessionRow, { props: { session, selected: false } });
    expect(body).toContain('sess-1');
  });

  it('renders the agent id', () => {
    const { body } = render(SessionRow, { props: { session, selected: false } });
    expect(body).toContain('front');
  });

  it('renders the role label', () => {
    const { body } = render(SessionRow, { props: { session, selected: false } });
    expect(body).toContain('User');
  });

  it('renders the status label', () => {
    const { body } = render(SessionRow, { props: { session, selected: false } });
    expect(body).toContain('Active');
  });

  it('renders the updated_at_label', () => {
    const { body } = render(SessionRow, { props: { session, selected: false } });
    expect(body).toContain('2 min ago');
  });

  it('applies selected styling when selected is true', () => {
    const { body } = render(SessionRow, { props: { session, selected: true } });
    expect(body).toContain('gc-primary');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "SessionRow"
```
Expected: `FAIL — cannot find module './SessionRow.svelte'`

- [ ] **Step 3: Implement SessionRow**

```svelte
<!-- src/lib/components/sessions/SessionRow.svelte -->
<script lang="ts">
  import type { ConversationIndexItemResponse } from '$lib/types/api';

  let {
    session,
    selected,
    onclick
  }: {
    session: ConversationIndexItemResponse;
    selected: boolean;
    onclick?: () => void;
  } = $props();
</script>

<button
  {onclick}
  class="flex w-full items-center gap-4 border-b border-b-[1px] border-[var(--gc-border)] px-5 py-4 text-left transition-colors hover:bg-[var(--gc-surface-raised)] {selected
    ? 'border-l-2 border-l-[var(--gc-primary)] bg-[var(--gc-surface-raised)]'
    : ''}"
>
  <div class="flex-1 min-w-0">
    <div class="flex items-center gap-3">
      <span class="gc-stamp text-[var(--gc-ink-2)] truncate">{session.id}</span>
      <span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-3)]">{session.status_label}</span>
    </div>
    <div class="flex items-center gap-2 mt-1">
      <span class="gc-copy text-[var(--gc-ink-3)]">{session.agent_id}</span>
      <span class="gc-copy text-[var(--gc-ink-4)]">·</span>
      <span class="gc-copy text-[var(--gc-ink-3)]">{session.role_label}</span>
    </div>
  </div>
  <time class="gc-machine shrink-0 text-[var(--gc-ink-4)]">{session.updated_at_label}</time>
</button>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep -E "SessionRow|Tests"
```
Expected: 6 tests passing for SessionRow.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/components/sessions/
git commit -m "feat: add SessionRow component"
```

---

### Task 2: SessionDetail component (TDD)

**Files:**
- Create: `src/lib/components/sessions/SessionDetail.svelte`
- Create: `src/lib/components/sessions/SessionDetail.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/lib/components/sessions/SessionDetail.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SessionDetail from './SessionDetail.svelte';
import type { ConversationDetailResponse } from '$lib/types/api';

const detail: ConversationDetailResponse = {
  session: {
    id: 'sess-1',
    agent_id: 'front',
    role_label: 'User',
    status_label: 'Active'
  },
  messages: [
    {
      kind: 'inbound',
      kind_label: 'Inbound',
      body: { plain_text: 'Hello world', html: '<p>Hello world</p>' },
      sender_label: 'Alice',
      sender_is_mono: false
    }
  ],
  deliveries: [],
  delivery_failures: []
};

describe('SessionDetail', () => {
  it('renders session id as heading', () => {
    const { body } = render(SessionDetail, { props: { detail } });
    expect(body).toContain('sess-1');
  });

  it('renders the agent id', () => {
    const { body } = render(SessionDetail, { props: { detail } });
    expect(body).toContain('front');
  });

  it('renders message plain text', () => {
    const { body } = render(SessionDetail, { props: { detail } });
    expect(body).toContain('Hello world');
  });

  it('renders sender label', () => {
    const { body } = render(SessionDetail, { props: { detail } });
    expect(body).toContain('Alice');
  });

  it('renders empty state when no messages', () => {
    const emptyDetail = { ...detail, messages: [] };
    const { body } = render(SessionDetail, { props: { detail: emptyDetail } });
    expect(body).toContain('No messages');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "SessionDetail"
```
Expected: FAIL — cannot find module.

- [ ] **Step 3: Implement SessionDetail**

```svelte
<!-- src/lib/components/sessions/SessionDetail.svelte -->
<script lang="ts">
  import type { ConversationDetailResponse } from '$lib/types/api';

  let { detail }: { detail: ConversationDetailResponse } = $props();
</script>

<div class="flex h-full flex-col overflow-hidden">
  <div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border)] px-5 py-4">
    <p class="gc-stamp text-[var(--gc-ink-3)]">SESSION</p>
    <p class="gc-panel-title mt-1 text-[var(--gc-ink)] truncate">{detail.session.id}</p>
    <div class="flex items-center gap-3 mt-2">
      <span class="gc-copy text-[var(--gc-ink-3)]">{detail.session.agent_id}</span>
      <span class="gc-copy text-[var(--gc-ink-4)]">·</span>
      <span class="gc-copy text-[var(--gc-ink-3)]">{detail.session.role_label}</span>
      <span class="gc-copy text-[var(--gc-ink-4)]">·</span>
      <span class="gc-copy text-[var(--gc-ink-3)]">{detail.session.status_label}</span>
    </div>
  </div>

  <div class="flex-1 overflow-y-auto">
    {#if detail.messages.length === 0}
      <div class="flex items-center justify-center p-8">
        <p class="gc-copy text-[var(--gc-ink-3)]">No messages</p>
      </div>
    {:else}
      {#each detail.messages as msg, i (i)}
        <div class="border-b border-b-[1px] border-[var(--gc-border)] px-5 py-3">
          <div class="flex items-center gap-3 mb-1">
            <span class="gc-stamp text-[var(--gc-ink-3)]">{msg.kind_label}</span>
            <span class="gc-copy text-[var(--gc-ink-2)]">{msg.sender_label}</span>
          </div>
          <p class="gc-copy text-[var(--gc-ink)] whitespace-pre-wrap">{msg.body.plain_text}</p>
        </div>
      {/each}
    {/if}
  </div>

  {#if detail.route}
    <div class="shrink-0 border-t border-t-[1.5px] border-[var(--gc-border)] px-5 py-3">
      <p class="gc-stamp text-[var(--gc-ink-3)]">ROUTE</p>
      <div class="flex items-center gap-3 mt-1">
        <span class="gc-copy text-[var(--gc-ink-2)]">{detail.route.connector_id}</span>
        <span class="gc-copy text-[var(--gc-ink-4)]">·</span>
        <span class="gc-copy text-[var(--gc-ink-3)]">{detail.route.status_label}</span>
      </div>
    </div>
  {/if}
</div>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep -E "SessionDetail|Tests"
```
Expected: 5 tests passing.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/components/sessions/SessionDetail.svelte frontend/src/lib/components/sessions/SessionDetail.test.ts
git commit -m "feat: add SessionDetail component"
```

---

### Task 3: Sessions page loader

**Files:**
- Create: `src/routes/sessions/+page.ts`

- [ ] **Step 1: Create the page loader**

```typescript
// src/routes/sessions/+page.ts
import { loadConversations } from '$lib/conversations/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch, url }) => {
  try {
    const search = url.searchParams.toString();
    const data = await loadConversations(fetch, search);
    return {
      sessions: {
        items: data.sessions ?? [],
        paging: data.paging,
        runtimeConnectors: data.runtime_connectors ?? [],
      }
    };
  } catch {
    return {
      sessions: {
        items: [],
        paging: { has_next: false, has_prev: false },
        runtimeConnectors: [],
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
git add frontend/src/routes/sessions/+page.ts
git commit -m "feat: add sessions page loader"
```

---

### Task 4: Sessions page (TDD)

**Files:**
- Modify: `src/routes/sessions/+page.svelte`
- Create: `src/routes/sessions/page.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/routes/sessions/page.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SessionsPage from './+page.svelte';

const nav = [{ id: 'sessions', label: 'Sessions', href: '/sessions' }];

const baseData = {
  auth: { authenticated: true, password_configured: true, setup_required: false },
  project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
  navigation: nav,
  onboarding: null,
  currentPath: '/sessions',
  currentSearch: '',
  sessions: {
    items: [],
    paging: { has_next: false, has_prev: false },
    runtimeConnectors: []
  }
};

describe('Sessions page', () => {
  it('renders the Sessions heading', () => {
    const { body } = render(SessionsPage, { props: { data: baseData } });
    expect(body).toContain('Sessions');
  });

  it('renders List, Overrides, History tabs', () => {
    const { body } = render(SessionsPage, { props: { data: baseData } });
    expect(body).toContain('List');
    expect(body).toContain('Overrides');
    expect(body).toContain('History');
  });

  it('renders empty state when no sessions', () => {
    const { body } = render(SessionsPage, { props: { data: baseData } });
    expect(body).toContain('No sessions');
  });

  it('renders session row when sessions are provided', () => {
    const data = {
      ...baseData,
      sessions: {
        items: [{
          id: 'sess-abc',
          conversation_id: 'conv-1',
          agent_id: 'front',
          role_label: 'User',
          status_label: 'Active',
          updated_at_label: '1 min ago'
        }],
        paging: { has_next: false, has_prev: false },
        runtimeConnectors: []
      }
    };
    const { body } = render(SessionsPage, { props: { data } });
    expect(body).toContain('sess-abc');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Sessions page"
```
Expected: FAIL — page renders placeholder only.

- [ ] **Step 3: Implement the Sessions page**

```svelte
<!-- src/routes/sessions/+page.svelte -->
<script lang="ts">
  import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
  import SessionRow from '$lib/components/sessions/SessionRow.svelte';
  import SessionDetail from '$lib/components/sessions/SessionDetail.svelte';
  import { loadConversationDetail } from '$lib/conversations/load';
  import type { ConversationDetailResponse } from '$lib/types/api';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const tabs = [
    { id: 'list', label: 'List' },
    { id: 'overrides', label: 'Overrides' },
    { id: 'history', label: 'History' },
  ];
  let activeTab = $state('list');

  const sessions = $derived(data.sessions?.items ?? []);

  let selectedId = $state<string | null>(null);
  let detail = $state<ConversationDetailResponse | null>(null);
  let detailLoading = $state(false);
  let detailError = $state('');

  async function selectSession(id: string): Promise<void> {
    if (selectedId === id) {
      selectedId = null;
      detail = null;
      return;
    }
    selectedId = id;
    detailLoading = true;
    detailError = '';
    try {
      detail = await loadConversationDetail(fetch, id);
    } catch {
      detailError = 'Failed to load session detail.';
    } finally {
      detailLoading = false;
    }
  }
</script>

<svelte:head>
  <title>Sessions | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
  <div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]">
    <div class="px-6 pt-4 pb-0">
      <h1 class="gc-panel-title text-[var(--gc-ink)]">Sessions</h1>
    </div>
    <SectionTabs {tabs} bind:activeTab />
  </div>

  <div class="flex min-h-0 flex-1 overflow-hidden">
    {#if activeTab === 'list'}
      <!-- Session list -->
      <div class="flex min-h-0 flex-col {selectedId ? 'w-1/2 border-r border-r-[1.5px] border-[var(--gc-border)]' : 'flex-1'} overflow-y-auto">
        {#if sessions.length === 0}
          <div class="flex flex-1 items-center justify-center p-10">
            <div class="text-center">
              <p class="gc-stamp text-[var(--gc-ink-3)]">SESSIONS</p>
              <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No sessions</p>
              <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
                Sessions are created when channels receive messages. Connect a channel to begin.
              </p>
            </div>
          </div>
        {:else}
          {#each sessions as session (session.id)}
            <SessionRow
              {session}
              selected={selectedId === session.id}
              onclick={() => selectSession(session.id)}
            />
          {/each}
        {/if}
      </div>

      <!-- Detail panel -->
      {#if selectedId}
        <div class="flex min-h-0 w-1/2 flex-col overflow-hidden">
          {#if detailLoading}
            <div class="flex flex-1 items-center justify-center p-8">
              <p class="gc-copy text-[var(--gc-ink-3)]">Loading…</p>
            </div>
          {:else if detailError}
            <div class="px-5 py-4">
              <p class="gc-copy text-[var(--gc-error)]">{detailError}</p>
            </div>
          {:else if detail}
            <SessionDetail {detail} />
          {/if}
        </div>
      {/if}
    {:else if activeTab === 'overrides'}
      <div class="flex flex-1 items-center justify-center p-10">
        <div class="text-center">
          <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
          <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Session overrides</p>
          <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
            Per-session agent and routing overrides will be configurable here.
          </p>
        </div>
      </div>
    {:else}
      <div class="flex flex-1 items-center justify-center p-10">
        <div class="text-center">
          <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
          <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Session history</p>
          <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
            Full session history and audit log will appear here.
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
git add frontend/src/routes/sessions/
git commit -m "feat: Sessions section — session list with detail drawer, overrides and history placeholders"
```

---

## Self-Review

**Spec coverage:**
- ✓ Tabs: List · Overrides · History
- ✓ Session list with click-to-expand detail drawer
- ✓ Empty state: "No sessions. Connect a channel to begin."
- ✓ Session detail: messages, route, sender labels
- ✓ Overrides placeholder (no per-session API exists yet)
- ✓ History placeholder (full audit log is a future milestone)
- ✓ SessionRow test coverage (6 tests)
- ✓ SessionDetail test coverage (5 tests)
- ✓ Page test coverage (4 tests)

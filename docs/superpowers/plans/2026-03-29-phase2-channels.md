# Channels Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `/channels` section showing connector health status (Status tab), connection login guidance (Login tab), and channel settings placeholder (Settings tab).

**Architecture:** The page loads connector health data from `GET /api/conversations` (which returns `runtime_connectors` and `health` arrays). Three tabs are rendered via `SectionTabs`. Status tab shows each connector as a row with state badge. Login tab shows connection instructions. Settings tab is a placeholder. Page data is typed via `+page.ts`.

**Tech Stack:** SvelteKit, Svelte 5 runes, `$lib/conversations/load.ts` (existing), `$lib/types/api.ts` (existing types), Vitest for tests, Warm Brutalism CSS classes from `layout.css`.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `src/routes/channels/+page.ts` | Create | Load `ConversationsResponse` on page entry |
| `src/routes/channels/+page.svelte` | Modify | Full Channels page with tabs |
| `src/lib/components/channels/ConnectorRow.svelte` | Create | Single connector status row |
| `src/lib/components/channels/ConnectorRow.test.ts` | Create | Render tests for ConnectorRow |
| `src/routes/channels/page.test.ts` | Create | Render tests for the page |

---

### Task 1: ConnectorRow component (TDD)

**Files:**
- Create: `src/lib/components/channels/ConnectorRow.svelte`
- Create: `src/lib/components/channels/ConnectorRow.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/lib/components/channels/ConnectorRow.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ConnectorRow from './ConnectorRow.svelte';
import type { RecoverRuntimeHealthResponse } from '$lib/types/api';

const activeConnector: RecoverRuntimeHealthResponse = {
  connector_id: 'telegram',
  state: 'active',
  state_label: 'Active',
  state_class: 'is-success',
  summary: 'Connected',
  checked_at_label: '1 min ago',
  restart_suggested: false
};

const errorConnector: RecoverRuntimeHealthResponse = {
  connector_id: 'whatsapp',
  state: 'error',
  state_label: 'Error',
  state_class: 'is-error',
  summary: 'Connection lost',
  checked_at_label: '5 min ago',
  restart_suggested: true
};

describe('ConnectorRow', () => {
  it('renders the connector id as a stamp label', () => {
    const { body } = render(ConnectorRow, { props: { connector: activeConnector } });
    expect(body).toContain('telegram');
  });

  it('renders the state label', () => {
    const { body } = render(ConnectorRow, { props: { connector: activeConnector } });
    expect(body).toContain('Active');
  });

  it('renders the summary text', () => {
    const { body } = render(ConnectorRow, { props: { connector: activeConnector } });
    expect(body).toContain('Connected');
  });

  it('renders restart suggestion badge when restart_suggested is true', () => {
    const { body } = render(ConnectorRow, { props: { connector: errorConnector } });
    expect(body).toContain('RESTART');
  });

  it('does not render restart badge when restart_suggested is false', () => {
    const { body } = render(ConnectorRow, { props: { connector: activeConnector } });
    expect(body).not.toContain('RESTART');
  });

  it('renders the checked_at_label', () => {
    const { body } = render(ConnectorRow, { props: { connector: activeConnector } });
    expect(body).toContain('1 min ago');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "ConnectorRow"
```
Expected: `FAIL — cannot find module './ConnectorRow.svelte'`

- [ ] **Step 3: Implement ConnectorRow**

```svelte
<!-- src/lib/components/channels/ConnectorRow.svelte -->
<script lang="ts">
  import type { RecoverRuntimeHealthResponse } from '$lib/types/api';

  let { connector }: { connector: RecoverRuntimeHealthResponse } = $props();

  const stateColor: Record<string, string> = {
    'is-success': 'var(--gc-success)',
    'is-error': 'var(--gc-error)',
    'is-active': 'var(--gc-primary)',
    'is-muted': 'var(--gc-ink-3)',
  };
  const color = $derived(stateColor[connector.state_class] ?? 'var(--gc-ink-3)');
</script>

<div class="flex items-center gap-4 border-b border-b-[1px] border-[var(--gc-border)] px-5 py-4">
  <div class="flex-1 min-w-0">
    <div class="flex items-center gap-3">
      <span class="gc-stamp text-[var(--gc-ink-2)]">{connector.connector_id}</span>
      <span
        class="gc-badge"
        style="border-color: {color}; color: {color};"
      >{connector.state_label}</span>
      {#if connector.restart_suggested}
        <span class="gc-badge border-[var(--gc-warning)] text-[var(--gc-warning)]">RESTART</span>
      {/if}
    </div>
    <p class="gc-copy mt-1 text-[var(--gc-ink-2)]">{connector.summary}</p>
  </div>
  <time class="gc-machine shrink-0 text-[var(--gc-ink-4)]">{connector.checked_at_label ?? ''}</time>
</div>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep -E "ConnectorRow|Tests"
```
Expected: 6 tests passing for ConnectorRow.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/components/channels/
git commit -m "feat: add ConnectorRow component with state badge and restart hint"
```

---

### Task 2: Channels page loader

**Files:**
- Create: `src/routes/channels/+page.ts`

- [ ] **Step 1: Create the page loader**

```typescript
// src/routes/channels/+page.ts
import { loadConversations } from '$lib/conversations/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
  try {
    const data = await loadConversations(fetch);
    return {
      channels: {
        connectors: data.runtime_connectors ?? [],
        deliveryHealth: data.health ?? [],
      }
    };
  } catch {
    return {
      channels: {
        connectors: [],
        deliveryHealth: [],
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
git add frontend/src/routes/channels/+page.ts
git commit -m "feat: add channels page loader from /api/conversations"
```

---

### Task 3: Channels page (TDD)

**Files:**
- Modify: `src/routes/channels/+page.svelte`
- Create: `src/routes/channels/page.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/routes/channels/page.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ChannelsPage from './+page.svelte';

const nav = [{ id: 'channels', label: 'Channels', href: '/channels' }];

const baseData = {
  auth: { authenticated: true, password_configured: true, setup_required: false },
  project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
  navigation: nav,
  onboarding: null,
  currentPath: '/channels',
  currentSearch: '',
  channels: { connectors: [], deliveryHealth: [] }
};

describe('Channels page', () => {
  it('renders the Channels heading', () => {
    const { body } = render(ChannelsPage, { props: { data: baseData } });
    expect(body).toContain('Channels');
  });

  it('renders Status, Login, Settings tabs', () => {
    const { body } = render(ChannelsPage, { props: { data: baseData } });
    expect(body).toContain('Status');
    expect(body).toContain('Login');
    expect(body).toContain('Settings');
  });

  it('renders empty state when no connectors', () => {
    const { body } = render(ChannelsPage, { props: { data: baseData } });
    expect(body).toContain('No channels connected');
  });

  it('renders connector row when connectors are provided', () => {
    const data = {
      ...baseData,
      channels: {
        connectors: [{
          connector_id: 'telegram',
          state: 'active',
          state_label: 'Active',
          state_class: 'is-success',
          summary: 'Bot is connected',
          checked_at_label: '2 min ago',
          restart_suggested: false
        }],
        deliveryHealth: []
      }
    };
    const { body } = render(ChannelsPage, { props: { data } });
    expect(body).toContain('telegram');
    expect(body).toContain('Bot is connected');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Channels page"
```
Expected: FAIL — page renders placeholder only (missing tabs, etc.)

- [ ] **Step 3: Implement the Channels page**

```svelte
<!-- src/routes/channels/+page.svelte -->
<script lang="ts">
  import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
  import ConnectorRow from '$lib/components/channels/ConnectorRow.svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const tabs = [
    { id: 'status', label: 'Status' },
    { id: 'login', label: 'Login' },
    { id: 'settings', label: 'Settings' },
  ];
  let activeTab = $state('status');

  const connectors = $derived(data.channels?.connectors ?? []);
</script>

<svelte:head>
  <title>Channels | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
  <div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]">
    <div class="px-6 pt-4 pb-0">
      <h1 class="gc-panel-title text-[var(--gc-ink)]">Channels</h1>
    </div>
    <SectionTabs {tabs} bind:activeTab />
  </div>

  <div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
    {#if activeTab === 'status'}
      {#if connectors.length === 0}
        <div class="flex flex-1 items-center justify-center p-10">
          <div class="text-center">
            <p class="gc-stamp text-[var(--gc-ink-3)]">CHANNELS</p>
            <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No channels connected</p>
            <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
              Add a channel to receive messages. Configure Telegram or WhatsApp tokens in Config.
            </p>
          </div>
        </div>
      {:else}
        <div>
          {#each connectors as connector (connector.connector_id)}
            <ConnectorRow {connector} />
          {/each}
        </div>
      {/if}
    {:else if activeTab === 'login'}
      <div class="px-6 py-6">
        <p class="gc-stamp text-[var(--gc-ink-3)]">LOGIN</p>
        <h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Connect a channel</h2>
        <p class="gc-copy mt-3 max-w-lg text-[var(--gc-ink-2)]">
          Channel authentication is managed locally. Set your Telegram bot token or WhatsApp
          credentials via the Config section, then restart the runtime to connect.
        </p>
        <div class="mt-6 gc-panel-soft max-w-lg px-5 py-4">
          <p class="gc-stamp text-[var(--gc-ink-3)]">TELEGRAM</p>
          <p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
            Set <code class="gc-code text-[var(--gc-signal)]">telegram_token</code> in Config → General,
            then restart GistClaw.
          </p>
        </div>
        <div class="mt-4 gc-panel-soft max-w-lg px-5 py-4">
          <p class="gc-stamp text-[var(--gc-ink-3)]">WHATSAPP</p>
          <p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
            QR login is initiated from the local GistClaw CLI. Run
            <code class="gc-code text-[var(--gc-signal)]">gistclaw connect whatsapp</code> to start.
          </p>
        </div>
      </div>
    {:else}
      <div class="flex flex-1 items-center justify-center p-10">
        <div class="text-center">
          <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
          <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Channel settings</p>
          <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
            Per-channel configuration will be available here.
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
git add frontend/src/routes/channels/
git commit -m "feat: Channels section — connector status, login guide, settings placeholder"
```

---

## Self-Review

**Spec coverage:**
- ✓ Tabs: Status · Login · Settings
- ✓ Channel list with connection state badge
- ✓ Empty state: "No channels connected. Add a channel to receive messages."
- ✓ Login guidance for Telegram + WhatsApp
- ⚠ QR/login panel: Backend has no live QR endpoint; Login tab shows CLI instructions (acceptable for Phase 2)
- ⚠ Settings drawer: Settings tab is placeholder (no per-channel API exists yet)
- ✓ ConnectorRow test coverage
- ✓ Page test coverage

**All steps have actual code. No TBDs.**

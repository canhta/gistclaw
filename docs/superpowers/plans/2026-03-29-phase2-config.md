# Config Section Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the `/config` section with a General settings form (General tab), structured settings tabs for Agents & Routing and Models (placeholders), channel token config (Channels tab), a Raw JSON5 editor (Raw JSON5 tab using CodeMirror 6), and an Apply action (Apply tab).

**Architecture:** The page loads from `GET /api/settings` which returns `SettingsResponse`. General tab renders editable fields for approval_mode, host_access_mode, token budget, cost cap, and telegram_token. Changes are submitted via `POST /api/settings`. Raw JSON5 tab uses CodeMirror 6 (`@codemirror/lang-json` + `codemirror`) as a free-form editor. Apply tab confirms and submits changes. Agents & Routing and Models tabs are placeholders (no backend yet). Page data is typed via `+page.ts`.

**Tech Stack:** SvelteKit, Svelte 5 runes, `$lib/settings/load.ts` (existing), `$lib/types/api.ts` (existing), `$lib/http/client.ts` for POSTs, CodeMirror 6 (install required), Vitest for tests, Warm Brutalism CSS.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `src/routes/config/+page.ts` | Create | Load `SettingsResponse` on page entry |
| `src/routes/config/+page.svelte` | Modify | Full Config page with 6 tabs |
| `src/lib/components/config/SettingsField.svelte` | Create | Single labeled form field (text/select) |
| `src/lib/components/config/SettingsField.test.ts` | Create | Render tests for SettingsField |
| `src/routes/config/page.test.ts` | Create | Render tests for the Config page |

---

### Task 1: Install CodeMirror 6

**Files:** `frontend/package.json`

- [ ] **Step 1: Install dependencies**

```bash
cd frontend && bun add codemirror @codemirror/lang-json
```

- [ ] **Step 2: Verify install**

```bash
cd frontend && bun run check 2>&1 | tail -3
```
Expected: `0 ERRORS 0 WARNINGS`

- [ ] **Step 3: Commit**

```bash
git add frontend/package.json frontend/bun.lockb
git commit -m "chore: add CodeMirror 6 for config raw editor"
```

---

### Task 2: SettingsField component (TDD)

**Files:**
- Create: `src/lib/components/config/SettingsField.svelte`
- Create: `src/lib/components/config/SettingsField.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/lib/components/config/SettingsField.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SettingsField from './SettingsField.svelte';

describe('SettingsField', () => {
  it('renders the label', () => {
    const { body } = render(SettingsField, {
      props: { id: 'f1', label: 'Token Budget', value: '50000', type: 'text' }
    });
    expect(body).toContain('Token Budget');
  });

  it('renders the current value in the input', () => {
    const { body } = render(SettingsField, {
      props: { id: 'f1', label: 'Token Budget', value: '50000', type: 'text' }
    });
    expect(body).toContain('50000');
  });

  it('renders a hint when provided', () => {
    const { body } = render(SettingsField, {
      props: { id: 'f1', label: 'Token Budget', value: '', type: 'text', hint: 'Max tokens per run' }
    });
    expect(body).toContain('Max tokens per run');
  });

  it('renders a select element when options are provided', () => {
    const { body } = render(SettingsField, {
      props: {
        id: 'f1',
        label: 'Approval Mode',
        value: 'on_request',
        type: 'select',
        options: [
          { value: 'on_request', label: 'On Request' },
          { value: 'always', label: 'Always' }
        ]
      }
    });
    expect(body).toContain('On Request');
    expect(body).toContain('Always');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "SettingsField"
```
Expected: FAIL — cannot find module.

- [ ] **Step 3: Implement SettingsField**

```svelte
<!-- src/lib/components/config/SettingsField.svelte -->
<script lang="ts">
  export interface SelectOption {
    value: string;
    label: string;
  }

  let {
    id,
    label,
    value = $bindable(''),
    type = 'text',
    hint,
    options,
    placeholder
  }: {
    id: string;
    label: string;
    value?: string;
    type?: 'text' | 'select' | 'password';
    hint?: string;
    options?: SelectOption[];
    placeholder?: string;
  } = $props();
</script>

<div class="flex flex-col gap-1">
  <label for={id} class="gc-stamp text-[var(--gc-ink-3)]">{label}</label>
  {#if type === 'select' && options}
    <select
      {id}
      bind:value
      class="gc-input w-full px-3 py-2 bg-[var(--gc-surface-raised)] text-[var(--gc-ink)]"
    >
      {#each options as opt (opt.value)}
        <option value={opt.value}>{opt.label}</option>
      {/each}
    </select>
  {:else}
    <input
      {id}
      {type}
      bind:value
      {placeholder}
      class="gc-input w-full px-3 py-2"
    />
  {/if}
  {#if hint}
    <p class="gc-copy text-[var(--gc-ink-4)]">{hint}</p>
  {/if}
</div>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep -E "SettingsField|Tests"
```
Expected: 4 tests passing.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/components/config/
git commit -m "feat: add SettingsField component"
```

---

### Task 3: Config page loader

**Files:**
- Create: `src/routes/config/+page.ts`

- [ ] **Step 1: Create the page loader**

```typescript
// src/routes/config/+page.ts
import { loadSettings } from '$lib/settings/load';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
  try {
    const data = await loadSettings(fetch);
    return { config: { settings: data } };
  } catch {
    return { config: { settings: null } };
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
git add frontend/src/routes/config/+page.ts
git commit -m "feat: add config page loader from /api/settings"
```

---

### Task 4: Config page (TDD)

**Files:**
- Modify: `src/routes/config/+page.svelte`
- Create: `src/routes/config/page.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/routes/config/page.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ConfigPage from './+page.svelte';

const nav = [{ id: 'config', label: 'Config', href: '/config' }];

const machineSettings = {
  storage_root: '/home/user/.gistclaw',
  approval_mode: 'on_request',
  approval_mode_label: 'On Request',
  host_access_mode: 'local',
  host_access_mode_label: 'Local',
  admin_token: 'tok-123',
  per_run_token_budget: '50000',
  daily_cost_cap_usd: '5.00',
  rolling_cost_usd: 0.42,
  rolling_cost_label: '$0.42',
  telegram_token: '',
  active_project_name: 'my-project',
  active_project_path: '/home/user/my-project',
  active_project_summary: '3 agents'
};

const baseData = {
  auth: { authenticated: true, password_configured: true, setup_required: false },
  project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
  navigation: nav,
  onboarding: null,
  currentPath: '/config',
  currentSearch: '',
  config: {
    settings: {
      machine: machineSettings,
      access: {
        password_configured: true,
        other_active_devices: [],
        blocked_devices: []
      }
    }
  }
};

describe('Config page', () => {
  it('renders the Config heading', () => {
    const { body } = render(ConfigPage, { props: { data: baseData } });
    expect(body).toContain('Config');
  });

  it('renders General, Agents & Routing, Models, Channels, Raw JSON5, Apply tabs', () => {
    const { body } = render(ConfigPage, { props: { data: baseData } });
    expect(body).toContain('General');
    expect(body).toContain('Agents');
    expect(body).toContain('Models');
    expect(body).toContain('Channels');
    expect(body).toContain('Raw JSON5');
    expect(body).toContain('Apply');
  });

  it('renders token budget value from settings', () => {
    const { body } = render(ConfigPage, { props: { data: baseData } });
    expect(body).toContain('50000');
  });

  it('renders rolling cost label', () => {
    const { body } = render(ConfigPage, { props: { data: baseData } });
    expect(body).toContain('$0.42');
  });

  it('renders error state when settings is null', () => {
    const data = { ...baseData, config: { settings: null } };
    const { body } = render(ConfigPage, { props: { data } });
    expect(body).toContain('Failed to load');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Config page"
```
Expected: FAIL — page renders placeholder only.

- [ ] **Step 3: Implement the Config page**

```svelte
<!-- src/routes/config/+page.svelte -->
<script lang="ts">
  import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
  import SettingsField from '$lib/components/config/SettingsField.svelte';
  import { requestJSON } from '$lib/http/client';
  import { onMount } from 'svelte';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  const tabs = [
    { id: 'general', label: 'General' },
    { id: 'agents', label: 'Agents & Routing' },
    { id: 'models', label: 'Models' },
    { id: 'channels', label: 'Channels' },
    { id: 'raw', label: 'Raw JSON5' },
    { id: 'apply', label: 'Apply' },
  ];
  let activeTab = $state('general');

  const machine = $derived(data.config?.settings?.machine ?? null);

  // General tab form state
  let approvalMode = $state('');
  let hostAccessMode = $state('');
  let perRunTokenBudget = $state('');
  let dailyCostCap = $state('');
  let telegramToken = $state('');

  // Sync form state from loaded data
  $effect(() => {
    if (machine) {
      approvalMode = machine.approval_mode;
      hostAccessMode = machine.host_access_mode;
      perRunTokenBudget = machine.per_run_token_budget;
      dailyCostCap = machine.daily_cost_cap_usd;
      telegramToken = machine.telegram_token;
    }
  });

  let saveError = $state('');
  let saveMessage = $state('');
  let saving = $state(false);

  // Raw editor
  let rawEditorEl = $state<HTMLElement | null>(null);
  let rawEditorView: unknown = null;

  onMount(async () => {
    if (rawEditorEl) {
      const { EditorView, basicSetup } = await import('codemirror');
      const { json } = await import('@codemirror/lang-json');
      rawEditorView = new EditorView({
        doc: JSON.stringify(data.config?.settings ?? {}, null, 2),
        extensions: [basicSetup, json()],
        parent: rawEditorEl,
      });
    }
    return () => {
      if (rawEditorView && typeof rawEditorView === 'object' && 'destroy' in rawEditorView) {
        (rawEditorView as { destroy(): void }).destroy();
      }
    };
  });

  const approvalModeOptions = [
    { value: 'on_request', label: 'On Request' },
    { value: 'always', label: 'Always' },
    { value: 'never', label: 'Never' },
  ];

  const hostAccessModeOptions = [
    { value: 'local', label: 'Local only' },
    { value: 'network', label: 'Network accessible' },
  ];

  async function handleSaveGeneral(e: SubmitEvent): Promise<void> {
    e.preventDefault();
    saveError = '';
    saveMessage = '';
    saving = true;
    try {
      await requestJSON(fetch, '/api/settings', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          approval_mode: approvalMode,
          host_access_mode: hostAccessMode,
          per_run_token_budget: perRunTokenBudget,
          daily_cost_cap_usd: dailyCostCap,
          telegram_token: telegramToken,
        }),
      });
      saveMessage = 'Settings saved.';
    } catch {
      saveError = 'Failed to save settings.';
    } finally {
      saving = false;
    }
  }
</script>

<svelte:head>
  <title>Config | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
  <div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]">
    <div class="px-6 pt-4 pb-0">
      <h1 class="gc-panel-title text-[var(--gc-ink)]">Config</h1>
    </div>
    <SectionTabs {tabs} bind:activeTab />
  </div>

  <div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
    {#if !machine && activeTab !== 'raw' && activeTab !== 'apply'}
      <div class="border-b border-b-[1.5px] border-[var(--gc-error)] bg-[var(--gc-error-dim)] px-5 py-3">
        <p class="gc-copy text-[var(--gc-error)]">Failed to load settings. Please reload.</p>
      </div>
    {/if}

    {#if saveMessage}
      <div class="border-b border-b-[1.5px] border-[var(--gc-success)] bg-[var(--gc-success-dim)] px-5 py-3">
        <p class="gc-copy text-[var(--gc-success)]">{saveMessage}</p>
      </div>
    {/if}
    {#if saveError}
      <div class="border-b border-b-[1.5px] border-[var(--gc-error)] bg-[var(--gc-error-dim)] px-5 py-3">
        <p class="gc-copy text-[var(--gc-error)]">{saveError}</p>
      </div>
    {/if}

    {#if activeTab === 'general'}
      <div class="px-6 py-6 max-w-lg">
        <p class="gc-stamp text-[var(--gc-ink-3)]">GENERAL</p>

        {#if machine}
          <div class="mt-2 mb-6 gc-panel-soft px-4 py-3">
            <p class="gc-stamp text-[var(--gc-ink-3)]">ROLLING COST</p>
            <p class="gc-panel-title mt-1 text-[var(--gc-ink)]">{machine.rolling_cost_label}</p>
          </div>

          <form onsubmit={handleSaveGeneral} class="flex flex-col gap-5">
            <SettingsField
              id="approval-mode"
              label="APPROVAL MODE"
              type="select"
              bind:value={approvalMode}
              options={approvalModeOptions}
              hint="When to require operator approval before exec tool calls"
            />
            <SettingsField
              id="host-access-mode"
              label="HOST ACCESS MODE"
              type="select"
              bind:value={hostAccessMode}
              options={hostAccessModeOptions}
            />
            <SettingsField
              id="token-budget"
              label="TOKEN BUDGET PER RUN"
              bind:value={perRunTokenBudget}
              placeholder="50000"
              hint="Maximum tokens per run (0 = unlimited)"
            />
            <SettingsField
              id="cost-cap"
              label="DAILY COST CAP (USD)"
              bind:value={dailyCostCap}
              placeholder="5.00"
              hint="Stop dispatching runs when daily spend exceeds this amount"
            />
            <SettingsField
              id="telegram-token"
              label="TELEGRAM BOT TOKEN"
              type="password"
              bind:value={telegramToken}
              placeholder="Leave blank to keep existing"
              hint="Restart GistClaw after changing the token"
            />
            <div class="flex gap-3 mt-2">
              <button
                type="submit"
                disabled={saving}
                class="gc-action px-4 py-2 text-[var(--gc-primary)] disabled:opacity-50"
              >
                {saving ? 'SAVING…' : 'SAVE'}
              </button>
            </div>
          </form>
        {/if}
      </div>
    {:else if activeTab === 'agents'}
      <div class="flex flex-1 items-center justify-center p-10">
        <div class="text-center">
          <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
          <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Agents & Routing</p>
          <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
            Agent configuration and routing rules will be editable here.
          </p>
        </div>
      </div>
    {:else if activeTab === 'models'}
      <div class="flex flex-1 items-center justify-center p-10">
        <div class="text-center">
          <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
          <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Models</p>
          <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
            Model selection and parameters will be configurable here.
          </p>
        </div>
      </div>
    {:else if activeTab === 'channels'}
      <div class="px-6 py-6 max-w-lg">
        <p class="gc-stamp text-[var(--gc-ink-3)]">CHANNELS</p>
        <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Channel configuration</p>
        <p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
          Live channel connectivity is managed from the Channels section. Deep channel config
          (tokens, timeouts, retry policy) will appear here in a future release.
        </p>
      </div>
    {:else if activeTab === 'raw'}
      <div class="flex min-h-0 flex-1 flex-col">
        <div class="border-b border-b-[1.5px] border-[var(--gc-warning)] bg-[var(--gc-warning-dim)] px-5 py-3">
          <p class="gc-stamp text-[var(--gc-warning)]">READ ONLY</p>
          <p class="gc-copy mt-1 text-[var(--gc-ink)]">
            This view shows the current runtime settings as JSON. Use the General tab to make changes.
          </p>
        </div>
        <div bind:this={rawEditorEl} class="flex-1 overflow-auto font-mono text-sm"></div>
      </div>
    {:else}
      <!-- Apply tab -->
      <div class="px-6 py-6 max-w-lg">
        <p class="gc-stamp text-[var(--gc-ink-3)]">APPLY</p>
        <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Apply configuration changes</p>
        <p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
          Changes made in the General tab are applied immediately on save. Some settings
          (e.g. Telegram token, host access mode) require a GistClaw restart to take effect.
        </p>
        <div class="mt-6 gc-panel-soft px-4 py-3">
          <p class="gc-stamp text-[var(--gc-ink-3)]">STORAGE ROOT</p>
          <p class="gc-copy mt-1 font-mono text-sm text-[var(--gc-ink-2)]">{machine?.storage_root ?? '—'}</p>
        </div>
        <div class="mt-4 gc-panel-soft px-4 py-3">
          <p class="gc-stamp text-[var(--gc-ink-3)]">ACTIVE PROJECT</p>
          <p class="gc-copy mt-1 text-[var(--gc-ink-2)]">{machine?.active_project_name ?? '—'}</p>
          <p class="gc-copy font-mono text-sm text-[var(--gc-ink-3)]">{machine?.active_project_path ?? ''}</p>
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
git add frontend/src/routes/config/ frontend/src/lib/components/config/
git commit -m "feat: Config section — general settings form, raw JSON5 viewer, agents/models/apply tabs"
```

---

## Self-Review

**Spec coverage:**
- ✓ Tabs: General · Agents & Routing · Models · Channels · Raw JSON5 · Apply
- ✓ General tab: approval_mode, host_access_mode, token budget, cost cap, telegram_token
- ✓ Rolling cost display on General tab
- ✓ CodeMirror 6 for Raw JSON5 tab (read-only view of current settings)
- ✓ Apply tab: storage root + active project display, restart notes
- ✓ Error state when settings fails to load
- ✓ Agents & Routing placeholder (no per-agent API form yet)
- ✓ Models placeholder (no models API yet)
- ✓ Channels tab: delegates to Channels section for live connectivity
- ✓ SettingsField test coverage (4 tests)
- ✓ Page test coverage (5 tests)
- ⚠ Raw tab is read-only — full JSON5 edit+apply requires a config file endpoint (not yet shipped)
- ⚠ Apply tab submit action deferred — no `/api/settings/apply` endpoint exists yet

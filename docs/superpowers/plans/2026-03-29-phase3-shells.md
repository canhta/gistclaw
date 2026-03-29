# Phase 3 Placeholder Shells Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement proper placeholder shells for all 6 Phase 3 sections: Instances, Skills, Nodes, Debug, Logs, Update. Each gets a page title, correct tabs matching the spec, and a "COMING SOON" state per tab.

**Architecture:** Each page follows the same shell pattern: `SectionTabs` + `<svelte:head>` + tab-switched placeholder panels. No loaders needed (no backend endpoints). Tests verify heading and tab labels render. All 6 sections are in this single plan since the implementation is uniform.

**Tech Stack:** SvelteKit, Svelte 5 runes, `SectionTabs.svelte` (existing), Vitest for tests, Warm Brutalism CSS.

---

## File Structure

| File | Action | Responsibility |
|---|---|---|
| `src/routes/instances/+page.svelte` | Modify | Instances shell — Presence · Details tabs |
| `src/routes/skills/+page.svelte` | Modify | Skills shell — Installed · Available · Credentials tabs |
| `src/routes/nodes/+page.svelte` | Modify | Nodes shell — List · Capabilities tabs |
| `src/routes/debug/+page.svelte` | Modify | Debug shell — Status · Health · Models · Events · RPC tabs |
| `src/routes/logs/+page.svelte` | Modify | Logs shell — Live Tail · Filters · Export tabs |
| `src/routes/update/+page.svelte` | Modify | Update shell — Run Update · Restart Report tabs |
| `src/routes/instances/page.test.ts` | Create | Tests for Instances page |
| `src/routes/skills/page.test.ts` | Create | Tests for Skills page |
| `src/routes/nodes/page.test.ts` | Create | Tests for Nodes page |
| `src/routes/debug/page.test.ts` | Create | Tests for Debug page |
| `src/routes/logs/page.test.ts` | Create | Tests for Logs page |
| `src/routes/update/page.test.ts` | Create | Tests for Update page |

---

### Task 1: Instances page

**Files:**
- Modify: `src/routes/instances/+page.svelte`
- Create: `src/routes/instances/page.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/routes/instances/page.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import InstancesPage from './+page.svelte';

const baseData = {
  auth: { authenticated: true, password_configured: true, setup_required: false },
  project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
  navigation: [{ id: 'instances', label: 'Instances', href: '/instances' }],
  onboarding: null,
  currentPath: '/instances',
  currentSearch: '',
};

describe('Instances page', () => {
  it('renders the Instances heading', () => {
    const { body } = render(InstancesPage, { props: { data: baseData } });
    expect(body).toContain('Instances');
  });

  it('renders Presence and Details tabs', () => {
    const { body } = render(InstancesPage, { props: { data: baseData } });
    expect(body).toContain('Presence');
    expect(body).toContain('Details');
  });

  it('renders COMING SOON state', () => {
    const { body } = render(InstancesPage, { props: { data: baseData } });
    expect(body).toContain('COMING SOON');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Instances page"
```
Expected: FAIL — page is a placeholder without tabs.

- [ ] **Step 3: Implement Instances page**

```svelte
<!-- src/routes/instances/+page.svelte -->
<script lang="ts">
  import SectionTabs from '$lib/components/shell/SectionTabs.svelte';

  const tabs = [
    { id: 'presence', label: 'Presence' },
    { id: 'details', label: 'Details' },
  ];
  let activeTab = $state('presence');
</script>

<svelte:head>
  <title>Instances | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
  <div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]">
    <div class="px-6 pt-4 pb-0">
      <h1 class="gc-panel-title text-[var(--gc-ink)]">Instances</h1>
    </div>
    <SectionTabs {tabs} bind:activeTab />
  </div>

  <div class="flex min-h-0 flex-1 items-center justify-center p-10">
    <div class="text-center">
      <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
      <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Instance presence</p>
      <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
        Instance inventory and presence tracking are not connected to a backend yet.
      </p>
    </div>
  </div>
</div>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Instances page"
```
Expected: 3 tests passing.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/instances/
git commit -m "feat: Instances section — Presence · Details shell with COMING SOON state"
```

---

### Task 2: Skills page

**Files:**
- Modify: `src/routes/skills/+page.svelte`
- Create: `src/routes/skills/page.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/routes/skills/page.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SkillsPage from './+page.svelte';

const baseData = {
  auth: { authenticated: true, password_configured: true, setup_required: false },
  project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
  navigation: [{ id: 'skills', label: 'Skills', href: '/skills' }],
  onboarding: null,
  currentPath: '/skills',
  currentSearch: '',
};

describe('Skills page', () => {
  it('renders the Skills heading', () => {
    const { body } = render(SkillsPage, { props: { data: baseData } });
    expect(body).toContain('Skills');
  });

  it('renders Installed, Available, Credentials tabs', () => {
    const { body } = render(SkillsPage, { props: { data: baseData } });
    expect(body).toContain('Installed');
    expect(body).toContain('Available');
    expect(body).toContain('Credentials');
  });

  it('renders COMING SOON state', () => {
    const { body } = render(SkillsPage, { props: { data: baseData } });
    expect(body).toContain('COMING SOON');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Skills page"
```
Expected: FAIL — page is a placeholder without tabs.

- [ ] **Step 3: Implement Skills page**

```svelte
<!-- src/routes/skills/+page.svelte -->
<script lang="ts">
  import SectionTabs from '$lib/components/shell/SectionTabs.svelte';

  const tabs = [
    { id: 'installed', label: 'Installed' },
    { id: 'available', label: 'Available' },
    { id: 'credentials', label: 'Credentials' },
  ];
  let activeTab = $state('installed');
</script>

<svelte:head>
  <title>Skills | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
  <div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]">
    <div class="px-6 pt-4 pb-0">
      <h1 class="gc-panel-title text-[var(--gc-ink)]">Skills</h1>
    </div>
    <SectionTabs {tabs} bind:activeTab />
  </div>

  <div class="flex min-h-0 flex-1 items-center justify-center p-10">
    <div class="text-center">
      <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
      <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Skill management</p>
      <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
        Skill management is not connected to a backend yet.
      </p>
    </div>
  </div>
</div>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Skills page"
```
Expected: 3 tests passing.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/skills/
git commit -m "feat: Skills section — Installed · Available · Credentials shell with COMING SOON state"
```

---

### Task 3: Nodes page

**Files:**
- Modify: `src/routes/nodes/+page.svelte`
- Create: `src/routes/nodes/page.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/routes/nodes/page.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import NodesPage from './+page.svelte';

const baseData = {
  auth: { authenticated: true, password_configured: true, setup_required: false },
  project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
  navigation: [{ id: 'nodes', label: 'Nodes', href: '/nodes' }],
  onboarding: null,
  currentPath: '/nodes',
  currentSearch: '',
};

describe('Nodes page', () => {
  it('renders the Nodes heading', () => {
    const { body } = render(NodesPage, { props: { data: baseData } });
    expect(body).toContain('Nodes');
  });

  it('renders List and Capabilities tabs', () => {
    const { body } = render(NodesPage, { props: { data: baseData } });
    expect(body).toContain('List');
    expect(body).toContain('Capabilities');
  });

  it('renders COMING SOON state', () => {
    const { body } = render(NodesPage, { props: { data: baseData } });
    expect(body).toContain('COMING SOON');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Nodes page"
```
Expected: FAIL — page is a placeholder without tabs.

- [ ] **Step 3: Implement Nodes page**

```svelte
<!-- src/routes/nodes/+page.svelte -->
<script lang="ts">
  import SectionTabs from '$lib/components/shell/SectionTabs.svelte';

  const tabs = [
    { id: 'list', label: 'List' },
    { id: 'capabilities', label: 'Capabilities' },
  ];
  let activeTab = $state('list');
</script>

<svelte:head>
  <title>Nodes | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
  <div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]">
    <div class="px-6 pt-4 pb-0">
      <h1 class="gc-panel-title text-[var(--gc-ink)]">Nodes</h1>
    </div>
    <SectionTabs {tabs} bind:activeTab />
  </div>

  <div class="flex min-h-0 flex-1 items-center justify-center p-10">
    <div class="text-center">
      <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
      <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Node inventory</p>
      <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
        Node inventory is not connected to a backend yet.
      </p>
    </div>
  </div>
</div>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Nodes page"
```
Expected: 3 tests passing.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/nodes/
git commit -m "feat: Nodes section — List · Capabilities shell with COMING SOON state"
```

---

### Task 4: Debug page

**Files:**
- Modify: `src/routes/debug/+page.svelte`
- Create: `src/routes/debug/page.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/routes/debug/page.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import DebugPage from './+page.svelte';

const baseData = {
  auth: { authenticated: true, password_configured: true, setup_required: false },
  project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
  navigation: [{ id: 'debug', label: 'Debug', href: '/debug' }],
  onboarding: null,
  currentPath: '/debug',
  currentSearch: '',
};

describe('Debug page', () => {
  it('renders the Debug heading', () => {
    const { body } = render(DebugPage, { props: { data: baseData } });
    expect(body).toContain('Debug');
  });

  it('renders Status, Health, Models, Events, RPC tabs', () => {
    const { body } = render(DebugPage, { props: { data: baseData } });
    expect(body).toContain('Status');
    expect(body).toContain('Health');
    expect(body).toContain('Models');
    expect(body).toContain('Events');
    expect(body).toContain('RPC');
  });

  it('renders COMING SOON state', () => {
    const { body } = render(DebugPage, { props: { data: baseData } });
    expect(body).toContain('COMING SOON');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Debug page"
```
Expected: FAIL — page is a placeholder without tabs.

- [ ] **Step 3: Implement Debug page**

```svelte
<!-- src/routes/debug/+page.svelte -->
<script lang="ts">
  import SectionTabs from '$lib/components/shell/SectionTabs.svelte';

  const tabs = [
    { id: 'status', label: 'Status' },
    { id: 'health', label: 'Health' },
    { id: 'models', label: 'Models' },
    { id: 'events', label: 'Events' },
    { id: 'rpc', label: 'RPC' },
  ];
  let activeTab = $state('status');
</script>

<svelte:head>
  <title>Debug | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
  <div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]">
    <div class="px-6 pt-4 pb-0">
      <h1 class="gc-panel-title text-[var(--gc-ink)]">Debug</h1>
    </div>
    <SectionTabs {tabs} bind:activeTab />
  </div>

  <div class="flex min-h-0 flex-1 items-center justify-center p-10">
    {#if activeTab === 'rpc'}
      <div class="text-center">
        <p class="gc-stamp text-[var(--gc-warning)]">COMING SOON</p>
        <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">RPC console</p>
        <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
          The RPC console executes privileged runtime commands. It will require explicit confirmation
          before any command runs.
        </p>
      </div>
    {:else}
      <div class="text-center">
        <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
        <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Debug — {activeTab}</p>
        <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
          This debug panel is not connected to a backend endpoint yet.
        </p>
      </div>
    {/if}
  </div>
</div>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Debug page"
```
Expected: 3 tests passing.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/debug/
git commit -m "feat: Debug section — Status · Health · Models · Events · RPC shell with COMING SOON state"
```

---

### Task 5: Logs page

**Files:**
- Modify: `src/routes/logs/+page.svelte`
- Create: `src/routes/logs/page.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/routes/logs/page.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import LogsPage from './+page.svelte';

const baseData = {
  auth: { authenticated: true, password_configured: true, setup_required: false },
  project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
  navigation: [{ id: 'logs', label: 'Logs', href: '/logs' }],
  onboarding: null,
  currentPath: '/logs',
  currentSearch: '',
};

describe('Logs page', () => {
  it('renders the Logs heading', () => {
    const { body } = render(LogsPage, { props: { data: baseData } });
    expect(body).toContain('Logs');
  });

  it('renders Live Tail, Filters, Export tabs', () => {
    const { body } = render(LogsPage, { props: { data: baseData } });
    expect(body).toContain('Live Tail');
    expect(body).toContain('Filters');
    expect(body).toContain('Export');
  });

  it('renders COMING SOON state', () => {
    const { body } = render(LogsPage, { props: { data: baseData } });
    expect(body).toContain('COMING SOON');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Logs page"
```
Expected: FAIL — page is a placeholder without tabs.

- [ ] **Step 3: Implement Logs page**

```svelte
<!-- src/routes/logs/+page.svelte -->
<script lang="ts">
  import SectionTabs from '$lib/components/shell/SectionTabs.svelte';

  const tabs = [
    { id: 'tail', label: 'Live Tail' },
    { id: 'filters', label: 'Filters' },
    { id: 'export', label: 'Export' },
  ];
  let activeTab = $state('tail');
</script>

<svelte:head>
  <title>Logs | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
  <div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]">
    <div class="px-6 pt-4 pb-0">
      <h1 class="gc-panel-title text-[var(--gc-ink)]">Logs</h1>
    </div>
    <SectionTabs {tabs} bind:activeTab />
  </div>

  <div class="flex min-h-0 flex-1 items-center justify-center p-10">
    <div class="text-center">
      <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
      <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Log streaming</p>
      <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
        Log streaming is not connected to a backend yet.
      </p>
    </div>
  </div>
</div>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Logs page"
```
Expected: 3 tests passing.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/logs/
git commit -m "feat: Logs section — Live Tail · Filters · Export shell with COMING SOON state"
```

---

### Task 6: Update page

**Files:**
- Modify: `src/routes/update/+page.svelte`
- Create: `src/routes/update/page.test.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// src/routes/update/page.test.ts
import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import UpdatePage from './+page.svelte';

const baseData = {
  auth: { authenticated: true, password_configured: true, setup_required: false },
  project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
  navigation: [{ id: 'update', label: 'Update', href: '/update' }],
  onboarding: null,
  currentPath: '/update',
  currentSearch: '',
};

describe('Update page', () => {
  it('renders the Update heading', () => {
    const { body } = render(UpdatePage, { props: { data: baseData } });
    expect(body).toContain('Update');
  });

  it('renders Run Update and Restart Report tabs', () => {
    const { body } = render(UpdatePage, { props: { data: baseData } });
    expect(body).toContain('Run Update');
    expect(body).toContain('Restart Report');
  });

  it('renders COMING SOON state', () => {
    const { body } = render(UpdatePage, { props: { data: baseData } });
    expect(body).toContain('COMING SOON');
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Update page"
```
Expected: FAIL — page is a placeholder without tabs.

- [ ] **Step 3: Implement Update page**

```svelte
<!-- src/routes/update/+page.svelte -->
<script lang="ts">
  import SectionTabs from '$lib/components/shell/SectionTabs.svelte';

  const tabs = [
    { id: 'run', label: 'Run Update' },
    { id: 'report', label: 'Restart Report' },
  ];
  let activeTab = $state('run');
</script>

<svelte:head>
  <title>Update | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
  <div class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]">
    <div class="px-6 pt-4 pb-0">
      <h1 class="gc-panel-title text-[var(--gc-ink)]">Update</h1>
    </div>
    <SectionTabs {tabs} bind:activeTab />
  </div>

  <div class="flex min-h-0 flex-1 items-center justify-center p-10">
    <div class="text-center">
      <p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
      <p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Update workflow</p>
      <p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
        Update workflow is not connected to a backend yet.
      </p>
    </div>
  </div>
</div>
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd frontend && bun run test:unit -- --run --reporter=verbose 2>&1 | grep "Update page"
```
Expected: 3 tests passing.

- [ ] **Step 5: Final test sweep + check + lint**

```bash
cd frontend && bun run test:unit -- --run 2>&1 | tail -5
bun run check 2>&1 | tail -3
bun run format && bun run lint 2>&1 | tail -3
```
Expected: All tests pass across all pages, 0 errors, 0 warnings.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/routes/update/
git commit -m "feat: Update section — Run Update · Restart Report shell with COMING SOON state"
```

---

## Self-Review

**Spec coverage:**
- ✓ Instances: Presence · Details tabs, "Instances are not connected to a backend yet."
- ✓ Skills: Installed · Available · Credentials tabs, "Skill management is not connected to a backend yet."
- ✓ Nodes: List · Capabilities tabs, "Node inventory is not connected to a backend yet."
- ✓ Debug: Status · Health · Models · Events · RPC tabs, "not connected to a backend yet." (RPC tab includes danger warning)
- ✓ Logs: Live Tail · Filters · Export tabs, "Log streaming is not connected to a backend yet."
- ✓ Update: Run Update · Restart Report tabs, "Update workflow is not connected to a backend yet."
- ✓ All 6 pages pass their 3 tests each (18 total new tests)
- ✓ No fake data — all placeholder panels only
- ✓ `COMING SOON` stamp per spec for all placeholder sections

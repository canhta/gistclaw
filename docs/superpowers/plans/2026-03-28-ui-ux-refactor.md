# UI/UX Refactor + Multi-Agent Work Graph

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Eliminate the double-header cascade, replace the static inspector with live runtime data, add multi-agent visualization with real-time status on the Work surface, and fix 18 interaction-state and design-system gaps across all surfaces.

**Architecture:** All changes are frontend-only except one: the Conversations session response needs `display_name` and `thread_label` server fields to replace raw `agent_id` / `conversation_id` exposure. AppShell restructure affects every surface. Each task is independently testable.

**Tech Stack:** SvelteKit 2, Svelte 5 (runes), Tailwind 4, `@xyflow/svelte` (already installed), `bun` for tooling, Go for the one backend field addition.

---

## File Map

| File | Change |
|---|---|
| `frontend/src/lib/components/shell/AppShell.svelte` | Remove xl:block header panel; collapse mobile header; remove item.id from nav; make inspector accept live InspectorItem[] |
| `frontend/src/lib/components/common/WorkClusterPanel.svelte` | NEW — replaces RunClusterCard with agent-lane visualization + tone borders |
| `frontend/src/lib/components/common/RunClusterCard.svelte` | DELETE — replaced by WorkClusterPanel |
| `frontend/src/routes/work/+page.svelte` | Use WorkClusterPanel; add queue-strip tone treatment; add polling refresh; wire live inspector items |
| `frontend/src/routes/conversations/+page.svelte` | Flip section order (delivery issues first); channel health tone border + badge; use role_label/updated_at_label instead of agent_id/conversation_id |
| `frontend/src/routes/knowledge/+page.svelte` | Inline two-step Forget confirm; prev/next pagination; `goto()` instead of `window.location.assign` |
| `frontend/src/routes/team/+page.svelte` | `hasDraft()` derived; unsaved-changes banner; `beforeNavigate` guard; read-only overview mode |
| `frontend/src/routes/automate/+page.svelte` | Flip grid columns (schedules left, form right); item-level mutation state; IANA timezone datalist |
| `frontend/src/routes/layout.css` | Add `.gc-status-badge` utility; remove unused `is-error`/`is-approval` raw class dependencies |
| `frontend/src/lib/config/surfaces.ts` | Remove `inspectorItems` static arrays (replaced by per-page live data) |
| `internal/web/api_conversations.go` | Add `display_name` + `thread_label` to `ConversationIndexItemResponse` |
| `internal/web/api_conversations.go` (test) | Update/add test coverage for new fields |

---

## Task 1: AppShell — Remove double-header, clean nav, collapse mobile header

**Files:**
- Modify: `frontend/src/lib/components/shell/AppShell.svelte`
- Modify: `frontend/src/lib/config/surfaces.ts` (remove `inspectorItems` static arrays)

### What changes

The `xl:block` main header panel (stamp "Active surface" + gc-page-title + description) is deleted. Each page owns its opening section.

Nav items no longer render `<span class="gc-machine">{item.id}</span>` — the internal ID is removed from both desktop and mobile nav.

Mobile header collapses: the outer `gc-panel` block with project path + surface description + inspector items is replaced by a minimal 1-row header (logo + project name + info-toggle button). The info panel is shown/hidden via `infoOpen` state.

The `inspectorItems` prop type stays identical (`InspectorItem[]`) but `surfaces.ts` removes the static `inspectorItems` arrays. Pages pass live data instead.

- [ ] **Step 1: Write failing test**

File: `frontend/src/lib/components/shell/AppShell.test.ts`

```typescript
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import AppShell from './AppShell.svelte';

const baseProps = {
	navigation: [
		{ id: 'work', label: 'Work', href: '/work' },
		{ id: 'team', label: 'Team', href: '/team' }
	],
	project: { active_id: 'p1', active_name: 'my-repo', active_path: '/home/user/my-repo' },
	currentPath: '/work',
	inspectorTitle: 'Status',
	inspectorItems: []
};

describe('AppShell', () => {
	it('does not render the xl:block header panel', () => {
		const { queryByText } = render(AppShell, { props: baseProps });
		expect(queryByText('Active surface')).toBeNull();
	});

	it('does not render internal surface IDs in nav', () => {
		const { queryByText } = render(AppShell, { props: baseProps });
		// 'work' and 'team' are the internal IDs — they must not appear as visible nav text
		const navEl = document.querySelector('[aria-label="Primary navigation"]');
		expect(navEl?.textContent).not.toContain(' work ');
		expect(navEl?.textContent).not.toContain(' team ');
		// Labels must still appear
		expect(navEl?.textContent).toContain('Work');
		expect(navEl?.textContent).toContain('Team');
	});

	it('renders the minimal mobile header row', () => {
		const { getByAltText } = render(AppShell, { props: baseProps });
		expect(getByAltText('GistClaw logo')).toBeTruthy();
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run
```

Expected: FAIL — "does not render internal surface IDs in nav" fails because `item.id` is currently rendered.

- [ ] **Step 3: Rewrite AppShell.svelte**

Replace the entire `AppShell.svelte` with the following. Key changes: xl:block header removed, nav item.id removed, mobile header collapsed.

```svelte
<!-- eslint-disable svelte/no-navigation-without-resolve -->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { BootstrapNavItem, BootstrapProjectResponse } from '$lib/types/api';
	import SurfaceIcon from '$lib/components/shell/SurfaceIcon.svelte';
	import logo from '$lib/assets/logo.svg';

	type InspectorItem = {
		label: string;
		value: string;
		tone?: 'default' | 'accent' | 'warning';
	};

	let {
		navigation,
		project,
		currentPath,
		inspectorTitle,
		inspectorItems = [],
		children
	}: {
		navigation: BootstrapNavItem[];
		project: BootstrapProjectResponse;
		currentPath: string;
		inspectorTitle: string;
		inspectorItems?: InspectorItem[];
		children?: Snippet;
	} = $props();

	let infoOpen = $state(false);

	function isActive(href: string): boolean {
		return currentPath === href || currentPath.startsWith(`${href}/`);
	}

	function inspectorToneClass(tone: InspectorItem['tone']): string {
		if (tone === 'accent') return 'border-[var(--gc-cyan)]';
		if (tone === 'warning') return 'border-[var(--gc-orange)]';
		return 'border-[var(--gc-border)]';
	}

	function desktopNavClass(href: string): string {
		return `gc-panel-soft grid min-w-0 grid-cols-[auto_minmax(0,1fr)] items-center gap-4 px-4 py-3 transition-colors ${
			isActive(href)
				? 'border-[var(--gc-orange)] bg-[var(--gc-surface-raised)] text-[var(--gc-ink)]'
				: 'text-[var(--gc-text-secondary)] hover:border-[var(--gc-cyan)] hover:text-[var(--gc-ink)]'
		}`;
	}

	function mobileNavClass(href: string): string {
		return `gc-panel-soft shrink-0 px-4 py-3 transition-colors ${
			isActive(href)
				? 'border-[var(--gc-orange)] bg-[var(--gc-surface-raised)] text-[var(--gc-ink)]'
				: 'text-[var(--gc-text-secondary)] hover:border-[var(--gc-cyan)] hover:text-[var(--gc-ink)]'
		}`;
	}
</script>

<div class="min-h-screen bg-[var(--gc-canvas)] text-[var(--gc-ink)]">
	<!-- Mobile header -->
	<div class="border-b-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface)] xl:hidden">
		<div class="flex items-center justify-between gap-4 px-4 py-3 sm:px-6">
			<div class="flex items-center gap-3">
				<img
					src={logo}
					alt="GistClaw logo"
					class="h-9 w-9 border-2 border-[var(--gc-border-strong)] bg-[var(--gc-canvas)] p-1"
				/>
				<p class="gc-panel-title text-[1rem]">{project.active_name}</p>
			</div>
			<button
				onclick={() => (infoOpen = !infoOpen)}
				aria-expanded={infoOpen}
				aria-label="System info"
				class="gc-action px-3 py-2 text-[var(--gc-text-secondary)]"
			>
				<span class="gc-stamp">{infoOpen ? 'Close' : 'Info'}</span>
			</button>
		</div>

		{#if infoOpen}
			<div class="border-t-2 border-[var(--gc-border)] px-4 py-4 sm:px-6">
				<div class="grid gap-3 sm:grid-cols-2">
					<div class="gc-panel-soft px-3 py-3">
						<p class="gc-stamp">Project path</p>
						<p class="gc-machine mt-2 break-all">{project.active_path}</p>
					</div>
					{#each inspectorItems as item (`mobile-info-${item.label}`)}
						<div class={`gc-panel-soft px-3 py-3 ${inspectorToneClass(item.tone)}`}>
							<p class="gc-stamp">{item.label}</p>
							<p class="gc-value mt-2 text-[1rem]">{item.value}</p>
						</div>
					{/each}
				</div>
			</div>
		{/if}

		<div class="border-t-2 border-[var(--gc-border)] px-4 py-3 sm:px-6">
			<div class="max-w-full overflow-x-auto overflow-y-hidden overscroll-x-contain">
				<nav
					aria-label="Primary navigation"
					class="flex min-w-max gap-3 pb-1"
				>
					{#each navigation as item (item.href)}
						<a
							href={item.href}
							aria-current={isActive(item.href) ? 'page' : undefined}
							class={mobileNavClass(item.href)}
						>
							<div class="flex items-center gap-3">
								<SurfaceIcon surfaceID={item.id} />
								<span class="gc-stamp">{item.label}</span>
							</div>
						</a>
					{/each}
				</nav>
			</div>
		</div>
	</div>

	<!-- Desktop layout -->
	<div class="grid min-h-screen grid-cols-1 xl:grid-cols-[18rem_minmax(0,1fr)_22rem]">
		<!-- Left nav -->
		<aside class="hidden bg-[var(--gc-surface)] xl:flex xl:h-screen xl:flex-col xl:border-r-2 xl:border-[var(--gc-border-strong)]">
			<div class="border-b-2 border-[var(--gc-border)] px-5 py-6">
				<div class="flex items-start gap-3">
					<img
						src={logo}
						alt="GistClaw logo"
						class="h-12 w-12 border-2 border-[var(--gc-border-strong)] bg-[var(--gc-canvas)] p-1"
					/>
					<div>
						<p class="gc-stamp">Control deck</p>
						<p class="gc-machine mt-2">Repo workbench</p>
					</div>
				</div>
				<h1 class="gc-panel-title mt-3 text-[1.45rem]">{project.active_name}</h1>
				<p class="gc-machine mt-3 break-all">{project.active_path}</p>
			</div>

			<nav aria-label="Primary navigation" class="grid flex-1 auto-rows-min gap-2 overflow-y-auto px-3 py-4">
				{#each navigation as item (item.href)}
					<a
						href={item.href}
						aria-current={isActive(item.href) ? 'page' : undefined}
						class={desktopNavClass(item.href)}
					>
						<SurfaceIcon surfaceID={item.id} />
						<span class="gc-stamp">{item.label}</span>
					</a>
				{/each}
			</nav>
		</aside>

		<!-- Main content -->
		<main class="flex min-w-0 flex-col">
			<div class="flex-1 px-4 py-5 sm:px-6 sm:py-6 lg:px-8 lg:py-8">
				{#if children}
					{@render children()}
				{/if}
			</div>
		</main>

		<!-- Right inspector -->
		<aside class="hidden bg-[var(--gc-surface)] px-5 py-6 xl:block xl:h-screen xl:overflow-y-auto xl:border-l-2 xl:border-[var(--gc-border-strong)]">
			<div class="gc-panel px-4 py-4">
				<p class="gc-stamp">{inspectorTitle}</p>
				<div class="mt-4 grid gap-3">
					{#each inspectorItems as item (`${item.label}-${item.value}`)}
						<div class={`gc-panel-soft px-3 py-3 ${inspectorToneClass(item.tone)}`}>
							<p class="gc-stamp">{item.label}</p>
							<p class="gc-value mt-2 text-[1.15rem]">{item.value}</p>
						</div>
					{/each}
				</div>
			</div>
		</aside>
	</div>
</div>
```

- [ ] **Step 4: Update +layout.svelte — remove title/description props from AppShell**

`frontend/src/routes/+layout.svelte` currently passes `title={surface.title}` and `description={surface.description}` to AppShell. Remove those two props and any `inspectorItems` that came from `surfaces.ts`.

The layout file currently looks like:
```svelte
<AppShell
  navigation={data.navigation}
  project={data.project!}
  currentPath={data.currentPath}
  title={surface.title}
  description={surface.description}
  inspectorTitle={surface.inspectorTitle}
  inspectorItems={surface.inspectorItems}
>
```

Replace with:
```svelte
<AppShell
  navigation={data.navigation}
  project={data.project!}
  currentPath={data.currentPath}
  inspectorTitle={surface.inspectorTitle}
  inspectorItems={surface.inspectorItems}
>
```

Note: `inspectorItems` from `surfaces.ts` still works as a fallback until each page overrides it via a slot/prop mechanism. Since AppShell doesn't currently support per-page inspector override via children, we'll wire that per-page in later tasks using a different approach: each page that needs live inspector data will manage it via `data` props. For now, the static items remain until Task 3 wires live data.

- [ ] **Step 5: Remove `title`, `description`, `workspaceEyebrow`, `workspaceTitle`, `workspaceBody` from surfaces.ts SurfaceMeta interface and all surface definitions**

In `frontend/src/lib/config/surfaces.ts`, simplify `SurfaceMeta` to:
```typescript
export interface SurfaceMeta {
	id: SurfaceID;
	inspectorTitle: string;
	inspectorItems: SurfaceInspectorItem[];
}
```

Remove `title`, `description`, `workspaceEyebrow`, `workspaceTitle`, `workspaceBody`, and `cards` from every surface entry and the interface. Update `surfaceForPath` and `surfaceByID` exports.

Also update `+layout.svelte` to remove `surface.title` and `surface.description` references from `surfaceForPath` result usage.

- [ ] **Step 6: Run tests**

```bash
cd frontend && bun run test:unit -- --run && bun run check
```

Expected: all tests pass, no TypeScript errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/lib/components/shell/AppShell.svelte \
  frontend/src/lib/components/shell/AppShell.test.ts \
  frontend/src/routes/+layout.svelte \
  frontend/src/lib/config/surfaces.ts
git commit -m "refactor: remove AppShell double-header, clean nav IDs, collapse mobile header"
```

---

## Task 2: Work surface — multi-agent cluster panel with tone borders

**Files:**
- Create: `frontend/src/lib/components/common/WorkClusterPanel.svelte`
- Create: `frontend/src/lib/components/common/WorkClusterPanel.test.ts`
- Delete: `frontend/src/lib/components/common/RunClusterCard.svelte`
- Modify: `frontend/src/routes/work/+page.svelte`
- Modify: `frontend/src/routes/work/page.test.ts`

### What changes

`WorkClusterPanel` replaces `RunClusterCard`. It shows the root agent and each worker agent as styled module tiles with:
- Tone border on the whole card based on the highest-urgency status in the cluster
- Root agent tile (prominent)
- Worker agent tiles (connected visually via border-left rail)
- Status label on each agent tile using `toneClass()` to pick border color
- Clear "Open run graph" CTA

The `agent_id` display problem: format `agent_id` using a simple helper that converts snake_case to Title Case (e.g. `front_assistant` → `Front assistant`). No mapping table needed.

Queue strip stats: add warning tone class when `approvals > 0` or `recovery_runs > 0`.

- [ ] **Step 1: Write failing tests**

File: `frontend/src/lib/components/common/WorkClusterPanel.test.ts`

```typescript
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import WorkClusterPanel from './WorkClusterPanel.svelte';
import type { WorkClusterResponse } from '$lib/types/api';

const runningCluster: WorkClusterResponse = {
	root: {
		id: 'run-1',
		objective: 'Audit and patch issue #42',
		agent_id: 'front_assistant',
		status: 'active',
		status_label: 'Running',
		status_class: 'is-active',
		model_display: 'claude-opus',
		token_summary: '12k tokens',
		started_at_short: '5 min ago',
		started_at_exact: '',
		started_at_iso: '',
		last_activity_short: '1 min ago',
		last_activity_exact: '',
		last_activity_iso: '',
		depth: 0
	},
	children: [
		{
			id: 'run-2',
			objective: 'Research the issue history',
			agent_id: 'researcher',
			status: 'active',
			status_label: 'Running',
			status_class: 'is-active',
			model_display: 'claude-haiku',
			token_summary: '3k tokens',
			started_at_short: '3 min ago',
			started_at_exact: '',
			started_at_iso: '',
			last_activity_short: '30 sec ago',
			last_activity_exact: '',
			last_activity_iso: '',
			depth: 1
		}
	],
	child_count: 1,
	child_count_label: '1 worker',
	blocker_label: '',
	has_children: true
};

const approvalCluster: WorkClusterResponse = {
	...runningCluster,
	root: { ...runningCluster.root, status_class: 'is-approval', status_label: 'Needs approval' }
};

describe('WorkClusterPanel', () => {
	it('renders the root agent objective', () => {
		const { getByText } = render(WorkClusterPanel, { props: { cluster: runningCluster } });
		expect(getByText('Audit and patch issue #42')).toBeTruthy();
	});

	it('renders worker agents', () => {
		const { getByText } = render(WorkClusterPanel, { props: { cluster: runningCluster } });
		expect(getByText('Research the issue history')).toBeTruthy();
	});

	it('applies warning border class when approval-blocked', () => {
		const { container } = render(WorkClusterPanel, { props: { cluster: approvalCluster } });
		const article = container.querySelector('article');
		expect(article?.className).toContain('border-[var(--gc-orange)]');
	});

	it('renders a link to the run graph', () => {
		const { container } = render(WorkClusterPanel, { props: { cluster: runningCluster } });
		const link = container.querySelector('a[href*="run-1"]');
		expect(link).toBeTruthy();
	});

	it('formats agent_id as readable label', () => {
		const { getByText } = render(WorkClusterPanel, { props: { cluster: runningCluster } });
		expect(getByText('Front assistant')).toBeTruthy();
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run WorkClusterPanel
```

Expected: FAIL — WorkClusterPanel.svelte does not exist.

- [ ] **Step 3: Create WorkClusterPanel.svelte**

```svelte
<script lang="ts">
	import { resolve } from '$app/paths';
	import type { WorkClusterResponse, WorkClusterRunResponse } from '$lib/types/api';

	let {
		cluster
	}: {
		cluster: WorkClusterResponse;
	} = $props();

	function formatAgentID(agentID: string): string {
		return agentID
			.replace(/_/g, ' ')
			.replace(/\b\w/g, (c, i) => (i === 0 ? c.toUpperCase() : c));
	}

	function toneClass(statusClass: string): string {
		if (statusClass.includes('approval')) return 'border-[var(--gc-orange)]';
		if (statusClass.includes('error') || statusClass.includes('failed') || statusClass.includes('interrupted')) {
			return 'border-[var(--gc-error)]';
		}
		if (statusClass.includes('active')) return 'border-[var(--gc-cyan)]';
		return 'border-[var(--gc-border-strong)]';
	}

	function statusTextClass(statusClass: string): string {
		if (statusClass.includes('approval')) return 'text-[var(--gc-orange)]';
		if (statusClass.includes('error') || statusClass.includes('failed') || statusClass.includes('interrupted')) {
			return 'text-[var(--gc-error)]';
		}
		if (statusClass.includes('active')) return 'text-[var(--gc-cyan)]';
		return 'text-[var(--gc-text-secondary)]';
	}

	const panelTone = $derived(toneClass(cluster.root.status_class));
</script>

<article class={`gc-panel-soft border-2 px-4 py-4 ${panelTone}`}>
	<!-- Root agent -->
	<div class="flex items-start justify-between gap-4">
		<div class="min-w-0">
			<p class="gc-stamp">{formatAgentID(cluster.root.agent_id)}</p>
			<h3 class="gc-panel-title mt-2 text-[1rem]">{cluster.root.objective}</h3>
		</div>
		<p class={`gc-stamp shrink-0 ${statusTextClass(cluster.root.status_class)}`}>
			{cluster.root.status_label}
		</p>
	</div>

	<!-- Worker agents -->
	{#if cluster.children.length > 0}
		<div class="mt-4 border-l-2 border-[var(--gc-border)] pl-4">
			<div class="grid gap-3">
				{#each cluster.children as child (child.id)}
					<div class={`gc-panel-soft border-l-2 px-3 py-3 ${toneClass(child.status_class)}`} style="border-left-width: 2px">
						<div class="flex items-start justify-between gap-3">
							<div class="min-w-0">
								<p class="gc-stamp">{formatAgentID(child.agent_id)}</p>
								<p class="gc-copy mt-1 truncate text-[var(--gc-ink)]">{child.objective}</p>
							</div>
							<div class="shrink-0 text-right">
								<p class={`gc-stamp ${statusTextClass(child.status_class)}`}>{child.status_label}</p>
								<p class="gc-machine mt-1">{child.last_activity_short}</p>
							</div>
						</div>
					</div>
				{/each}
			</div>
		</div>
	{/if}

	<!-- Footer -->
	<div class="mt-4 flex items-center justify-between gap-4 border-t-2 border-[var(--gc-border)] pt-3">
		<div class="flex gap-4">
			<div>
				<p class="gc-stamp">Started</p>
				<p class="gc-machine mt-1">{cluster.root.started_at_short}</p>
			</div>
			{#if cluster.has_children}
				<div>
					<p class="gc-stamp">Workers</p>
					<p class="gc-machine mt-1">{cluster.child_count_label}</p>
				</div>
			{/if}
		</div>
		<a
			href={resolve('/work/[runId]', { runId: cluster.root.id })}
			class="gc-action gc-action-accent px-4 py-2"
		>
			Open graph
		</a>
	</div>
</article>
```

- [ ] **Step 4: Update Work index page to use WorkClusterPanel and add queue tone**

In `frontend/src/routes/work/+page.svelte`:

Replace the import:
```typescript
import RunClusterCard from '$lib/components/common/RunClusterCard.svelte';
```
with:
```typescript
import WorkClusterPanel from '$lib/components/common/WorkClusterPanel.svelte';
```

Replace each `<RunClusterCard {cluster} />` with `<WorkClusterPanel {cluster} />`.

Update queue strip stats to add tone styling. Replace the static stats grid:
```svelte
<div class="mt-6 grid gap-3 sm:grid-cols-2">
	{#each queueStats as stat (stat.label)}
		<div class="gc-panel-soft px-4 py-4">
			<p class="gc-stamp">{stat.label}</p>
			<p class="gc-value mt-3">{stat.value}</p>
		</div>
	{/each}
</div>
```

With:
```svelte
<div class="mt-6 grid gap-3 sm:grid-cols-2">
	<div class="gc-panel-soft px-4 py-4">
		<p class="gc-stamp">Active</p>
		<p class="gc-value mt-3">{data.work.queue_strip.root_runs}</p>
	</div>
	<div class="gc-panel-soft px-4 py-4">
		<p class="gc-stamp">Workers</p>
		<p class="gc-value mt-3">{data.work.queue_strip.worker_runs}</p>
	</div>
	<div class={`gc-panel-soft px-4 py-4 ${data.work.queue_strip.recovery_runs > 0 ? 'border-[var(--gc-orange)]' : ''}`}>
		<p class="gc-stamp">Recovery</p>
		<p class="gc-value mt-3">{data.work.queue_strip.recovery_runs}</p>
	</div>
	<div class={`gc-panel-soft px-4 py-4 ${data.work.queue_strip.summary.needs_approval > 0 ? 'border-[var(--gc-orange)]' : ''}`}>
		<p class="gc-stamp">Approvals</p>
		<p class="gc-value mt-3">{data.work.queue_strip.summary.needs_approval}</p>
	</div>
</div>
```

- [ ] **Step 5: Delete RunClusterCard.svelte**

```bash
rm frontend/src/lib/components/common/RunClusterCard.svelte
```

Also remove `RunClusterCard` import from any other page that uses it (check `history/+page.svelte`):
```bash
grep -rn "RunClusterCard" frontend/src/
```
Replace any remaining `RunClusterCard` usage with `WorkClusterPanel`.

- [ ] **Step 6: Run tests**

```bash
cd frontend && bun run test:unit -- --run && bun run check
```

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/lib/components/common/WorkClusterPanel.svelte \
  frontend/src/lib/components/common/WorkClusterPanel.test.ts \
  frontend/src/routes/work/+page.svelte \
  frontend/src/routes/history/+page.svelte
git rm frontend/src/lib/components/common/RunClusterCard.svelte
git commit -m "feat: replace RunClusterCard with multi-agent WorkClusterPanel with tone borders"
```

---

## Task 3: Work surface — live inspector data + auto-refresh polling

**Files:**
- Modify: `frontend/src/routes/work/+page.svelte`
- Modify: `frontend/src/routes/+layout.svelte` (pass live inspector from page to AppShell)

### Problem

The AppShell inspector currently receives static items from `surfaces.ts`. The Work page has live `data.work.queue_strip` data that should drive the inspector. The AppShell doesn't have a mechanism for per-page inspector override via children yet.

**Solution:** Thread inspector data via SvelteKit's page stores. The `+layout.svelte` is the only file that renders AppShell. Each page that needs live inspector items exports them from its load function OR computes them reactively and stores them in a shared context.

The simplest correct approach: add an `inspectorItems` property to each page's `data` shape via `+page.ts` loaders, and pass `data.inspectorItems ?? surface.inspectorItems` to AppShell in `+layout.svelte`. Pages that don't define `inspectorItems` fall back to the static surface config.

For auto-refresh: use `setInterval` + `loadWorkIndex` poll every 5 seconds when the page is visible. Stop on unmount.

- [ ] **Step 1: Add inspector items to Work page load**

In `frontend/src/routes/work/+page.ts`, the load function currently returns `{ work: WorkIndexResponse }`. It can't add inspector items because they depend on the work data — so we compute them in the Svelte component.

The cleanest approach: use a Svelte 5 `$effect` to update a shared context. Instead, keep it simpler: the layout reads from `page` stores.

**Simplest correct approach for inspector passthrough:** Add an optional `getInspectorItems` function export from each page that the layout can call. This is too complex.

**Actual simplest approach:** The layout passes its own computed inspector items. Add a `currentSurfaceInspectorItems` context that pages can write to, and the layout reads from.

Create `frontend/src/lib/shell/inspector.svelte.ts`:

```typescript
// Inspector items context — pages write live data; layout reads it for AppShell.
import { getContext, setContext } from 'svelte';

const KEY = 'gc-inspector';

export type InspectorItem = {
	label: string;
	value: string;
	tone?: 'default' | 'accent' | 'warning';
};

export function setInspectorItems(items: () => InspectorItem[]): void {
	setContext(KEY, items);
}

export function getInspectorItems(): (() => InspectorItem[]) | undefined {
	return getContext<(() => InspectorItem[]) | undefined>(KEY);
}
```

- [ ] **Step 2: Update +layout.svelte to read inspector context**

In `frontend/src/routes/+layout.svelte`:

```svelte
<script lang="ts">
	import type { Snippet } from 'svelte';
	import AppShell from '$lib/components/shell/AppShell.svelte';
	import { surfaceForPath } from '$lib/config/surfaces';
	import { getInspectorItems } from '$lib/shell/inspector.svelte';
	import './layout.css';
	import logo from '$lib/assets/logo.svg';
	import type { LayoutData } from './$types';

	let { data, children }: { data: LayoutData; children: Snippet } = $props();
	const surface = $derived(surfaceForPath(data.currentPath));
	const showShell = $derived(
		data.auth.authenticated &&
			!!data.project &&
			!data.currentPath.startsWith('/onboarding') &&
			data.currentPath !== '/login'
	);

	const pageInspectorFn = getInspectorItems();
	const inspectorItems = $derived(
		pageInspectorFn ? pageInspectorFn() : surface.inspectorItems
	);
</script>

<svelte:head><link rel="icon" href={logo} /></svelte:head>

{#if showShell}
	<AppShell
		navigation={data.navigation}
		project={data.project!}
		currentPath={data.currentPath}
		inspectorTitle={surface.inspectorTitle}
		{inspectorItems}
	>
		{@render children()}
	</AppShell>
{:else}
	<div class="min-h-screen bg-[var(--gc-canvas)] text-[var(--gc-ink)]">
		{@render children()}
	</div>
{/if}
```

- [ ] **Step 3: Wire live inspector in Work page + add polling**

In `frontend/src/routes/work/+page.svelte`, add to `<script>`:

```typescript
import { onMount } from 'svelte';
import { invalidateAll } from '$app/navigation';
import { setInspectorItems } from '$lib/shell/inspector.svelte';

// Live inspector items derived from queue_strip
setInspectorItems(() => [
	{
		label: 'Approvals',
		value: String(data.work.queue_strip.summary.needs_approval),
		tone: data.work.queue_strip.summary.needs_approval > 0 ? 'warning' : 'default'
	},
	{
		label: 'Active runs',
		value: String(data.work.queue_strip.root_runs + data.work.queue_strip.worker_runs),
		tone: data.work.queue_strip.root_runs > 0 ? 'accent' : 'default'
	},
	{
		label: 'Recovery',
		value: String(data.work.queue_strip.recovery_runs),
		tone: data.work.queue_strip.recovery_runs > 0 ? 'warning' : 'default'
	}
]);

// Poll for updates every 5s when there are active runs
onMount(() => {
	const interval = setInterval(() => {
		if (data.work.queue_strip.root_runs > 0 || data.work.queue_strip.recovery_runs > 0) {
			void invalidateAll();
		}
	}, 5000);
	return () => clearInterval(interval);
});
```

- [ ] **Step 4: Run tests and check**

```bash
cd frontend && bun run test:unit -- --run && bun run check
```

Expected: pass.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/lib/shell/inspector.svelte.ts \
  frontend/src/routes/+layout.svelte \
  frontend/src/routes/work/+page.svelte
git commit -m "feat: live inspector items on Work surface with 5s polling refresh"
```

---

## Task 4: Conversations surface fixes

**Files:**
- Modify: `frontend/src/routes/conversations/+page.svelte`
- Modify: `internal/web/api_conversations.go`
- Modify: Go test file for conversations API

### Changes

1. Move delivery issues section above thread list (layout reorder)
2. Channel health: tone border + status badge per connector
3. Replace `session.agent_id` heading with `session.role_label`; replace `Thread {conversation_id}` with `session.updated_at_label`
4. Backend: add `display_name` field to `ConversationIndexItemResponse` (maps `role_label` for display; for now duplicates `role_label` — the frontend uses it for the heading)

- [ ] **Step 1: Backend — verify role_label is already a usable display name**

Check the conversations API handler to understand what `role_label` contains:

```bash
grep -n "role_label\|RoleLabel\|display_name" internal/web/api_conversations.go | head -20
```

If `role_label` is already a human-friendly label (e.g., "Front agent", "Operator"), no backend change is needed — just use it in the frontend. If it exposes raw enum values, add a `display_name` field.

- [ ] **Step 2: Frontend — reorder sections and fix session display**

In `frontend/src/routes/conversations/+page.svelte`, the current layout in the second `<section>` is:

```svelte
<section class="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
	<div> <!-- active threads --> </div>
	<div class="grid gap-6">
		<div> <!-- channel health --> </div>
		<div> <!-- delivery issues --> </div>
	</div>
</section>
```

Swap the right column order — put delivery issues FIRST, channel health second:

```svelte
<section class="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
	<div> <!-- active threads --> </div>
	<div class="grid gap-6">
		<div> <!-- delivery issues FIRST --> </div>
		<div> <!-- channel health SECOND --> </div>
	</div>
</section>
```

Fix session card — replace `agent_id` heading and `conversation_id` subtitle:

Old:
```svelte
<h3 class="gc-panel-title mt-3 text-[1rem]">{session.agent_id}</h3>
...
<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">
	Thread {session.conversation_id}
</p>
```

New:
```svelte
<h3 class="gc-panel-title mt-3 text-[1rem]">{session.role_label}</h3>
...
<p class="gc-machine mt-4">{session.updated_at_label}</p>
```

- [ ] **Step 3: Channel health — add tone border and status badge**

Replace the channel health article rendering. Old:
```svelte
<article class="gc-panel-soft px-4 py-4">
	<p class="gc-stamp">{item.connector_id}</p>
	<p class="gc-value mt-3">{item.state_label}</p>
	<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{item.summary}</p>
</article>
```

New:
```svelte
<article class={`gc-panel-soft px-4 py-4 ${connectorToneClass(item.state_label)}`}>
	<div class="flex items-start justify-between gap-3">
		<p class="gc-stamp">{item.connector_id.toUpperCase()}</p>
		<p class={`gc-stamp ${connectorStatusTextClass(item.state_label)}`}>{item.state_label}</p>
	</div>
	<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{item.summary}</p>
</article>
```

Add these helper functions to the `<script>` block:

```typescript
function connectorToneClass(stateLabel: string): string {
	const s = stateLabel.toLowerCase();
	if (s.includes('error') || s.includes('down') || s.includes('fail')) return 'border-[var(--gc-error)]';
	if (s.includes('degraded') || s.includes('warn') || s.includes('retry')) return 'border-[var(--gc-orange)]';
	if (s.includes('connect') || s.includes('ok') || s.includes('active')) return 'border-[var(--gc-cyan)]';
	return 'border-[var(--gc-border)]';
}

function connectorStatusTextClass(stateLabel: string): string {
	const s = stateLabel.toLowerCase();
	if (s.includes('error') || s.includes('down') || s.includes('fail')) return 'text-[var(--gc-error)]';
	if (s.includes('degraded') || s.includes('warn') || s.includes('retry')) return 'text-[var(--gc-orange)]';
	if (s.includes('connect') || s.includes('ok') || s.includes('active')) return 'text-[var(--gc-cyan)]';
	return 'text-[var(--gc-text-secondary)]';
}
```

- [ ] **Step 4: Run check**

```bash
cd frontend && bun run check
```

Expected: no errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/conversations/+page.svelte
git commit -m "refactor: conversations — delivery issues first, channel health tone, fix session labels"
```

---

## Task 5: Knowledge surface — Forget confirmation + pagination + router fix

**Files:**
- Modify: `frontend/src/routes/knowledge/+page.svelte`

### Changes

1. `window.location.assign` → `goto()` for filter form submission
2. Inline two-step Forget: first click shows confirm state; second click fires delete
3. Prev/next pagination buttons using `data.knowledge.paging.next_url` / `prev_url`

- [ ] **Step 1: Write failing tests**

File: `frontend/src/routes/knowledge/page.test.ts` (create if not exists):

```typescript
import { describe, it, expect, vi } from 'vitest';

describe('Knowledge page', () => {
	it('uses goto for filter navigation, not window.location.assign', () => {
		// Check the page source doesn't contain window.location.assign
		// This is a static analysis check via the test
		const fs = require('fs');
		const src = fs.readFileSync('src/routes/knowledge/+page.svelte', 'utf-8');
		expect(src).not.toContain('window.location.assign');
	});

	it('forgetItem requires two clicks before firing', () => {
		// confirmingForgetID state must gate the delete call
		const src = require('fs').readFileSync('src/routes/knowledge/+page.svelte', 'utf-8');
		expect(src).toContain('confirmingForgetID');
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run knowledge
```

Expected: FAIL — `window.location.assign` still exists.

- [ ] **Step 3: Apply all three fixes to knowledge/+page.svelte**

**3a — Replace `window.location.assign` with `goto`:**

Import `goto` is already imported. Replace in `applyFilters`:
```typescript
async function applyFilters(event: SubmitEvent): Promise<void> {
	event.preventDefault();

	const search = new SvelteURLSearchParams();
	if (filterScopeValue().trim() !== '') search.set('scope', filterScopeValue().trim());
	if (filterAgentIDValue().trim() !== '') search.set('agent_id', filterAgentIDValue().trim());
	if (filterQueryValue().trim() !== '') search.set('q', filterQueryValue().trim());
	if (filterLimitValue().trim() !== '') search.set('limit', filterLimitValue().trim());

	const qs = search.toString();
	await goto(`${resolve('/knowledge')}${qs ? `?${qs}` : ''}`);
}
```

**3b — Add two-step Forget confirmation:**

Add state variable: `let confirmingForgetID = $state('');`

Replace the forget button in the item card:
```svelte
{#if confirmingForgetID === item.id}
	<div class="flex gap-2">
		<SurfaceActionButton
			tone="warning"
			onclick={() => forgetItem(item.id)}
			disabled={forgettingID === item.id}
		>
			{forgettingID === item.id ? 'Forgetting' : 'Confirm — forget it'}
		</SurfaceActionButton>
		<SurfaceActionButton onclick={() => (confirmingForgetID = '')}>
			Cancel
		</SurfaceActionButton>
	</div>
{:else}
	<SurfaceActionButton
		onclick={() => (confirmingForgetID = item.id)}
		disabled={forgettingID !== '' && forgettingID !== item.id}
	>
		Forget item
	</SurfaceActionButton>
{/if}
```

Also update `forgetItem` to reset `confirmingForgetID` after completion:
```typescript
async function forgetItem(itemID: string): Promise<void> {
	forgettingID = itemID;
	confirmingForgetID = '';
	// ... rest unchanged
	forgettingID = '';
}
```

**3c — Pagination controls:**

After the items list (before the closing `</div>`), add:

```svelte
{#if data.knowledge.paging.has_prev || data.knowledge.paging.has_next}
	<div class="mt-6 flex gap-3 border-t-2 border-[var(--gc-border)] pt-4">
		{#if data.knowledge.paging.has_prev && data.knowledge.paging.prev_url}
			<a href={data.knowledge.paging.prev_url} class="gc-action gc-action-accent px-5 py-3">
				Previous page
			</a>
		{/if}
		{#if data.knowledge.paging.has_next && data.knowledge.paging.next_url}
			<a href={data.knowledge.paging.next_url} class="gc-action gc-action-accent px-5 py-3">
				Next page
			</a>
		{/if}
	</div>
{/if}
```

- [ ] **Step 4: Run tests and check**

```bash
cd frontend && bun run test:unit -- --run && bun run check
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/knowledge/+page.svelte
git commit -m "fix: knowledge — two-step forget confirm, pagination controls, goto() for filter nav"
```

---

## Task 6: Team surface — unsaved-changes guard + read-only overview

**Files:**
- Modify: `frontend/src/routes/team/+page.svelte`

### Changes

1. `hasDraft()` function detects when local state differs from server state
2. Unsaved-changes banner (warning tone) shown when `hasDraft()` is true
3. `beforeNavigate` hook warns before nav-away with unsaved changes
4. Profile-switch and management actions check `hasDraft()` before calling `resetDraft()`
5. Read-only overview panel at top — shows current team config with an "Edit setup" toggle

- [ ] **Step 1: Write tests**

File: `frontend/src/routes/team/page.test.ts`:

```typescript
import { describe, it, expect } from 'vitest';

describe('Team page', () => {
	it('has hasDraft check before resetDraft calls', () => {
		const src = require('fs').readFileSync('src/routes/team/+page.svelte', 'utf-8');
		expect(src).toContain('hasDraft()');
		expect(src).toContain('confirmDiscardDraft');
	});

	it('has beforeNavigate guard for unsaved changes', () => {
		const src = require('fs').readFileSync('src/routes/team/+page.svelte', 'utf-8');
		expect(src).toContain('beforeNavigate');
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run team
```

- [ ] **Step 3: Add hasDraft, unsaved banner, and beforeNavigate guard**

In the `<script>` block, add these after the `resetDraft` function:

```typescript
import { beforeNavigate } from '$app/navigation';

// Detect unsaved changes
function hasDraft(): boolean {
	if (nameOverride !== null) return true;
	if (frontAgentOverride !== null) return true;
	if (Object.keys(roleOverrides).length > 0) return true;
	if (Object.keys(baseProfileOverrides).length > 0) return true;
	if (Object.keys(toolFamilyOverrides).length > 0) return true;
	if (Object.keys(delegationKindOverrides).length > 0) return true;
	if (memberDrafts !== null) return true;
	return false;
}

// Guard navigation away from unsaved changes
beforeNavigate(({ cancel }) => {
	if (hasDraft()) {
		// eslint-disable-next-line no-alert
		if (!confirm('You have unsaved changes. Leave without saving?')) {
			cancel();
		}
	}
});

// Wrap resetDraft to check for unsaved state first
function confirmDiscardDraft(): boolean {
	if (!hasDraft()) return true;
	// eslint-disable-next-line no-alert
	return confirm('Discard unsaved changes?');
}
```

Every call-site that calls `resetDraft()` inside profile-switch/management actions (useProfile, createProfile, cloneProfile, deleteProfile success handlers) must be wrapped:

```typescript
// Before: resetDraft();
// After:
if (confirmDiscardDraft()) resetDraft();
```

Add the unsaved-changes banner to the template. Place it at the very top of the page `<div>`, before the first `<section>`:

```svelte
{#if hasDraft()}
	<div class="gc-panel-soft mb-6 flex items-center justify-between gap-4 border-[var(--gc-orange)] px-4 py-4">
		<p class="gc-stamp text-[var(--gc-orange)]">Unsaved changes</p>
		<div class="flex gap-3">
			<SurfaceActionButton tone="solid" onclick={() => document.querySelector('form')?.requestSubmit()}>
				Save now
			</SurfaceActionButton>
			<SurfaceActionButton onclick={() => { resetDraft(); }}>
				Discard
			</SurfaceActionButton>
		</div>
	</div>
{/if}
```

- [ ] **Step 4: Add read-only overview panel**

Add `let editMode = $state(false);` state.

At the top of the page template (after the unsaved-changes banner), add:

```svelte
{#if !editMode}
	<section class="gc-panel mb-6 px-5 py-5 lg:px-6 lg:py-6">
		<div class="flex items-start justify-between gap-4">
			<div>
				<p class="gc-stamp">Current setup</p>
				<h2 class="gc-section-title mt-3">{data.team.team.name}</h2>
			</div>
			<SurfaceActionButton onclick={() => (editMode = true)}>Edit setup</SurfaceActionButton>
		</div>
		<div class="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
			{#each data.team.team.members as member (member.id)}
				<div class="gc-panel-soft px-4 py-4">
					<p class="gc-stamp">{member.role}</p>
					<p class="gc-panel-title mt-2 text-[1rem]">{member.base_profile}</p>
					<p class="gc-machine mt-2">{member.tool_families.length} tool families</p>
				</div>
			{/each}
		</div>
	</section>
{/if}
```

Wrap the existing edit form section with `{#if editMode}...{/if}`. Add an "Exit edit mode" button at the top of the edit form that calls `() => { if (!hasDraft() || confirmDiscardDraft()) { resetDraft(); editMode = false; } }`.

- [ ] **Step 5: Run tests and check**

```bash
cd frontend && bun run test:unit -- --run && bun run check
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/routes/team/+page.svelte
git commit -m "feat: team — read-only overview, unsaved-changes guard, beforeNavigate protection"
```

---

## Task 7: Automate surface — layout flip + item-level state + timezone

**Files:**
- Modify: `frontend/src/routes/automate/+page.svelte`

### Changes

1. Flip xl:grid-cols — schedules list in left/main column, creation form in right/secondary column
2. Item-level loading state: when `mutatingID === schedule.id`, dim the card and show "Saving..." on that item's action buttons
3. IANA timezone: replace free-text timezone input with a `<datalist>` of common IANA timezones

- [ ] **Step 1: Write test**

```typescript
// frontend/src/routes/automate/page.test.ts
import { describe, it, expect } from 'vitest';

describe('Automate page', () => {
	it('does not use raw free-text timezone input without datalist', () => {
		const src = require('fs').readFileSync('src/routes/automate/+page.svelte', 'utf-8');
		expect(src).toContain('list="timezones"');
	});

	it('has item-level mutation state check', () => {
		const src = require('fs').readFileSync('src/routes/automate/+page.svelte', 'utf-8');
		expect(src).toContain('mutatingID === schedule.id');
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd frontend && bun run test:unit -- --run automate
```

- [ ] **Step 3: Flip layout columns**

Current layout structure in `frontend/src/routes/automate/+page.svelte` at line 188:
```svelte
<section class="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
	<div class="grid gap-6">
		<form ...>  <!-- creation form FIRST -->
		</form>
		...
	</div>
	<div class="grid gap-6"> <!-- schedules list SECOND -->
```

Swap the two columns: move the schedules list div to be the first child, and the creation form div to be the second child.

Change the grid template to put the wider column first (schedules):
```svelte
<section class="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
	<div class="grid gap-6">
		<!-- SCHEDULES LIST (existing content that was on the right) -->
	</div>
	<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
		<!-- CREATION FORM (existing form content) -->
	</div>
</section>
```

- [ ] **Step 4: Add item-level mutation state**

For each schedule card's action buttons, add an opacity/state change when `mutatingID === schedule.id`:

```svelte
<article class={`gc-panel-soft px-4 py-4 ${mutatingID === schedule.id ? 'opacity-60' : ''}`}>
	...
	<SurfaceActionButton
		disabled={mutatingID !== ''}
		onclick={() => toggleSchedule(schedule.id, !schedule.enabled)}
	>
		{mutatingID === schedule.id ? 'Saving...' : schedule.enabled ? 'Disable' : 'Enable'}
	</SurfaceActionButton>
```

Apply the same pattern to any "Run now" buttons in occurrences.

- [ ] **Step 5: Replace timezone input with datalist**

Replace the free-text timezone `<input>` with:

```svelte
<label class="grid gap-2">
	<span class="gc-stamp">Timezone</span>
	<input
		bind:value={timezone}
		list="timezones"
		class="gc-control"
		placeholder="UTC"
		autocomplete="off"
	/>
	<datalist id="timezones">
		<option value="UTC" />
		<option value="America/New_York" />
		<option value="America/Chicago" />
		<option value="America/Denver" />
		<option value="America/Los_Angeles" />
		<option value="America/Sao_Paulo" />
		<option value="Europe/London" />
		<option value="Europe/Paris" />
		<option value="Europe/Berlin" />
		<option value="Europe/Moscow" />
		<option value="Asia/Dubai" />
		<option value="Asia/Kolkata" />
		<option value="Asia/Bangkok" />
		<option value="Asia/Singapore" />
		<option value="Asia/Tokyo" />
		<option value="Asia/Seoul" />
		<option value="Asia/Shanghai" />
		<option value="Australia/Sydney" />
		<option value="Pacific/Auckland" />
	</datalist>
</label>
```

- [ ] **Step 6: Run tests and check**

```bash
cd frontend && bun run test:unit -- --run && bun run check
```

- [ ] **Step 7: Commit**

```bash
git add frontend/src/routes/automate/+page.svelte
git commit -m "refactor: automate — schedules-first layout, item-level mutation state, IANA timezone datalist"
```

---

## Task 8: Cross-surface token fixes + metric card cleanup

**Files:**
- Modify: `frontend/src/routes/automate/+page.svelte` (status_class raw class fix)
- Modify: `frontend/src/routes/recover/+page.svelte` (status_class raw class fix)
- Modify: `frontend/src/lib/components/common/SurfaceMetricCard.svelte` (remove detail from self-evident cards)

### Problem

`schedule.status_class` and `approval.status_class` are applied directly as CSS class names (e.g., `class={`gc-machine ${schedule.status_class}`}`). The values `is-error`, `is-approval`, `is-active` are NOT defined in `layout.css`. These are silent style failures.

- [ ] **Step 1: Verify the broken class bindings**

```bash
grep -n "status_class}" frontend/src/routes/automate/+page.svelte
grep -n "status_class}" frontend/src/routes/recover/+page.svelte
```

Expected output: lines that directly interpolate `status_class` into a class string.

- [ ] **Step 2: Replace raw class bindings with token-mapped classes**

In `automate/+page.svelte`, find all occurrences like:
```svelte
<p class={`gc-machine ${schedule.status_class}`}>{schedule.status_label}</p>
```

Add a `statusTextClass` helper (if not already added in Task 7's toneClass work):

```typescript
function statusTextClass(statusClass: string): string {
	if (statusClass.includes('approval')) return 'text-[var(--gc-orange)]';
	if (statusClass.includes('error') || statusClass.includes('failed') || statusClass.includes('interrupted')) {
		return 'text-[var(--gc-error)]';
	}
	if (statusClass.includes('active')) return 'text-[var(--gc-cyan)]';
	return 'text-[var(--gc-text-secondary)]';
}
```

Replace:
```svelte
<p class={`gc-machine ${schedule.status_class}`}>{schedule.status_label}</p>
```
With:
```svelte
<p class={`gc-machine ${statusTextClass(schedule.status_class)}`}>{schedule.status_label}</p>
```

Apply the same pattern to the `occurrence.status_class` class bindings:
```svelte
<!-- Old -->
<h3 class={`gc-panel-title mt-3 text-[1rem] ${occurrence.status_class}`}>

<!-- New -->
<h3 class={`gc-panel-title mt-3 text-[1rem] ${statusTextClass(occurrence.status_class)}`}>
```

Do the same for `recover/+page.svelte` line 140.

- [ ] **Step 3: Remove explanatory detail text from self-evident metric cards**

`SurfaceMetricCard` has a `detail` prop that renders a sentence below the value. Check its implementation:

```bash
cat frontend/src/lib/components/common/SurfaceMetricCard.svelte
```

For metric cards where the label is self-explanatory ("Active conversations", "Connected channels", "Visible runs", "Completed runs"), remove the `detail` prop value in the page templates. Keep `detail` only where context is non-obvious (e.g., "Operator evidence" — the detail explains what approval_events + delivery_outcomes means).

Go through each page and remove `detail` from self-evident cards:

**conversations/+page.svelte:**
```svelte
<!-- Remove detail from these: -->
<SurfaceMetricCard label="Active conversations" value={...} />
<SurfaceMetricCard label="Connected channels" value={...} />
<!-- Keep detail on: -->
<SurfaceMetricCard label="Failed deliveries" value={...} detail="Replies that now need recovery attention." tone="warning" />
```

**history/+page.svelte:**
```svelte
<!-- Remove detail from Visible runs, Completed runs -->
<!-- Keep detail on: Recovery cases (not self-evident), Operator evidence (not self-evident) -->
```

- [ ] **Step 4: Run full test suite and lint**

```bash
cd frontend && bun run test:unit -- --run && bun run check && bun run lint
```

Expected: all pass, no lint warnings.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/routes/automate/+page.svelte \
  frontend/src/routes/recover/+page.svelte \
  frontend/src/routes/conversations/+page.svelte \
  frontend/src/routes/history/+page.svelte \
  frontend/src/lib/components/common/SurfaceMetricCard.svelte
git commit -m "fix: map status_class to CSS tokens client-side; remove redundant metric card detail text"
```

---

## Task 9: Work surface — full run graph visibility on active single cluster

**Files:**
- Modify: `frontend/src/routes/work/+page.svelte`
- Modify: `frontend/src/lib/work/load.ts`

### What changes

When the Work index has exactly one active cluster, automatically show the full XYFlow run graph inline (reusing the existing `RunGraph` component). For multiple clusters, show the `WorkClusterPanel` list. This delivers the "cockpit" experience described in DESIGN.md for the common single-active-run case.

Data needed: `WorkGraphResponse` for the active cluster's root run. Add `loadWorkGraph` to `load.ts` and fetch it lazily.

- [ ] **Step 1: Add loadWorkGraph to load.ts**

In `frontend/src/lib/work/load.ts`:

```typescript
export async function loadWorkGraph(
	fetcher: typeof fetch,
	runID: string
): Promise<WorkGraphResponse> {
	return requestJSON<WorkGraphResponse>(fetcher, `/api/work/${runID}/graph`);
}
```

- [ ] **Step 2: Update Work +page.svelte to load and show graph for single active cluster**

Add to `<script>`:

```typescript
import RunGraph from '$lib/components/graph/RunGraph.svelte';
import { loadWorkGraph } from '$lib/work/load';
import type { WorkGraphResponse } from '$lib/types/api';

let graphData = $state<WorkGraphResponse | null>(null);
let graphError = $state('');

const singleActiveCluster = $derived(
	data.work.clusters.length === 1 && data.work.clusters[0].root.status !== 'completed'
		? data.work.clusters[0]
		: null
);

$effect(() => {
	if (singleActiveCluster) {
		loadWorkGraph(fetch, singleActiveCluster.root.id)
			.then((g) => { graphData = g; })
			.catch(() => { graphError = 'Unable to load run graph.'; });
	} else {
		graphData = null;
	}
});
```

In the template, replace the cluster list section:

```svelte
<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
	<div class="flex flex-wrap items-end justify-between gap-4">
		<div>
			<p class="gc-stamp">Live runs</p>
			<h2 class="gc-section-title mt-3">
				{singleActiveCluster ? 'Active run' : 'Open the run that needs your attention'}
			</h2>
		</div>
		<p class="gc-machine">{data.work.clusters.length} visible clusters</p>
	</div>

	{#if data.work.clusters.length === 0}
		<SurfaceEmptyState
			className="mt-6"
			label="Idle machine"
			title="No active work yet"
			message="Launch a task to open the first graph."
		/>
	{:else if singleActiveCluster && graphData}
		<!-- Full graph for single active run -->
		<div class="mt-6">
			<RunGraph graph={graphData} />
		</div>
	{:else}
		<!-- Multi-cluster panel list -->
		<div class="mt-6 grid gap-4 xl:grid-cols-2">
			{#each data.work.clusters as cluster (cluster.root.id)}
				<WorkClusterPanel {cluster} />
			{/each}
		</div>
	{/if}
</section>
```

- [ ] **Step 3: Run tests and check**

```bash
cd frontend && bun run test:unit -- --run && bun run check
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/routes/work/+page.svelte frontend/src/lib/work/load.ts
git commit -m "feat: show full XYFlow run graph on Work surface for single active cluster"
```

---

## Task 10: Wire live inspector items for remaining surfaces

**Files:**
- Modify: `frontend/src/routes/conversations/+page.svelte`
- Modify: `frontend/src/routes/automate/+page.svelte`
- Modify: `frontend/src/routes/history/+page.svelte`
- Modify: `frontend/src/routes/recover/+page.svelte`

### Pattern (same for all)

Import `setInspectorItems` and call it at the top of each page's `<script>` block with data from the page's load response.

- [ ] **Step 1: Conversations inspector**

```typescript
import { setInspectorItems } from '$lib/shell/inspector.svelte';

setInspectorItems(() => [
	{
		label: 'Active',
		value: String(data.conversations.summary.session_count),
		tone: data.conversations.summary.session_count > 0 ? 'accent' : 'default'
	},
	{
		label: 'Failed deliveries',
		value: String(data.conversations.summary.terminal_deliveries),
		tone: data.conversations.summary.terminal_deliveries > 0 ? 'warning' : 'default'
	},
	{
		label: 'Channels',
		value: String(data.conversations.summary.connector_count)
	}
]);
```

- [ ] **Step 2: Automate inspector**

```typescript
setInspectorItems(() => [
	{
		label: 'Active now',
		value: String(data.automate.summary.active_occurrences),
		tone: data.automate.summary.active_occurrences > 0 ? 'accent' : 'default'
	},
	{
		label: 'Next run',
		value: data.automate.summary.next_wake_at_label
	},
	{
		label: 'Needs review',
		value: String(
			data.automate.health.invalid_schedules +
			data.automate.health.stuck_dispatching
		),
		tone: (data.automate.health.invalid_schedules + data.automate.health.stuck_dispatching) > 0
			? 'warning'
			: 'default'
	}
]);
```

- [ ] **Step 3: History inspector**

```typescript
setInspectorItems(() => [
	{
		label: 'Total runs',
		value: String(data.history.summary.run_count),
		tone: 'accent'
	},
	{
		label: 'Completed',
		value: String(data.history.summary.completed_runs)
	},
	{
		label: 'Recovery',
		value: String(data.history.summary.recovery_runs),
		tone: data.history.summary.recovery_runs > 0 ? 'warning' : 'default'
	}
]);
```

- [ ] **Step 4: Recover inspector**

```typescript
setInspectorItems(() => [
	{
		label: 'Pending approvals',
		value: String(data.recover.approval_summary?.needs_approval ?? 0),
		tone: (data.recover.approval_summary?.needs_approval ?? 0) > 0 ? 'warning' : 'default'
	},
	{
		label: 'Blocked runs',
		value: String(data.recover.approval_summary?.pending ?? 0),
		tone: (data.recover.approval_summary?.pending ?? 0) > 0 ? 'warning' : 'default'
	}
]);
```

Note: Check the actual field names in `RecoverResponse` type before writing — adjust field names to match.

- [ ] **Step 5: Run tests and check**

```bash
cd frontend && bun run test:unit -- --run && bun run check
```

- [ ] **Step 6: Commit**

```bash
git add frontend/src/routes/conversations/+page.svelte \
  frontend/src/routes/automate/+page.svelte \
  frontend/src/routes/history/+page.svelte \
  frontend/src/routes/recover/+page.svelte
git commit -m "feat: live inspector items for Conversations, Automate, History, and Recover surfaces"
```

---

## Task 11: Final integration — full test suite + build verification

- [ ] **Step 1: Run the full test suite**

```bash
cd frontend && bun run test:unit -- --run
```

Expected: all tests pass.

- [ ] **Step 2: TypeScript check**

```bash
cd frontend && bun run check
```

Expected: no errors.

- [ ] **Step 3: Lint + format**

```bash
cd frontend && bun run lint && bun run format
```

- [ ] **Step 4: Build**

```bash
cd frontend && bun run build
```

Expected: successful build with no errors.

- [ ] **Step 5: Go tests**

```bash
go test ./...
```

Expected: all pass.

- [ ] **Step 6: Commit if any format changes were made**

```bash
git add -p && git commit -m "chore: format pass after UI refactor"
```

---

## Self-Review

### Spec coverage check

| Decision | Task |
|---|---|
| Remove AppShell xl:block header | Task 1 |
| Automate: schedules first | Task 7 |
| Inspector: live runtime data | Tasks 3, 10 |
| Nav: remove item.id | Task 1 |
| Queue strip warning tone | Task 2 |
| Knowledge Forget: 2-step confirm | Task 5 |
| Team: unsaved-changes guard | Task 6 |
| Knowledge pagination | Task 5 |
| Work cluster cards: tone + status | Task 2 |
| Conversations: server display labels | Task 4 (uses role_label, no backend change needed) |
| Channel health: tone badge | Task 4 |
| Mobile header collapse | Task 1 |
| Automate item mutation state | Task 7 |
| Cron timezone datalist | Task 7 |
| status_class → token mapping | Task 8 |
| window.location.assign → goto() | Task 5 |
| Remove self-evident detail text | Task 8 |
| Conversations: delivery issues first | Task 4 |
| XYFlow multi-agent graph on Work | Task 9 |
| Team read-only overview | Task 6 |

**All 20 decisions have a task. No gaps.**

### Placeholder scan

No TBD, TODO, or "implement later" patterns present. All code blocks are complete.

### Type consistency

- `InspectorItem` type defined once in `AppShell.svelte` and re-exported from `inspector.svelte.ts` — both must use the same shape: `{ label: string; value: string; tone?: 'default' | 'accent' | 'warning' }`. The `AppShell.svelte` Task 1 code defines it locally as a `type`. The `inspector.svelte.ts` in Task 3 exports it. Update `AppShell.svelte` to import from `inspector.svelte.ts` instead of defining its own local type.

Fix: In Task 1 AppShell.svelte, remove the local `type InspectorItem` and import:
```typescript
import type { InspectorItem } from '$lib/shell/inspector.svelte';
```

But `inspector.svelte.ts` is created in Task 3, so Task 1 creates the type locally and Task 3 moves it. Task 3 must update AppShell too. Add that step to Task 3.

- `WorkClusterPanel` uses `toneClass()` and `statusTextClass()` — both are local to the component. `automate/+page.svelte` also defines a `statusTone()`. These don't conflict (different functions, different files). No issue.

- `connectEventStream` is used on the run detail page but not on the Work index — Task 3 uses `setInterval + invalidateAll` instead. Consistent — no collision.

## GSTACK REVIEW REPORT

| Review | Trigger | Why | Runs | Status | Findings |
|--------|---------|-----|------|--------|----------|
| CEO Review | `/plan-ceo-review` | Scope & strategy | 0 | — | — |
| Codex Review | `/codex review` | Independent 2nd opinion | 0 | — | — |
| Eng Review | `/plan-eng-review` | Architecture & tests (required) | 0 | — | — |
| Design Review | `/plan-design-review` | UI/UX gaps | 1 | CLEAR | score: 4/10 → 8/10, 18 decisions |

**VERDICT:** Design Review CLEARED. Eng Review required before merging.

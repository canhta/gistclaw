<script lang="ts">
	import SurfaceLoadErrorPanel from '$lib/components/common/SurfaceLoadErrorPanel.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SurfaceStatusCard from '$lib/components/skills/SurfaceStatusCard.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'installed' | 'available' | 'credentials';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'installed', label: 'Installed' },
		{ id: 'available', label: 'Available' },
		{ id: 'credentials', label: 'Credentials' }
	];

	let activeTabOverride = $state<TabID | null>(null);

	function isTabID(value: string | null): value is TabID {
		return value === 'installed' || value === 'available' || value === 'credentials';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'installed';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const projectName = $derived(data.project?.active_name ?? 'No project');
	const projectPath = $derived(data.project?.active_path ?? 'No active project path');
	const skills = $derived(data.skills);
	const configuredSurfaces = $derived(
		skills ? skills.surfaces.filter((surface) => surface.configured || surface.active) : []
	);
	const credentialSurfaces = $derived(
		skills
			? skills.surfaces.filter((surface) => surface.credential_state !== 'operator_managed')
			: []
	);
	const repoTools = $derived(
		skills ? skills.tools.filter((tool) => tool.family === 'repo').length : 0
	);
	const connectorTools = $derived(
		skills ? skills.tools.filter((tool) => tool.family === 'connector').length : 0
	);
	const skillsNotice = $derived(data.skillsLoadError || skills?.notice || '');
</script>

<svelte:head>
	<title>Skills | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Skills</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		{#if skills === null}
			{#if skillsNotice !== ''}
				<SurfaceMessage label="SKILLS" message={skillsNotice} className="mb-4" />
			{/if}
			<div class="mt-2">
				<SurfaceLoadErrorPanel
					label="SKILLS"
					title="Skills board unavailable"
					detail="The browser could not load the extension status feed from this daemon. Reload to retry."
				/>
			</div>
		{:else}
			{#if skillsNotice !== ''}
				<SurfaceMessage label="SKILLS" message={skillsNotice} className="mb-4" />
			{/if}

			<div class="grid gap-4 xl:grid-cols-4">
				<SurfaceMetricCard
					label="Shipped Surfaces"
					value={String(skills.summary.shipped_surfaces)}
					detail={`${skills.summary.configured_surfaces} configured in this runtime.`}
					tone="accent"
				/>
				<SurfaceMetricCard
					label="Installed Tools"
					value={String(skills.summary.installed_tools)}
					detail={`${repoTools} repo · ${connectorTools} connector`}
				/>
				<SurfaceMetricCard
					label="Credentials Ready"
					value={String(skills.summary.ready_credentials)}
					detail={`${skills.summary.missing_credentials} need setup`}
					tone="warning"
				/>
				<SurfaceMetricCard label="Project" value={projectName} detail={projectPath} tone="accent" />
			</div>

			{#if activeTab === 'installed'}
				<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">INSTALLED</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Configured extension inventory</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							This board reflects the runtime that is actually booted for the active machine:
							configured provider posture, live connector surfaces, and the registered tool seam.
						</p>

						<div class="mt-6 grid gap-4 lg:grid-cols-2">
							<section class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Configured surfaces</p>
								{#if configuredSurfaces.length === 0}
									<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
										No configured provider, connector, research, or MCP surface found.
									</p>
								{:else}
									<div class="mt-4 grid gap-4">
										{#each configuredSurfaces as surface (surface.id)}
											<SurfaceStatusCard {surface} />
										{/each}
									</div>
								{/if}
							</section>

							<section class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Tool registry</p>
								{#if skills.tools.length === 0}
									<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">No registered tools found.</p>
								{:else}
									<div class="mt-4 grid gap-4">
										{#each skills.tools as tool (tool.name)}
											<div
												class="border-t border-[var(--gc-border)] pt-4 first:border-t-0 first:pt-0"
											>
												<div class="flex items-center justify-between gap-3">
													<p class="gc-machine text-[var(--gc-ink)]">{tool.name}</p>
													<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-2)]">
														{tool.family}
													</span>
												</div>
												<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">{tool.description}</p>
												<p class="gc-machine mt-2 text-[var(--gc-ink-3)]">
													risk {tool.risk}
													{#if tool.approval !== ''}
														· approval {tool.approval}{/if}
													{#if tool.side_effect !== ''}
														· {tool.side_effect}{/if}
												</p>
											</div>
										{/each}
									</div>
								{/if}
							</section>
						</div>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Operator note</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Keep installs explicit</h2>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							This page reports the runtime seam that already exists. It does not invent a browser
							marketplace or mutate install state outside the shipped config and connector auth
							flows.
						</p>
					</section>
				</div>
			{:else if activeTab === 'available'}
				<div class="mt-6 grid gap-4 lg:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)]">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">AVAILABLE</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Available in this build</h2>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							These are the shipped extension surfaces the current runtime knows about. They can be
							available without being configured, active, or credential-ready in this machine.
						</p>

						{#if skills.surfaces.length === 0}
							<p class="gc-copy mt-4 text-[var(--gc-ink-2)]">No shipped surfaces reported.</p>
						{:else}
							<div class="mt-5 grid gap-4 lg:grid-cols-2">
								{#each skills.surfaces as surface (surface.id)}
									<SurfaceStatusCard {surface} />
								{/each}
							</div>
						{/if}
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Activation rules</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
							Availability is not installation
						</h2>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							Provider and connector posture still comes from config, stored auth state, and runtime
							bootstrap. MCP surfaces stay operator-managed through explicit server configuration.
						</p>
					</section>
				</div>
			{:else}
				<div class="mt-6 grid gap-4 lg:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)]">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">CREDENTIALS</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Credential readiness</h2>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							Secret-bearing surfaces report readiness here without exposing the secret value
							itself. Missing state tells you setup is still required before that surface can
							activate cleanly.
						</p>

						{#if credentialSurfaces.length === 0}
							<p class="gc-copy mt-4 text-[var(--gc-ink-2)]">
								No credential-bearing surfaces reported.
							</p>
						{:else}
							<div class="mt-5 grid gap-4 lg:grid-cols-2">
								{#each credentialSurfaces as surface (surface.id)}
									<SurfaceStatusCard {surface} />
								{/each}
							</div>
						{/if}
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Boundary</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Secrets stay operator-owned</h2>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							The browser can report readiness, but secret rotation still belongs to config edits,
							connector auth flows, and deployment policy. This page stays read-only on purpose.
						</p>
					</section>
				</div>
			{/if}
		{/if}
	</div>
</div>

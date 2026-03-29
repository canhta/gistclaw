<script lang="ts">
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import OperatorCommandCard from '$lib/components/update/OperatorCommandCard.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'run-update' | 'restart-report';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'run-update', label: 'Run Update' },
		{ id: 'restart-report', label: 'Restart Report' }
	];

	let activeTabOverride = $state<TabID | null>(null);

	function isTabID(value: string | null): value is TabID {
		return value === 'run-update' || value === 'restart-report';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'run-update';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const projectName = $derived(data.project?.active_name ?? 'No project');
	const projectPath = $derived(data.project?.active_path ?? 'No active project path');
	const commitLabel = $derived.by(() =>
		data.update.release.commit === 'unknown' ? 'unknown' : data.update.release.commit.slice(0, 12)
	);
	const warningCount = $derived(data.update.storage.warnings.length);
	const backupPath = $derived(
		data.update.storage.latest_backup_path === ''
			? 'No backup path recorded.'
			: data.update.storage.latest_backup_path
	);
	const updateNotice = $derived(data.update.notice ?? '');
	const runUpdateCommands = $derived(data.update.commands.run_update ?? []);
	const restartReportCommands = $derived(data.update.commands.restart_report ?? []);
</script>

<svelte:head>
	<title>Update | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Update</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		{#if updateNotice !== ''}
			<SurfaceMessage label="UPDATE" message={updateNotice} className="mb-4" />
		{/if}

		<div class="grid gap-4 xl:grid-cols-4">
			<SurfaceMetricCard
				label="Release Version"
				value={data.update.release.version}
				detail={`Commit ${commitLabel} · built ${data.update.release.build_date_label}`}
				tone="accent"
			/>
			<SurfaceMetricCard
				label="Runtime Uptime"
				value={data.update.runtime.uptime_label}
				detail={`Started ${data.update.runtime.started_at_label}`}
			/>
			<SurfaceMetricCard
				label="Backup Status"
				value={data.update.storage.backup_status}
				detail={`${warningCount} storage warning${warningCount === 1 ? '' : 's'}`}
				tone={warningCount > 0 ? 'warning' : 'default'}
			/>
			<SurfaceMetricCard label="Project" value={projectName} detail={projectPath} tone="accent" />
		</div>

		{#if activeTab === 'run-update'}
			<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.25fr)_minmax(18rem,0.85fr)]">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">RUN UPDATE</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Run the shipped update path</h2>
					<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
						GistClaw ships a release-driven maintenance flow today. This board exposes the exact
						build, install, and service context the runtime is using so you can review the release,
						apply it, and bring the daemon back under observation without leaving the control UI.
					</p>

					<div class="mt-5 flex flex-wrap gap-3">
						<a
							href={data.update.guides.release_notes_url}
							rel="external"
							class="gc-action gc-action-solid px-4 py-2"
						>
							GitHub Releases
						</a>
						<span class="gc-action px-4 py-2">{data.update.guides.changelog_path}</span>
					</div>

					<div class="mt-6 grid gap-4 md:grid-cols-2">
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Build</p>
							<div class="mt-3 space-y-3">
								<p class="gc-copy text-[var(--gc-ink)]">
									{data.update.release.version} · {commitLabel}
								</p>
								<p class="gc-machine">{data.update.release.build_date_label}</p>
							</div>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Restart policy</p>
							<div class="mt-3 space-y-3">
								<p class="gc-copy text-[var(--gc-ink)]">{data.update.service.restart_policy}</p>
								<p class="gc-machine">{data.update.install.service_unit_path}</p>
							</div>
						</div>
					</div>

					<div class="mt-6 grid gap-4 md:grid-cols-2">
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Install paths</p>
							<div class="mt-3 grid gap-3">
								<div>
									<p class="gc-stamp">Config</p>
									<p class="gc-machine mt-2 break-all">{data.update.install.config_path}</p>
								</div>
								<div>
									<p class="gc-stamp">State</p>
									<p class="gc-machine mt-2 break-all">{data.update.install.state_dir}</p>
								</div>
								<div>
									<p class="gc-stamp">Storage root</p>
									<p class="gc-machine mt-2 break-all">{data.update.install.storage_root}</p>
								</div>
							</div>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Operator docs</p>
							<div class="mt-3 grid gap-3">
								<div>
									<p class="gc-stamp">Ubuntu</p>
									<p class="gc-machine mt-2">{data.update.guides.ubuntu_doc_path}</p>
								</div>
								<div>
									<p class="gc-stamp">macOS</p>
									<p class="gc-machine mt-2">{data.update.guides.macos_doc_path}</p>
								</div>
								<div>
									<p class="gc-stamp">Recovery</p>
									<p class="gc-machine mt-2">{data.update.guides.recovery_doc_path}</p>
								</div>
							</div>
						</div>
					</div>

					<div class="mt-6 border border-[var(--gc-border)] px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Service unit preview</p>
						<pre
							class="gc-code mt-4 max-h-[18rem] overflow-auto border border-[var(--gc-border)] bg-[var(--gc-canvas)] px-4 py-4">{data
								.update.service.unit_preview}</pre>
					</div>
				</section>

				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">Operator commands</p>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						Use the current install paths to inspect the binary, confirm the active service unit,
						and restart the daemon without leaving the control deck.
					</p>

					{#if runUpdateCommands.length === 0}
						<div class="mt-5 border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-copy text-[var(--gc-ink-2)]">
								Operator commands will appear here when the daemon can report its install paths.
							</p>
						</div>
					{:else}
						<div class="mt-5 grid gap-4">
							{#each runUpdateCommands as command (command.id)}
								<OperatorCommandCard
									label={command.label}
									detail={command.detail}
									command={command.command}
								/>
							{/each}
						</div>
					{/if}
				</section>
			</div>
		{:else}
			<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(18rem,0.85fr)]">
				<section class="gc-panel-soft px-5 py-5">
					<p class="gc-stamp text-[var(--gc-ink-3)]">RESTART REPORT</p>
					<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Runtime boot report</h2>
					<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
						This report captures the currently running daemon after its last boot so operators can
						check restart timing, queue posture, and storage health from one place.
					</p>

					<div class="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Started at</p>
							<p class="gc-copy mt-3 text-[var(--gc-ink)]">
								{data.update.runtime.started_at_label}
							</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Uptime</p>
							<p class="gc-value mt-3 text-[1.2rem]">{data.update.runtime.uptime_label}</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Pending approvals</p>
							<p class="gc-value mt-3 text-[1.2rem]">{data.update.runtime.pending_approvals}</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Active runs</p>
							<p class="gc-value mt-3 text-[1.2rem]">{data.update.runtime.active_runs}</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Interrupted runs</p>
							<p class="gc-value mt-3 text-[1.2rem]">{data.update.runtime.interrupted_runs}</p>
						</div>
						<div class="border border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Backup status</p>
							<p class="gc-copy mt-3 text-[var(--gc-ink)]">{data.update.storage.backup_status}</p>
						</div>
					</div>
				</section>

				<div class="grid gap-4">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Verification commands</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							Run these checks after the restart to confirm the daemon, journal, and storage are
							back in a healthy state.
						</p>

						{#if restartReportCommands.length === 0}
							<div class="mt-5 border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-copy text-[var(--gc-ink-2)]">
									Verification commands will appear here when the daemon can report its runtime and
									storage paths.
								</p>
							</div>
						{:else}
							<div class="mt-5 grid gap-4">
								{#each restartReportCommands as command (command.id)}
									<OperatorCommandCard
										label={command.label}
										detail={command.detail}
										command={command.command}
									/>
								{/each}
							</div>
						{/if}
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Recovery evidence</p>
						<div class="mt-4 space-y-4">
							<div>
								<p class="gc-stamp">Latest backup</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">{backupPath}</p>
								{#if data.update.storage.latest_backup_at_label !== ''}
									<p class="gc-machine mt-2">{data.update.storage.latest_backup_at_label}</p>
								{/if}
							</div>
							<div class="border-t border-[var(--gc-border)] pt-4">
								<p class="gc-stamp">Storage warnings</p>
								{#if data.update.storage.warnings.length === 0}
									<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">No storage warnings recorded.</p>
								{:else}
									<div class="mt-3 grid gap-2">
										{#each data.update.storage.warnings as warning (warning)}
											<p class="gc-machine">{warning}</p>
										{/each}
									</div>
								{/if}
							</div>
							<div class="border-t border-[var(--gc-border)] pt-4">
								<p class="gc-stamp">Database footprint</p>
								<p class="gc-machine mt-2">
									db={data.update.storage.database_bytes} wal={data.update.storage.wal_bytes}
									free={data.update.storage.free_disk_bytes}
								</p>
							</div>
						</div>
					</section>
				</div>
			</div>
		{/if}
	</div>
</div>

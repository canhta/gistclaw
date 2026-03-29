<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { buildAutomateCreateRequest, defaultAutomateEditorState } from '$lib/automate/editor';
	import { createAutomateSchedule } from '$lib/automate/load';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import CronEditorPanel from '$lib/components/cron/CronEditorPanel.svelte';
	import OccurrenceRow from '$lib/components/cron/OccurrenceRow.svelte';
	import ScheduleRow from '$lib/components/cron/ScheduleRow.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import type { AutomateEditorErrors } from '$lib/automate/editor';
	import type { PageData } from './$types';

	type TabID = 'jobs' | 'runs' | 'editor';

	let { data }: { data: PageData } = $props();

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'jobs', label: 'Jobs' },
		{ id: 'runs', label: 'Runs' },
		{ id: 'editor', label: 'Editor' }
	];

	let activeTabOverride = $state<TabID | null>(null);
	let editorState = $state(defaultAutomateEditorState());
	let editorErrors = $state<AutomateEditorErrors>({});
	let actionError = $state('');
	let actionMessage = $state('');
	let submitting = $state(false);

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'jobs';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const summary = $derived(data.cron.summary);
	const health = $derived(data.cron.health);
	const schedules = $derived(data.cron.schedules ?? []);
	const openOccurrences = $derived(data.cron.openOccurrences ?? []);
	const recentOccurrences = $derived(data.cron.recentOccurrences ?? []);
	const healthMessage = $derived.by(() => {
		const messages: string[] = [];
		if ((health.invalid_schedules ?? 0) > 0) {
			messages.push(
				`${health.invalid_schedules} invalid schedule${health.invalid_schedules === 1 ? '' : 's'}`
			);
		}
		if ((health.stuck_dispatching ?? 0) > 0) {
			messages.push(
				`${health.stuck_dispatching} stuck dispatch${health.stuck_dispatching === 1 ? '' : 'es'}`
			);
		}
		if ((health.missing_next_run ?? 0) > 0) {
			messages.push(
				`${health.missing_next_run} missing next wake${health.missing_next_run === 1 ? '' : 's'}`
			);
		}
		if (messages.length === 0) {
			return 'No invalid schedules, missing wakes, or stuck dispatchers.';
		}
		return messages.join(' • ');
	});
	const healthTone = $derived(
		(health.invalid_schedules ?? 0) > 0 ||
			(health.stuck_dispatching ?? 0) > 0 ||
			(health.missing_next_run ?? 0) > 0
			? 'error'
			: 'accent'
	);

	function isTabID(value: string | null): value is TabID {
		return value === 'jobs' || value === 'runs' || value === 'editor';
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	function clearMessages(): void {
		actionError = '';
		actionMessage = '';
	}

	async function refreshData(): Promise<void> {
		await invalidateAll();
	}

	async function postAction(path: string, successMessage: string): Promise<void> {
		clearMessages();
		try {
			await requestJSON<Record<string, unknown>>(globalThis.fetch.bind(globalThis), path, {
				method: 'POST'
			});
			actionMessage = successMessage;
			await refreshData();
		} catch (error) {
			actionError =
				error instanceof HTTPError ? error.message : 'Unable to update the scheduler right now.';
		}
	}

	async function submitCreate(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		clearMessages();

		const built = buildAutomateCreateRequest(editorState);
		if (!built.ok) {
			editorErrors = built.errors;
			return;
		}

		editorErrors = {};
		submitting = true;

		try {
			const schedule = await createAutomateSchedule(
				globalThis.fetch.bind(globalThis),
				built.request
			);
			actionMessage = `Created ${schedule.name}.`;
			editorState = defaultAutomateEditorState();
			activeTabOverride = 'jobs';
			await refreshData();
		} catch (error) {
			actionError =
				error instanceof HTTPError ? error.message : 'Unable to create the cron job right now.';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Cron Jobs | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Cron Jobs</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 py-6">
		<div class="grid gap-4 xl:grid-cols-4">
			<SurfaceMetricCard
				label="Scheduled"
				value={String(summary.total_schedules ?? 0)}
				detail="Jobs registered with the local scheduler."
			/>
			<SurfaceMetricCard
				label="Enabled"
				value={String(summary.enabled_schedules ?? 0)}
				detail="Schedules that can claim new work."
				tone="accent"
			/>
			<SurfaceMetricCard
				label="Due Now"
				value={String(summary.due_schedules ?? 0)}
				detail="Schedules waiting for the next dispatch loop."
			/>
			<SurfaceMetricCard
				label="Next Wake"
				value={summary.next_wake_at_label ?? 'No wake scheduled'}
				detail="Next scheduler wake observed by the runtime."
				tone="warning"
			/>
		</div>

		<SurfaceMessage
			label="Scheduler health"
			message={healthMessage}
			tone={healthTone}
			className="mt-5"
		/>

		{#if actionMessage}
			<SurfaceMessage label="Scheduler update" message={actionMessage} className="mt-5" />
		{/if}

		{#if actionError}
			<SurfaceMessage label="Scheduler error" message={actionError} tone="error" className="mt-5" />
		{/if}

		<div class="mt-5 min-h-0 flex-1">
			{#if activeTab === 'jobs'}
				{#if schedules.length === 0}
					<div class="flex min-h-[24rem] items-center justify-center p-10">
						<div class="text-center">
							<p class="gc-stamp text-[var(--gc-ink-3)]">CRON JOBS</p>
							<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No jobs scheduled</p>
							<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
								Create a job to run tasks automatically.
							</p>
						</div>
					</div>
				{:else}
					<div class="gc-panel-soft overflow-hidden">
						<div class="overflow-auto">
							<table class="w-full border-collapse">
								<thead class="sticky top-0 bg-[var(--gc-surface)]">
									<tr class="border-b border-b-[1.5px] border-[var(--gc-border-strong)]">
										<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Job</th>
										<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Schedule</th>
										<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Enabled</th>
										<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Status</th>
										<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Next Run</th>
										<th class="gc-stamp px-4 py-3 text-right text-[var(--gc-ink-3)]"> Actions </th>
									</tr>
								</thead>
								<tbody>
									{#each schedules as schedule (schedule.id)}
										<ScheduleRow
											{schedule}
											onEnable={(id) =>
												void postAction(`/api/automate/${id}/enable`, `Enabled ${schedule.name}.`)}
											onDisable={(id) =>
												void postAction(`/api/automate/${id}/disable`, `Paused ${schedule.name}.`)}
											onRunNow={(id) =>
												void postAction(`/api/automate/${id}/run`, `Queued ${schedule.name}.`)}
										/>
									{/each}
								</tbody>
							</table>
						</div>
					</div>
				{/if}
			{:else if activeTab === 'runs'}
				<div class="grid gap-5 xl:grid-cols-2">
					<section class="gc-panel-soft overflow-hidden">
						<div class="border-b border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Open runs</p>
							<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
								Currently dispatching, active, or waiting for approval.
							</p>
						</div>

						{#if openOccurrences.length === 0}
							<div class="px-4 py-6">
								<p class="gc-copy text-[var(--gc-ink-2)]">No open scheduler runs.</p>
							</div>
						{:else}
							<div class="overflow-auto">
								<table class="w-full border-collapse">
									<tbody>
										{#each openOccurrences as occurrence (occurrence.id)}
											<OccurrenceRow {occurrence} />
										{/each}
									</tbody>
								</table>
							</div>
						{/if}
					</section>

					<section class="gc-panel-soft overflow-hidden">
						<div class="border-b border-[var(--gc-border)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-ink-3)]">Recent runs</p>
							<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
								Recently completed, interrupted, failed, or skipped jobs.
							</p>
						</div>

						{#if recentOccurrences.length === 0}
							<div class="px-4 py-6">
								<p class="gc-copy text-[var(--gc-ink-2)]">No recent scheduler history yet.</p>
							</div>
						{:else}
							<div class="overflow-auto">
								<table class="w-full border-collapse">
									<tbody>
										{#each recentOccurrences as occurrence (occurrence.id)}
											<OccurrenceRow {occurrence} />
										{/each}
									</tbody>
								</table>
							</div>
						{/if}
					</section>
				</div>
			{:else}
				<CronEditorPanel
					bind:state={editorState}
					errors={editorErrors}
					{submitting}
					onsubmit={submitCreate}
				/>
			{/if}
		</div>
	</div>
</div>

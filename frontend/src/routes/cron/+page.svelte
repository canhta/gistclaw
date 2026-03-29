<script lang="ts">
	import { requestJSON } from '$lib/http/client';
	import OccurrenceRow from '$lib/components/cron/OccurrenceRow.svelte';
	import ScheduleRow from '$lib/components/cron/ScheduleRow.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = [
		{ id: 'jobs', label: 'Jobs' },
		{ id: 'runs', label: 'Runs' },
		{ id: 'editor', label: 'Editor' }
	];

	let activeTab = $state('jobs');

	const schedules = $derived(data.cron?.schedules ?? []);
	const occurrences = $derived(data.cron?.occurrences ?? []);

	async function postAction(path: string): Promise<void> {
		await requestJSON(globalThis.fetch.bind(globalThis), path, { method: 'POST' });
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
		<SectionTabs {tabs} bind:activeTab />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-hidden">
		{#if activeTab === 'jobs'}
			{#if schedules.length === 0}
				<div class="flex flex-1 items-center justify-center p-10">
					<div class="text-center">
						<p class="gc-stamp text-[var(--gc-ink-3)]">CRON JOBS</p>
						<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No jobs scheduled</p>
						<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
							Create a job to run tasks automatically.
						</p>
					</div>
				</div>
			{:else}
				<div class="min-h-0 flex-1 overflow-auto">
					<table class="w-full border-collapse">
						<thead class="sticky top-0 bg-[var(--gc-surface)]">
							<tr class="border-b border-b-[1.5px] border-[var(--gc-border-strong)]">
								<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Job</th>
								<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Schedule</th>
								<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Enabled</th>
								<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Status</th>
								<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Next Run</th>
								<th class="gc-stamp px-4 py-3 text-right text-[var(--gc-ink-3)]">Actions</th>
							</tr>
						</thead>
						<tbody>
							{#each schedules as schedule (schedule.id)}
								<ScheduleRow
									{schedule}
									onEnable={(id) => void postAction(`/api/automate/${id}/enable`)}
									onDisable={(id) => void postAction(`/api/automate/${id}/disable`)}
									onRunNow={(id) => void postAction(`/api/automate/${id}/run`)}
								/>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		{:else if activeTab === 'runs'}
			{#if occurrences.length === 0}
				<div class="flex flex-1 items-center justify-center p-10">
					<div class="text-center">
						<p class="gc-stamp text-[var(--gc-ink-3)]">RUNS</p>
						<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">No run history</p>
						<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
							Run history will appear here after a cron job executes.
						</p>
					</div>
				</div>
			{:else}
				<div class="min-h-0 flex-1 overflow-auto">
					<table class="w-full border-collapse">
						<thead class="sticky top-0 bg-[var(--gc-surface)]">
							<tr class="border-b border-b-[1.5px] border-[var(--gc-border-strong)]">
								<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Job</th>
								<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Status</th>
								<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Slot</th>
								<th class="gc-stamp px-4 py-3 text-left text-[var(--gc-ink-3)]">Updated</th>
							</tr>
						</thead>
						<tbody>
							{#each occurrences as occurrence (occurrence.id)}
								<OccurrenceRow {occurrence} />
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		{:else}
			<div class="min-h-0 flex-1 overflow-auto px-6 py-6">
				<div class="grid gap-5 lg:grid-cols-2">
					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Identity</p>
						<label class="mt-3 flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Name</span>
							<input class="gc-control min-h-[2.5rem]" placeholder="Daily digest" />
						</label>
					</section>

					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Schedule</p>
						<label class="mt-3 flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Cadence</span>
							<input class="gc-control min-h-[2.5rem]" placeholder="Every day at 09:00" />
						</label>
					</section>

					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Execution</p>
						<label class="mt-3 flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Working directory</span>
							<input class="gc-control min-h-[2.5rem]" placeholder="/home/user/project" />
						</label>
					</section>

					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Payload</p>
						<label class="mt-3 flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Task prompt</span>
							<textarea class="gc-control min-h-[8rem] resize-y" placeholder="Send a daily summary"
							></textarea>
						</label>
					</section>

					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Delivery</p>
						<label class="mt-3 flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Channel</span>
							<input class="gc-control min-h-[2.5rem]" placeholder="last active channel" />
						</label>
					</section>

					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Advanced</p>
						<label class="mt-3 flex flex-col gap-2">
							<span class="gc-copy text-[var(--gc-ink-2)]">Timeout</span>
							<input class="gc-control min-h-[2.5rem]" placeholder="300" />
						</label>
					</section>
				</div>

				<div class="mt-5 flex justify-end">
					<button
						type="button"
						class="gc-badge border-[var(--gc-primary)] text-[var(--gc-primary)]"
					>
						Create Job
					</button>
				</div>
			</div>
		{/if}
	</div>
</div>

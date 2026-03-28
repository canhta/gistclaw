<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import SurfaceActionButton from '$lib/components/common/SurfaceActionButton.svelte';
	import SurfaceEmptyState from '$lib/components/common/SurfaceEmptyState.svelte';
	import WorkClusterPanel from '$lib/components/common/WorkClusterPanel.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import type { WorkCreateResponse } from '$lib/types/api';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let task = $state('');
	let errorMessage = $state('');
	let submitting = $state(false);


	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		errorMessage = '';
		submitting = true;

		try {
			const result = await requestJSON<WorkCreateResponse>(fetch, '/api/work', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ task })
			});
			task = '';
			await goto(resolve('/work/[runId]', { runId: result.run_id }), {
				invalidateAll: true
			});
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to start the work right now.';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Work | GistClaw</title>
</svelte:head>

<div class="grid gap-6">
	<section class="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)]">
		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<p class="gc-stamp">Command intake</p>
			<h2 class="gc-section-title mt-3">Describe the work you want GistClaw to handle next</h2>
			<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">
				Describe the task in plain language. GistClaw will start it and open the run so you can keep
				it moving.
			</p>

			<form class="mt-6 grid gap-4" onsubmit={submit}>
				<label class="grid gap-2">
					<span class="gc-stamp">Task</span>
					<textarea
						bind:value={task}
						name="task"
						rows="5"
						required
						class="gc-control"
						placeholder="Audit the repository, identify the highest-risk issues, and draft the first safe patch."
					></textarea>
				</label>

				{#if errorMessage}
					<div class="gc-panel-soft border-[var(--gc-error)] px-4 py-4">
						<p class="gc-stamp text-[var(--gc-error)]">Launch error</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink)]">{errorMessage}</p>
					</div>
				{/if}

				<SurfaceActionButton type="submit" tone="solid" disabled={submitting}>
					{submitting ? 'Launching work' : 'Launch work'}
				</SurfaceActionButton>
			</form>
		</div>

		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<p class="gc-stamp">Queue strip</p>
			<h2 class="gc-section-title mt-3">{data.work.queue_strip.headline}</h2>
			<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">
				{data.work.active_project_name} · {data.work.active_project_path}
			</p>

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
		</div>
	</section>

	<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
		<div class="flex flex-wrap items-end justify-between gap-4">
			<div>
				<p class="gc-stamp">Live runs</p>
				<h2 class="gc-section-title mt-3">Open the run that needs your attention</h2>
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
		{:else}
			<div class="mt-6 grid gap-4 xl:grid-cols-2">
				{#each data.work.clusters as cluster (cluster.root.id)}
					<WorkClusterPanel {cluster} />
				{/each}
			</div>
		{/if}
	</section>
</div>

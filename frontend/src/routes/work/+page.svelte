<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import RunClusterCard from '$lib/components/common/RunClusterCard.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import type { WorkCreateResponse } from '$lib/types/api';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let task = $state('');
	let errorMessage = $state('');
	let submitting = $state(false);

	const queueStats = $derived([
		{ label: 'Root runs', value: data.work.queue_strip.root_runs },
		{ label: 'Worker runs', value: data.work.queue_strip.worker_runs },
		{ label: 'Recovery', value: data.work.queue_strip.recovery_runs },
		{ label: 'Approvals', value: data.work.queue_strip.summary.needs_approval }
	]);

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
				Start from the operator’s job. Frame the task, let the runtime fan it out, then move
				straight into the live graph when the run is created.
			</p>

			<form class="mt-6 grid gap-4" onsubmit={submit}>
				<label class="grid gap-2">
					<span class="gc-stamp">Task</span>
					<textarea
						bind:value={task}
						name="task"
						rows="5"
						required
						class="border-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] px-4 py-3 text-[var(--gc-ink)] transition-colors outline-none focus:border-[var(--gc-orange)]"
						placeholder="Audit the repository, identify the highest-risk issues, and draft the first safe patch."
					></textarea>
				</label>

				{#if errorMessage}
					<div class="gc-panel-soft border-[var(--gc-error)] px-4 py-4">
						<p class="gc-stamp text-[var(--gc-error)]">Launch error</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink)]">{errorMessage}</p>
					</div>
				{/if}

				<button
					type="submit"
					class="border-2 border-[var(--gc-orange)] bg-[var(--gc-orange)] px-4 py-3 text-left text-sm font-[var(--gc-font-mono)] font-bold tracking-[0.18em] text-[var(--gc-canvas)] uppercase transition-colors hover:border-[var(--gc-orange-hover)] hover:bg-[var(--gc-orange-hover)] disabled:cursor-not-allowed disabled:opacity-60"
					disabled={submitting}
				>
					{submitting ? 'Launching work' : 'Launch work'}
				</button>
			</form>
		</div>

		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<p class="gc-stamp">Queue strip</p>
			<h2 class="gc-section-title mt-3">{data.work.queue_strip.headline}</h2>
			<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">
				{data.work.active_project_name} · {data.work.active_project_path}
			</p>

			<div class="mt-6 grid gap-3 sm:grid-cols-2">
				{#each queueStats as stat (stat.label)}
					<div class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp">{stat.label}</p>
						<p class="gc-value mt-3">{stat.value}</p>
					</div>
				{/each}
			</div>
		</div>
	</section>

	<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
		<div class="flex flex-wrap items-end justify-between gap-4">
			<div>
				<p class="gc-stamp">Live runs</p>
				<h2 class="gc-section-title mt-3">Open a run where the operator can actually intervene</h2>
			</div>
			<p class="gc-machine">{data.work.clusters.length} visible clusters</p>
		</div>

		{#if data.work.clusters.length === 0}
			<div class="gc-panel-soft mt-6 px-4 py-4">
				<p class="gc-stamp">Idle machine</p>
				<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
					No active work yet. Launch a task to open the first graph.
				</p>
			</div>
		{:else}
			<div class="mt-6 grid gap-4 xl:grid-cols-2">
				{#each data.work.clusters as cluster (cluster.root.id)}
					<RunClusterCard {cluster} />
				{/each}
			</div>
		{/if}
	</section>
</div>

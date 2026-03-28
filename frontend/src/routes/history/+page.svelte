<script lang="ts">
	import WorkClusterPanel from '$lib/components/common/WorkClusterPanel.svelte';
	import SurfaceEmptyState from '$lib/components/common/SurfaceEmptyState.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import { resolve } from '$app/paths';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const summaryCards = $derived([
		{
			label: 'Visible runs',
			value: String(data.history.summary.run_count),
			detail: 'Recent root runs visible from the evidence surface.',
			tone: 'accent' as const
		},
		{
			label: 'Completed runs',
			value: String(data.history.summary.completed_runs),
			detail: 'Runs that reached a clean terminal state.'
		},
		{
			label: 'Recovery cases',
			value: String(data.history.summary.recovery_runs),
			detail: 'Runs that failed, were interrupted, or paused for approval.',
			tone: 'warning' as const
		},
		{
			label: 'Operator evidence',
			value: String(data.history.summary.approval_events + data.history.summary.delivery_outcomes),
			detail: 'Approvals and delivery receipts that explain user-visible outcomes.'
		}
	]);
</script>

<svelte:head>
	<title>History | GistClaw</title>
</svelte:head>

<div class="grid gap-6">
	<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
		<p class="gc-stamp">History</p>
		<h2 class="gc-section-title mt-3">See what happened before you decide what to do next</h2>
		<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">
			Review finished runs, approvals, and delivery results so the next step starts from evidence.
		</p>

		<div class="mt-6 grid gap-4 xl:grid-cols-4">
			{#each summaryCards as card (card.label)}
				<SurfaceMetricCard
					label={card.label}
					value={card.value}
					detail={card.detail}
					tone={card.tone ?? 'default'}
				/>
			{/each}
		</div>
	</section>

	<section class="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<div class="flex items-end justify-between gap-4">
				<div>
					<p class="gc-stamp">Run history</p>
					<h2 class="gc-section-title mt-3">Open the run that best explains what happened</h2>
				</div>
				<p class="gc-machine">{data.history.runs.length} visible roots</p>
			</div>

			<div class="mt-6 grid gap-4">
				{#if data.history.runs.length === 0}
					<SurfaceEmptyState
						label="No run evidence yet"
						title="History is still empty"
						message="Finished and interrupted runs will show up here after the first task."
						actionHref={resolve('/work')}
						actionLabel="Launch first run"
					/>
				{:else}
					{#each data.history.runs as cluster (cluster.root.id)}
						<WorkClusterPanel {cluster} />
					{/each}
				{/if}
			</div>
		</div>

		<div class="grid gap-6">
			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Decisions</p>
				<div class="mt-4 grid gap-4">
					{#if data.history.approvals.length === 0}
						<SurfaceEmptyState
							label="No resolved approvals"
							title="No operator decisions are recorded yet"
							message="Approval receipts will appear here once a human decision changes a run path."
						/>
					{:else}
						{#each data.history.approvals as approval (approval.id)}
							<article class="gc-panel-soft px-4 py-4">
								<div class="flex items-start justify-between gap-4">
									<div>
										<p class="gc-stamp">{approval.status_label}</p>
										<h3 class="gc-panel-title mt-3 text-[1rem]">{approval.tool_name}</h3>
									</div>
									<p class="gc-machine">{approval.resolved_by}</p>
								</div>
								<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">
									Resolved at {approval.resolved_at_label}
								</p>
								<a
									class="gc-machine mt-4 inline-flex underline"
									href={resolve('/work/[runId]', { runId: approval.run_id })}
								>
									Open run {approval.run_id}
								</a>
							</article>
						{/each}
					{/if}
				</div>
			</div>

			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Delivery results</p>
				<div class="mt-4 grid gap-4">
					{#if data.history.deliveries.length === 0}
						<SurfaceEmptyState
							label="No delivery receipts"
							title="No edge evidence is recorded yet"
							message="Connector receipts will appear here once runs reach an external surface."
						/>
					{:else}
						{#each data.history.deliveries as delivery (delivery.id)}
							<article class="gc-panel-soft px-4 py-4">
								<div class="flex items-start justify-between gap-4">
									<div>
										<p class="gc-stamp">{delivery.connector_id}</p>
										<h3 class="gc-panel-title mt-3 text-[1rem]">{delivery.status_label}</h3>
									</div>
									<p class="gc-machine">{delivery.attempts_label}</p>
								</div>
								<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">
									{delivery.message_preview}
								</p>
								<p class="gc-machine mt-4">{delivery.last_attempt_at_label}</p>
								<a
									class="gc-machine mt-4 inline-flex underline"
									href={resolve('/work/[runId]', { runId: delivery.run_id })}
								>
									Open run {delivery.run_id}
								</a>
							</article>
						{/each}
					{/if}
				</div>
			</div>
		</div>
	</section>
</div>

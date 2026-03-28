<script lang="ts">
	import { resolve } from '$app/paths';
	import SurfaceEmptyState from '$lib/components/common/SurfaceEmptyState.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import type { RecoverDeliveryHealthResponse } from '$lib/types/api';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	function pressureLabel(item: RecoverDeliveryHealthResponse): string {
		return `${item.pending_count} pending · ${item.retrying_count} retrying · ${item.terminal_count} terminal`;
	}
</script>

<svelte:head>
	<title>Conversations | GistClaw</title>
</svelte:head>

<div class="grid gap-6">
	<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
		<p class="gc-stamp">Conversations</p>
		<h2 class="gc-section-title mt-3">See who is waiting on a reply</h2>
		<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">
			Check who is talking to GistClaw, which channels are healthy, and where replies need help.
		</p>

		<div class="mt-6 grid gap-4 xl:grid-cols-3">
			<SurfaceMetricCard
				label="Active conversations"
				value={String(data.conversations.summary.session_count)}
				detail="Conversations currently attached to this project."
				tone="accent"
			/>
			<SurfaceMetricCard
				label="Connected channels"
				value={String(data.conversations.summary.connector_count)}
				detail="Channels with current delivery or health signal."
			/>
			<SurfaceMetricCard
				label="Failed deliveries"
				value={String(data.conversations.summary.terminal_deliveries)}
				detail="Replies that now need recovery attention."
				tone="warning"
			/>
		</div>
	</section>

	<section class="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<div class="flex items-end justify-between gap-4">
				<div>
					<p class="gc-stamp">Active threads</p>
					<h2 class="gc-section-title mt-3">Open the conversation that needs attention</h2>
				</div>
				<p class="gc-machine">{data.conversations.sessions.length} visible sessions</p>
			</div>

			<div class="mt-6 grid gap-4">
				{#if data.conversations.sessions.length === 0}
					<SurfaceEmptyState
						label="No conversations yet"
						title="No active conversation is visible yet"
						message="Start work or connect a chat to see it here."
						actionHref={resolve('/work')}
						actionLabel="Open Work"
					/>
				{:else}
					{#each data.conversations.sessions as session (session.id)}
						<a
							class="gc-panel-soft block px-4 py-4 transition-colors hover:border-[var(--gc-cyan)]"
							href={resolve('/conversations/[sessionId]', { sessionId: session.id })}
						>
							<div class="flex items-start justify-between gap-4">
								<div>
									<p class="gc-stamp">{session.role_label}</p>
									<h3 class="gc-panel-title mt-3 text-[1rem]">{session.agent_id}</h3>
								</div>
								<p class="gc-machine">{session.status_label}</p>
							</div>
							<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">
								Thread {session.conversation_id}
							</p>
							<p class="gc-machine mt-4">{session.updated_at_label}</p>
						</a>
					{/each}
				{/if}
			</div>
		</div>

		<div class="grid gap-6">
			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Channel health</p>
				<div class="mt-4 grid gap-4">
					{#each data.conversations.runtime_connectors as item (item.connector_id)}
						<article class="gc-panel-soft px-4 py-4">
							<p class="gc-stamp">{item.connector_id}</p>
							<p class="gc-value mt-3">{item.state_label}</p>
							<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{item.summary}</p>
						</article>
					{/each}
				</div>
			</div>

			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Delivery issues</p>
				<div class="mt-4 grid gap-4">
					{#each data.conversations.health as item (item.connector_id)}
						<article class="gc-panel-soft px-4 py-4">
							<p class="gc-stamp">{item.connector_id}</p>
							<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{pressureLabel(item)}</p>
						</article>
					{/each}
				</div>
			</div>
		</div>
	</section>
</div>

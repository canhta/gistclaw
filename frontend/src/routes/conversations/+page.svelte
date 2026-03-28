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
		<p class="gc-stamp">External surfaces</p>
		<h2 class="gc-section-title mt-3">See who is connected, healthy, and slipping</h2>
		<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">
			Conversations keeps session ownership, connector state, and delivery pressure readable from
			the user point of view instead of a transport-first admin page.
		</p>

		<div class="mt-6 grid gap-4 xl:grid-cols-3">
			<SurfaceMetricCard
				label="Visible sessions"
				value={String(data.conversations.summary.session_count)}
				detail="Sessions currently attached to the active project."
				tone="accent"
			/>
			<SurfaceMetricCard
				label="Connector surfaces"
				value={String(data.conversations.summary.connector_count)}
				detail="Distinct external surfaces with current delivery or runtime signal."
			/>
			<SurfaceMetricCard
				label="Terminal deliveries"
				value={String(data.conversations.summary.terminal_deliveries)}
				detail="Outbound attempts that now need recovery attention."
				tone="warning"
			/>
		</div>
	</section>

	<section class="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<div class="flex items-end justify-between gap-4">
				<div>
					<p class="gc-stamp">Session mailbox</p>
					<h2 class="gc-section-title mt-3">Open the conversation that needs operator context</h2>
				</div>
				<p class="gc-machine">{data.conversations.sessions.length} visible sessions</p>
			</div>

			<div class="mt-6 grid gap-4">
				{#if data.conversations.sessions.length === 0}
					<SurfaceEmptyState
						label="No active conversations"
						title="No bound sessions are visible yet"
						message="No bound sessions are visible for this project yet."
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
								Conversation {session.conversation_id}
							</p>
							<p class="gc-machine mt-4">{session.updated_at_label}</p>
						</a>
					{/each}
				{/if}
			</div>
		</div>

		<div class="grid gap-6">
			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Connector health</p>
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
				<p class="gc-stamp">Delivery pressure</p>
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

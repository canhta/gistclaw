<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import SurfaceActionButton from '$lib/components/common/SurfaceActionButton.svelte';
	import SurfaceEmptyState from '$lib/components/common/SurfaceEmptyState.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let errorMessage = $state('');
	let actionLabel = $state('');

	async function resolveApproval(
		approvalID: string,
		decision: 'approved' | 'denied'
	): Promise<void> {
		actionLabel = `${decision}:${approvalID}`;
		errorMessage = '';

		try {
			await requestJSON(fetch, `/api/recover/approvals/${approvalID}/resolve`, {
				method: 'POST',
				headers: { 'content-type': 'application/x-www-form-urlencoded' },
				body: new URLSearchParams({ decision }).toString()
			});
			await invalidateAll();
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to resolve this approval.';
		} finally {
			actionLabel = '';
		}
	}

	async function deactivateRoute(routeID: string): Promise<void> {
		actionLabel = `route:${routeID}`;
		errorMessage = '';

		try {
			await requestJSON(fetch, `/api/recover/routes/${routeID}/deactivate`, {
				method: 'POST'
			});
			await invalidateAll();
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to deactivate this route.';
		} finally {
			actionLabel = '';
		}
	}

	async function retryDelivery(deliveryID: string): Promise<void> {
		actionLabel = `delivery:${deliveryID}`;
		errorMessage = '';

		try {
			await requestJSON(fetch, `/api/recover/deliveries/${deliveryID}/retry`, {
				method: 'POST'
			});
			await invalidateAll();
		} catch (error) {
			errorMessage = error instanceof HTTPError ? error.message : 'Unable to retry this delivery.';
		} finally {
			actionLabel = '';
		}
	}
</script>

<svelte:head>
	<title>Recover | GistClaw</title>
</svelte:head>

<div class="grid gap-6">
	<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
		<p class="gc-stamp">Recover</p>
		<h2 class="gc-section-title mt-3">Fix blocked work before it piles up</h2>
		<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">
			Approve requests, retry failed deliveries, and repair routes from one place.
		</p>

		{#if errorMessage}
			<SurfaceMessage label="Recovery error" message={errorMessage} tone="error" className="mt-5" />
		{/if}

		<div class="mt-6 grid gap-4 xl:grid-cols-4">
			<SurfaceMetricCard
				label="Open approvals"
				value={String(data.recover.summary.open_approvals)}
				detail="Anything pending or expired still needing a human decision."
				tone="warning"
			/>
			<SurfaceMetricCard
				label="Pending now"
				value={String(data.recover.summary.pending_approvals)}
				detail="Approvals you can clear immediately."
				tone="accent"
			/>
			<SurfaceMetricCard
				label="Active routes"
				value={String(data.recover.summary.active_routes)}
				detail="Live bindings still carrying messages."
			/>
			<SurfaceMetricCard
				label="Terminal deliveries"
				value={String(data.recover.summary.terminal_deliveries)}
				detail="Outbound attempts needing a retry or manual repair."
				tone="warning"
			/>
		</div>
	</section>

	<section class="grid gap-6 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<div class="flex items-end justify-between gap-4">
				<div>
					<p class="gc-stamp">Approval queue</p>
					<h2 class="gc-section-title mt-3">Clear pending decisions first</h2>
				</div>
				<p class="gc-machine">{data.recover.approvals.length} visible tickets</p>
			</div>

			<div class="mt-6 grid gap-4">
				{#if data.recover.approvals.length === 0}
					<SurfaceEmptyState
						label="No waiting approvals"
						title="The intervention bench is clear"
						message="Nothing is waiting for your approval right now."
					/>
				{:else}
					{#each data.recover.approvals as approval (approval.id)}
						<article class="gc-panel-soft px-4 py-4">
							<div class="flex items-start justify-between gap-4">
								<div>
									<p class="gc-stamp">{approval.tool_name} · {approval.status_label}</p>
									<h3 class="gc-panel-title mt-3 text-[1rem]">{approval.binding_summary}</h3>
								</div>
								<p class={`gc-machine ${approval.status_class}`}>{approval.run_id}</p>
							</div>

							<div class="mt-4 flex flex-wrap gap-3">
								<SurfaceActionButton
									type="button"
									onclick={() => resolveApproval(approval.id, 'approved')}
									disabled={actionLabel !== '' && actionLabel !== `approved:${approval.id}`}
								>
									Approve
								</SurfaceActionButton>
								<SurfaceActionButton
									type="button"
									tone="warning"
									onclick={() => resolveApproval(approval.id, 'denied')}
									disabled={actionLabel !== '' && actionLabel !== `denied:${approval.id}`}
								>
									Deny
								</SurfaceActionButton>
							</div>
						</article>
					{/each}
				{/if}
			</div>
		</div>

		<div class="grid gap-6">
			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<div class="flex items-end justify-between gap-4">
					<div>
						<p class="gc-stamp">Route and delivery health</p>
						<h2 class="gc-section-title mt-3">Fix delivery paths before messages stall</h2>
					</div>
					<p class="gc-machine">{data.recover.repair.connector_count} connectors</p>
				</div>

				<div class="mt-6 grid gap-4 xl:grid-cols-2">
					{#each data.recover.repair.health as item (item.connector_id)}
						<article class="gc-panel-soft px-4 py-4">
							<p class="gc-stamp">{item.connector_id}</p>
							<p class="gc-value mt-3">{item.terminal_count} terminal</p>
							<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
								{item.pending_count} pending · {item.retrying_count} retrying
							</p>
						</article>
					{/each}
					{#each data.recover.repair.runtime_connectors as item (item.connector_id)}
						<article class="gc-panel-soft px-4 py-4">
							<p class="gc-stamp">{item.connector_id} runtime</p>
							<p class="gc-value mt-3">{item.state_label}</p>
							<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{item.summary}</p>
						</article>
					{/each}
				</div>
			</div>

			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Live routes</p>
				<div class="mt-4 grid gap-4">
					{#each data.recover.repair.active_routes as route (route.id)}
						<article class="gc-panel-soft px-4 py-4">
							<div
								class="flex min-w-0 flex-col gap-3 sm:flex-row sm:items-start sm:justify-between"
							>
								<div class="min-w-0">
									<p class="gc-stamp">{route.connector_id} · {route.role_label}</p>
									<h3 class="gc-panel-title mt-3 text-[1rem]">{route.external_id}</h3>
								</div>
								<a
									class="gc-machine break-all underline sm:text-right"
									href={resolve('/conversations/[sessionId]', { sessionId: route.session_id })}
								>
									{route.session_id}
								</a>
							</div>
							<SurfaceActionButton
								type="button"
								tone="warning"
								className="mt-4"
								onclick={() => deactivateRoute(route.id)}
								disabled={actionLabel !== '' && actionLabel !== `route:${route.id}`}
							>
								Deactivate route
							</SurfaceActionButton>
						</article>
					{/each}
				</div>
			</div>

			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Failed deliveries</p>
				<div class="mt-4 grid gap-4">
					{#if data.recover.repair.deliveries.length === 0}
						<SurfaceEmptyState
							label="No queued repairs"
							title="Outbound delivery is currently clear"
							message="Nothing in the outbound queue needs recovery right now."
						/>
					{:else}
						{#each data.recover.repair.deliveries as delivery (delivery.id)}
							<article class="gc-panel-soft px-4 py-4">
								<div class="flex items-start justify-between gap-4">
									<div>
										<p class="gc-stamp">{delivery.connector_id} · {delivery.status_label}</p>
										<h3 class="gc-panel-title mt-3 text-[1rem]">{delivery.chat_id}</h3>
									</div>
									<p class="gc-machine">{delivery.attempts_label}</p>
								</div>
								<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">
									{delivery.message.plain_text}
								</p>
								<SurfaceActionButton
									type="button"
									className="mt-4"
									onclick={() => retryDelivery(delivery.id)}
									disabled={actionLabel !== '' && actionLabel !== `delivery:${delivery.id}`}
								>
									Retry delivery
								</SurfaceActionButton>
							</article>
						{/each}
					{/if}
				</div>
			</div>
		</div>
	</section>
</div>

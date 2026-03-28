<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import SurfaceActionButton from '$lib/components/common/SurfaceActionButton.svelte';
	import SurfaceEmptyState from '$lib/components/common/SurfaceEmptyState.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let noticeMessage = $state('');
	let errorMessage = $state('');
	let saving = $state(false);
	let mutatingID = $state('');
	let name = $state('');
	let objective = $state('');
	let kind = $state<'every' | 'at' | 'cron'>('every');
	let anchorAt = $state('');
	let everyHours = $state('2');
	let cronExpr = $state('0 9 * * *');
	let timezone = $state('UTC');

	const summaryCards = $derived([
		{
			label: 'Running now',
			value: String(data.automate.summary.active_occurrences),
			detail: 'Scheduled work already running right now.',
			tone: 'accent' as const
		},
		{
			label: 'Active schedules',
			value: String(data.automate.summary.enabled_schedules),
			detail: 'Recurring tasks still set to run again.'
		},
		{
			label: 'Next run',
			value: data.automate.summary.next_wake_at_label,
			detail: 'When the next scheduled task should start.'
		},
		{
			label: 'Needs review',
			value: String(
				data.automate.health.invalid_schedules +
					data.automate.health.stuck_dispatching +
					data.automate.health.missing_next_run
			),
			detail: 'Schedules that may miss their next run or need a fix.',
			tone: 'warning' as const
		}
	]);

	async function createSchedule(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		saving = true;
		errorMessage = '';
		noticeMessage = '';

		try {
			await requestJSON(fetch, '/api/automate', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({
					name,
					objective,
					kind,
					anchor_at: serializeAnchorAt(anchorAt),
					every_hours: Number.parseInt(everyHours, 10),
					cron_expr: cronExpr,
					timezone
				})
			});
			resetDraft();
			noticeMessage = 'Schedule created.';
			await invalidateAll();
		} catch (error) {
			errorMessage = error instanceof HTTPError ? error.message : 'Unable to create this schedule.';
		} finally {
			saving = false;
		}
	}

	async function toggleSchedule(scheduleID: string, enabled: boolean): Promise<void> {
		mutatingID = scheduleID;
		errorMessage = '';
		noticeMessage = '';

		try {
			await requestJSON(fetch, `/api/automate/${scheduleID}/${enabled ? 'disable' : 'enable'}`, {
				method: 'POST'
			});
			noticeMessage = enabled ? 'Schedule paused.' : 'Schedule resumed.';
			await invalidateAll();
		} catch (error) {
			errorMessage = error instanceof HTTPError ? error.message : 'Unable to update this schedule.';
		} finally {
			mutatingID = '';
		}
	}

	async function launchNow(scheduleID: string): Promise<void> {
		mutatingID = scheduleID;
		errorMessage = '';
		noticeMessage = '';

		try {
			const response = await requestJSON<{ occurrence: { run_id?: string } }>(
				fetch,
				`/api/automate/${scheduleID}/run`,
				{
					method: 'POST'
				}
			);
			if (response.occurrence.run_id) {
				await goto(resolve('/work/[runId]', { runId: response.occurrence.run_id }), {
					invalidateAll: true
				});
				return;
			}
			noticeMessage = 'Schedule started.';
			await invalidateAll();
		} catch (error) {
			errorMessage = error instanceof HTTPError ? error.message : 'Unable to start this schedule.';
		} finally {
			mutatingID = '';
		}
	}

	function resetDraft(): void {
		name = '';
		objective = '';
		kind = 'every';
		anchorAt = '';
		everyHours = '2';
		cronExpr = '0 9 * * *';
		timezone = 'UTC';
	}

	function serializeAnchorAt(raw: string): string {
		if (raw.trim() === '') {
			return '';
		}
		return new Date(raw).toISOString();
	}

	function statusTone(statusClass: string): 'accent' | 'warning' {
		return statusClass === 'is-error' || statusClass === 'is-approval' ? 'warning' : 'accent';
	}
</script>

<svelte:head>
	<title>Automate | GistClaw</title>
</svelte:head>

<div class="grid gap-6">
	<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
		<p class="gc-stamp">Recurring work</p>
		<h2 class="gc-section-title mt-3">Keep recurring work moving without checking in manually</h2>
		<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">
			Set up repeat work, see what runs next, and fix schedules before they slip.
		</p>

		{#if noticeMessage}
			<SurfaceMessage label="Schedule notice" message={noticeMessage} className="mt-5" />
		{/if}

		{#if errorMessage}
			<SurfaceMessage label="Schedule error" message={errorMessage} tone="error" className="mt-5" />
		{/if}

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
		<div class="grid gap-6">
			<form class="gc-panel px-5 py-5 lg:px-6 lg:py-6" onsubmit={createSchedule}>
				<p class="gc-stamp">New schedule</p>
				<h2 class="gc-section-title mt-3">Set the next recurring task</h2>

				<div class="mt-6 grid gap-4 md:grid-cols-2">
					<label class="grid gap-2">
						<span class="gc-stamp">Schedule name</span>
						<input bind:value={name} required class="gc-control" placeholder="Nightly repo sweep" />
					</label>

					<label class="grid gap-2">
						<span class="gc-stamp">Cadence type</span>
						<select bind:value={kind} class="gc-control">
							<option value="every">Every</option>
							<option value="at">Once</option>
							<option value="cron">Cron</option>
						</select>
					</label>
				</div>

				<label class="mt-4 grid gap-2">
					<span class="gc-stamp">Objective</span>
					<textarea
						bind:value={objective}
						rows="4"
						required
						class="gc-control"
						placeholder="Inspect open pull requests and summarize blockers."
					></textarea>
				</label>

				{#if kind === 'every' || kind === 'at'}
					<label class="mt-4 grid gap-2">
						<span class="gc-stamp">Anchor time</span>
						<input bind:value={anchorAt} type="datetime-local" class="gc-control" />
					</label>
				{/if}

				{#if kind === 'every'}
					<label class="mt-4 grid gap-2">
						<span class="gc-stamp">Every hours</span>
						<input bind:value={everyHours} type="number" min="1" class="gc-control" />
					</label>
				{/if}

				{#if kind === 'cron'}
					<div class="mt-4 grid gap-4 md:grid-cols-2">
						<label class="grid gap-2">
							<span class="gc-stamp">Cron expression</span>
							<input bind:value={cronExpr} class="gc-control" />
						</label>

						<label class="grid gap-2">
							<span class="gc-stamp">Timezone</span>
							<input bind:value={timezone} class="gc-control" />
						</label>
					</div>
				{/if}

				<SurfaceActionButton type="submit" tone="solid" className="mt-6" disabled={saving}>
					{saving ? 'Creating schedule' : 'Create schedule'}
				</SurfaceActionButton>
			</form>

			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<div class="flex items-end justify-between gap-4">
					<div>
						<p class="gc-stamp">Scheduled tasks</p>
						<h2 class="gc-section-title mt-3">See which schedules are healthy or drifting</h2>
					</div>
					<p class="gc-machine">{data.automate.schedules.length} visible schedules</p>
				</div>

				<div class="mt-6 grid gap-4">
					{#if data.automate.schedules.length === 0}
						<SurfaceEmptyState
							label="No schedules yet"
							title="No recurring work is scheduled yet"
							message="Create the first recurring task to keep work moving automatically."
						/>
					{:else}
						{#each data.automate.schedules as schedule (schedule.id)}
							<article class="gc-panel-soft px-4 py-4">
								<div class="flex items-start justify-between gap-4">
									<div>
										<p class="gc-stamp">{schedule.kind_label} · {schedule.enabled_label}</p>
										<h3 class="gc-panel-title mt-3 text-[1rem]">{schedule.name}</h3>
									</div>
									<p class={`gc-machine ${schedule.status_class}`}>{schedule.status_label}</p>
								</div>

								<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">{schedule.objective}</p>
								<p class="gc-machine mt-4">{schedule.cadence_label}</p>

								<div
									class="mt-4 grid gap-2 border-t-2 border-[var(--gc-border)] pt-4 md:grid-cols-2"
								>
									<div>
										<p class="gc-stamp">Next wake</p>
										<p class="gc-copy mt-2 text-[var(--gc-ink)]">{schedule.next_run_at_label}</p>
									</div>
									<div>
										<p class="gc-stamp">Last run</p>
										<p class="gc-copy mt-2 text-[var(--gc-ink)]">{schedule.last_run_at_label}</p>
									</div>
								</div>

								{#if schedule.last_error}
									<p class="gc-copy mt-4 text-[var(--gc-orange)]">{schedule.last_error}</p>
								{/if}

								<div class="mt-5 flex flex-wrap gap-3">
									<SurfaceActionButton
										type="button"
										onclick={() => launchNow(schedule.id)}
										disabled={mutatingID !== '' && mutatingID !== schedule.id}
										tone={statusTone(schedule.status_class)}
									>
										Run now
									</SurfaceActionButton>
									<SurfaceActionButton
										type="button"
										onclick={() => toggleSchedule(schedule.id, schedule.enabled)}
										disabled={mutatingID !== '' && mutatingID !== schedule.id}
										tone="warning"
									>
										{schedule.enabled ? 'Pause schedule' : 'Resume schedule'}
									</SurfaceActionButton>
								</div>
							</article>
						{/each}
					{/if}
				</div>
			</div>
		</div>

		<div class="grid gap-6">
			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Running now</p>
				<h2 class="gc-section-title mt-3">See scheduled work that is already running</h2>
				<div class="mt-4 grid gap-4">
					{#if data.automate.open_occurrences.length === 0}
						<SurfaceEmptyState
							label="Nothing running now"
							title="No schedule is running right now"
							message="Recurring work will show up here while it is in progress."
						/>
					{:else}
						{#each data.automate.open_occurrences as occurrence (occurrence.id)}
							<article class="gc-panel-soft px-4 py-4">
								<div class="flex items-start justify-between gap-4">
									<div>
										<p class="gc-stamp">{occurrence.schedule_name}</p>
										<h3 class={`gc-panel-title mt-3 text-[1rem] ${occurrence.status_class}`}>
											{occurrence.status_label}
										</h3>
									</div>
									<p class="gc-machine">{occurrence.updated_at_label}</p>
								</div>

								<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">
									Slot {occurrence.slot_at_label}
								</p>

								{#if occurrence.run_id}
									<a
										class="gc-machine mt-4 inline-flex underline"
										href={resolve('/work/[runId]', { runId: occurrence.run_id })}
									>
										Open run {occurrence.run_id}
									</a>
								{/if}
							</article>
						{/each}
					{/if}
				</div>
			</div>

			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Recent executions</p>
				<h2 class="gc-section-title mt-3">Check the last result before the next run</h2>
				<div class="mt-4 grid gap-4">
					{#if data.automate.recent_occurrences.length === 0}
						<SurfaceEmptyState
							label="No recent runs"
							title="No schedule has finished yet"
							message="Recent results will show up here after the first schedule runs."
						/>
					{:else}
						{#each data.automate.recent_occurrences as occurrence (occurrence.id)}
							<article class="gc-panel-soft px-4 py-4">
								<div class="flex items-start justify-between gap-4">
									<div>
										<p class="gc-stamp">{occurrence.schedule_name}</p>
										<h3 class={`gc-panel-title mt-3 text-[1rem] ${occurrence.status_class}`}>
											{occurrence.status_label}
										</h3>
									</div>
									<p class="gc-machine">{occurrence.updated_at_label}</p>
								</div>

								<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">
									Slot {occurrence.slot_at_label}
								</p>

								{#if occurrence.error}
									<p class="gc-copy mt-4 text-[var(--gc-orange)]">{occurrence.error}</p>
								{/if}

								{#if occurrence.run_id}
									<a
										class="gc-machine mt-4 inline-flex underline"
										href={resolve('/work/[runId]', { runId: occurrence.run_id })}
									>
										Open run {occurrence.run_id}
									</a>
								{/if}
							</article>
						{/each}
					{/if}
				</div>
			</div>
		</div>
	</section>
</div>

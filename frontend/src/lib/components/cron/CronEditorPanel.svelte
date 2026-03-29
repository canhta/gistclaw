<script lang="ts">
	import SurfaceActionButton from '$lib/components/common/SurfaceActionButton.svelte';
	import type { AutomateEditorErrors, AutomateEditorState } from '$lib/automate/editor';

	let {
		state = $bindable(),
		errors = {},
		submitting = false,
		onsubmit
	}: {
		state: AutomateEditorState;
		errors?: AutomateEditorErrors;
		submitting?: boolean;
		onsubmit?: (event: SubmitEvent) => void;
	} = $props();
</script>

<form class="grid gap-5" {onsubmit}>
	<div class="grid gap-5 xl:grid-cols-[minmax(0,1.2fr)_minmax(20rem,0.8fr)]">
		<section class="gc-panel-soft px-4 py-4">
			<p class="gc-stamp text-[var(--gc-ink-3)]">Identity</p>

			<label class="mt-4 flex flex-col gap-2">
				<span class="gc-copy text-[var(--gc-ink-2)]">Name</span>
				<input
					bind:value={state.name}
					class="gc-control min-h-[2.75rem]"
					placeholder="Daily digest"
				/>
				{#if errors.name}
					<span class="gc-secondary text-[var(--gc-error)]">{errors.name}</span>
				{/if}
			</label>

			<label class="mt-4 flex flex-col gap-2">
				<span class="gc-copy text-[var(--gc-ink-2)]">Objective</span>
				<textarea
					bind:value={state.objective}
					class="gc-control min-h-[10rem]"
					placeholder="Send a daily summary"
				></textarea>
				{#if errors.objective}
					<span class="gc-secondary text-[var(--gc-error)]">{errors.objective}</span>
				{/if}
			</label>
		</section>

		<section class="gc-panel-soft px-4 py-4">
			<p class="gc-stamp text-[var(--gc-ink-3)]">Schedule</p>

			<label class="mt-4 flex flex-col gap-2">
				<span class="gc-copy text-[var(--gc-ink-2)]">Schedule type</span>
				<select bind:value={state.kind} class="gc-control min-h-[2.75rem]">
					<option value="cron">Cron</option>
					<option value="every">Every</option>
					<option value="at">Once</option>
				</select>
			</label>

			{#if state.kind === 'cron'}
				<label class="mt-4 flex flex-col gap-2">
					<span class="gc-copy text-[var(--gc-ink-2)]">Cron expression</span>
					<input
						bind:value={state.cronExpr}
						class="gc-control min-h-[2.75rem]"
						placeholder="0 9 * * *"
					/>
					{#if errors.cronExpr}
						<span class="gc-secondary text-[var(--gc-error)]">{errors.cronExpr}</span>
					{/if}
				</label>

				<label class="mt-4 flex flex-col gap-2">
					<span class="gc-copy text-[var(--gc-ink-2)]">Timezone</span>
					<input bind:value={state.timezone} class="gc-control min-h-[2.75rem]" placeholder="UTC" />
				</label>
			{:else}
				<label class="mt-4 flex flex-col gap-2">
					<span class="gc-copy text-[var(--gc-ink-2)]">Start time</span>
					<input
						bind:value={state.anchorAt}
						type="datetime-local"
						class="gc-control min-h-[2.75rem]"
					/>
					{#if errors.anchorAt}
						<span class="gc-secondary text-[var(--gc-error)]">{errors.anchorAt}</span>
					{/if}
				</label>

				{#if state.kind === 'every'}
					<label class="mt-4 flex flex-col gap-2">
						<span class="gc-copy text-[var(--gc-ink-2)]">Repeat every hours</span>
						<input
							bind:value={state.everyHours}
							type="number"
							min="1"
							step="1"
							class="gc-control min-h-[2.75rem]"
							placeholder="24"
						/>
						{#if errors.everyHours}
							<span class="gc-secondary text-[var(--gc-error)]">{errors.everyHours}</span>
						{/if}
					</label>
				{/if}
			{/if}
		</section>
	</div>

	<div class="flex justify-end">
		<SurfaceActionButton type="submit" tone="solid" disabled={submitting}>
			{submitting ? 'Creating Job' : 'Create Job'}
		</SurfaceActionButton>
	</div>
</form>

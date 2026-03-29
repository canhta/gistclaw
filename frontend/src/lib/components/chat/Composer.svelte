<script lang="ts">
	import type { RunStatus } from '$lib/chat/types';

	let {
		runStatus,
		canInject,
		onSend,
		onInject,
		onStop
	}: {
		runStatus: RunStatus;
		canInject: boolean;
		onSend: (text: string) => void;
		onInject: (text: string) => void;
		onStop: () => void;
	} = $props();

	let text = $state('');
	const isActive = $derived(runStatus === 'active');
	const canSend = $derived(text.trim().length > 0 && !isActive);
	const canInjectNow = $derived(text.trim().length > 0 && canInject);

	function handleSubmit(e: SubmitEvent): void {
		e.preventDefault();
		if (isActive) {
			onStop();
			return;
		}
		const trimmed = text.trim();
		if (!trimmed) return;
		onSend(trimmed);
		text = '';
	}

	function handleInject(): void {
		const trimmed = text.trim();
		if (!trimmed || !canInject) return;
		onInject(trimmed);
		text = '';
	}

	function handleKeydown(e: KeyboardEvent): void {
		if (e.key === 'Enter' && !e.shiftKey && !isActive) {
			e.preventDefault();
			const trimmed = text.trim();
			if (!trimmed) return;
			onSend(trimmed);
			text = '';
		}
	}
</script>

<div
	class="shrink-0 border-t border-t-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)] px-4 py-3"
>
	<form class="flex items-end gap-3" onsubmit={handleSubmit}>
		<textarea
			bind:value={text}
			onkeydown={handleKeydown}
			placeholder="Type a message"
			rows={1}
			aria-label="Message input"
			class="gc-control min-h-[2.5rem] flex-1 resize-none py-2 leading-normal"
			style="field-sizing: content; max-height: 7.5rem;"
		></textarea>

		<button
			type="button"
			disabled={!canInjectNow}
			aria-disabled={!canInjectNow}
			onclick={handleInject}
			title={canInject
				? 'Inject note into the selected run'
				: 'Select an active run to inject notes'}
			class="gc-action shrink-0 px-4 disabled:opacity-40"
		>
			INJECT
		</button>

		{#if isActive}
			<button
				type="submit"
				class="gc-action gc-action-warning shrink-0 px-4"
				aria-label="Stop the active run"
			>
				STOP
			</button>
		{:else}
			<button
				type="submit"
				disabled={!canSend}
				aria-disabled={!canSend}
				class="gc-action gc-action-solid shrink-0 px-4 disabled:opacity-40"
				aria-label="Send message"
			>
				SEND
			</button>
		{/if}
	</form>
</div>

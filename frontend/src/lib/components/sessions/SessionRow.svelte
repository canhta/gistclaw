<script lang="ts">
	import type { ConversationIndexItemResponse } from '$lib/types/api';

	let {
		session,
		selected,
		onclick
	}: {
		session: ConversationIndexItemResponse;
		selected: boolean;
		onclick?: () => void;
	} = $props();
</script>

<tr
	{onclick}
	class="cursor-pointer border-b border-[var(--gc-border)] transition-colors hover:bg-[var(--gc-surface-raised)] {selected
		? 'gc-primary bg-[var(--gc-surface-raised)]'
		: ''}"
>
	<td class="px-4 py-3 align-top">
		<span class="gc-stamp text-[var(--gc-ink-2)]">{session.id}</span>
	</td>
	<td class="px-4 py-3 align-top">
		<span class="gc-copy text-[var(--gc-ink-2)]">{session.agent_id}</span>
	</td>
	<td class="px-4 py-3 align-top">
		<span class="gc-copy text-[var(--gc-ink-3)]">{session.role_label}</span>
	</td>
	<td class="px-4 py-3 align-top">
		<span class="gc-badge border-[var(--gc-border)] text-[var(--gc-ink-3)]">
			{session.status_label}
		</span>
	</td>
	<td class="px-4 py-3 align-top">
		<time class="gc-machine text-[var(--gc-ink-4)]">{session.updated_at_label}</time>
	</td>
</tr>

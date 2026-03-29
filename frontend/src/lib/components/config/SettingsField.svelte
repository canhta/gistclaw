<script lang="ts">
	export interface SelectOption {
		value: string;
		label: string;
	}

	let {
		id,
		label,
		value = $bindable(''),
		type = 'text',
		hint,
		options,
		placeholder = ''
	}: {
		id: string;
		label: string;
		value?: string;
		type?: 'text' | 'select' | 'password';
		hint?: string;
		options?: SelectOption[];
		placeholder?: string;
	} = $props();
</script>

<div class="flex flex-col gap-2">
	<label for={id} class="gc-stamp text-[var(--gc-ink-3)]">{label}</label>

	{#if type === 'select' && options}
		<select {id} bind:value class="gc-control min-h-[2.75rem]">
			{#each options as option (option.value)}
				<option value={option.value}>{option.label}</option>
			{/each}
		</select>
	{:else}
		<input {id} {type} bind:value {placeholder} class="gc-control min-h-[2.75rem]" />
	{/if}

	{#if hint}
		<p class="gc-copy text-sm text-[var(--gc-ink-3)]">{hint}</p>
	{/if}
</div>

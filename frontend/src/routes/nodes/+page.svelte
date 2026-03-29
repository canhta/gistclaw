<script lang="ts">
	import PlaceholderSectionShell from '$lib/components/common/PlaceholderSectionShell.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = [
		{
			id: 'list',
			label: 'List',
			title: 'Node inventory',
			description: 'Node inventory is not connected to a backend yet.'
		},
		{
			id: 'capabilities',
			label: 'Capabilities',
			title: 'Node capabilities',
			description: 'Capability inspection is not connected to a backend yet.'
		}
	];
</script>

<svelte:head>
	<title>Nodes | GistClaw</title>
</svelte:head>

<PlaceholderSectionShell sectionTitle="Nodes" {tabs} currentPath={data.currentPath} />

<script lang="ts">
	import PlaceholderSectionShell from '$lib/components/common/PlaceholderSectionShell.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = [
		{
			id: 'status',
			label: 'Status',
			title: 'Runtime status',
			description: 'Runtime status is not connected to a backend yet.'
		},
		{
			id: 'health',
			label: 'Health',
			title: 'System health',
			description: 'Health inspection is not connected to a backend yet.'
		},
		{
			id: 'models',
			label: 'Models',
			title: 'Model inventory',
			description: 'Model inventory is not connected to a backend yet.'
		},
		{
			id: 'events',
			label: 'Events',
			title: 'Runtime events',
			description: 'Event inspection is not connected to a backend yet.'
		},
		{
			id: 'rpc',
			label: 'RPC',
			title: 'RPC console',
			description: 'RPC control is not connected to a backend yet.'
		}
	];
</script>

<svelte:head>
	<title>Debug | GistClaw</title>
</svelte:head>

<PlaceholderSectionShell sectionTitle="Debug" {tabs} currentPath={data.currentPath} />

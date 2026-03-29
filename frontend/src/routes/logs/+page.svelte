<script lang="ts">
	import PlaceholderSectionShell from '$lib/components/common/PlaceholderSectionShell.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = [
		{
			id: 'live-tail',
			label: 'Live Tail',
			title: 'Live log tail',
			description: 'Log streaming is not connected to a backend yet.'
		},
		{
			id: 'filters',
			label: 'Filters',
			title: 'Log filters',
			description: 'Log filtering is not connected to a backend yet.'
		},
		{
			id: 'export',
			label: 'Export',
			title: 'Log export',
			description: 'Log export is not connected to a backend yet.'
		}
	];
</script>

<svelte:head>
	<title>Logs | GistClaw</title>
</svelte:head>

<PlaceholderSectionShell sectionTitle="Logs" {tabs} currentPath={data.currentPath} />

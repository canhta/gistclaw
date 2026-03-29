<script lang="ts">
	import PlaceholderSectionShell from '$lib/components/common/PlaceholderSectionShell.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = [
		{
			id: 'presence',
			label: 'Presence',
			title: 'Instance presence',
			description: 'Instances are not connected to a backend yet.'
		},
		{
			id: 'details',
			label: 'Details',
			title: 'Instance details',
			description: 'Instance inventory and detail inspection are not connected to a backend yet.'
		}
	];
</script>

<svelte:head>
	<title>Instances | GistClaw</title>
</svelte:head>

<PlaceholderSectionShell sectionTitle="Instances" {tabs} currentPath={data.currentPath} />

<script lang="ts">
	import PlaceholderSectionShell from '$lib/components/common/PlaceholderSectionShell.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = [
		{
			id: 'run-update',
			label: 'Run Update',
			title: 'Run update',
			description: 'Update workflow is not connected to a backend yet.'
		},
		{
			id: 'restart-report',
			label: 'Restart Report',
			title: 'Restart report',
			description: 'Restart reporting is not connected to a backend yet.'
		}
	];
</script>

<svelte:head>
	<title>Update | GistClaw</title>
</svelte:head>

<PlaceholderSectionShell sectionTitle="Update" {tabs} currentPath={data.currentPath} />

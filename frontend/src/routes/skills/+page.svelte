<script lang="ts">
	import PlaceholderSectionShell from '$lib/components/common/PlaceholderSectionShell.svelte';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = [
		{
			id: 'installed',
			label: 'Installed',
			title: 'Installed skills',
			description: 'Skill management is not connected to a backend yet.'
		},
		{
			id: 'available',
			label: 'Available',
			title: 'Available skills',
			description: 'Skill discovery is not connected to a backend yet.'
		},
		{
			id: 'credentials',
			label: 'Credentials',
			title: 'Skill credentials',
			description: 'Skill credential management is not connected to a backend yet.'
		}
	];
</script>

<svelte:head>
	<title>Skills | GistClaw</title>
</svelte:head>

<PlaceholderSectionShell sectionTitle="Skills" {tabs} currentPath={data.currentPath} />

<script lang="ts">
	import SurfacePlaceholder from '$lib/components/shell/SurfacePlaceholder.svelte';
	import { surfaceByID } from '$lib/config/surfaces';

	const surface = surfaceByID('team');
</script>

<svelte:head>
	<title>{surface.title} | GistClaw</title>
</svelte:head>

<SurfacePlaceholder {surface} />

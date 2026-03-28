<script lang="ts">
	import type { Icon as TablerIcon } from '@tabler/icons-svelte-runes';
	import {
		IconBooks,
		IconBriefcase2,
		IconCalendarStats,
		IconHistory,
		IconLayoutGrid,
		IconLifebuoy,
		IconMessages,
		IconSettingsCog,
		IconUsersGroup
	} from '@tabler/icons-svelte-runes';

	const icons: Record<string, TablerIcon> = {
		work: IconBriefcase2,
		team: IconUsersGroup,
		knowledge: IconBooks,
		recover: IconLifebuoy,
		conversations: IconMessages,
		automate: IconCalendarStats,
		history: IconHistory,
		settings: IconSettingsCog
	};

	let { surfaceID }: { surfaceID: string } = $props();

	const Icon = $derived(icons[surfaceID] ?? IconLayoutGrid);
</script>

<span
	data-nav-icon={surfaceID}
	class="inline-flex h-10 w-10 items-center justify-center border-2 border-[var(--gc-border)] bg-[var(--gc-canvas)] text-[var(--gc-text-secondary)]"
>
	<Icon aria-hidden="true" size={18} stroke={1.8} />
</span>

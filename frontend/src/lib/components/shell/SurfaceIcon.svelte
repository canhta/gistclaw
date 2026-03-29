<script lang="ts">
	import type { Icon as TablerIcon } from '@tabler/icons-svelte-runes';
	import {
		IconBug,
		IconCalendarStats,
		IconCpu,
		IconFileText,
		IconGitBranch,
		IconLayoutGrid,
		IconList,
		IconMessages,
		IconRefresh,
		IconSettings2,
		IconShieldCheck,
		IconTerminal2,
		IconWifi
	} from '@tabler/icons-svelte-runes';

	const icons: Record<string, TablerIcon> = {
		chat: IconMessages,
		channels: IconWifi,
		instances: IconCpu,
		sessions: IconGitBranch,
		cron: IconCalendarStats,
		skills: IconFileText,
		nodes: IconTerminal2,
		approvals: IconShieldCheck,
		config: IconSettings2,
		debug: IconBug,
		logs: IconList,
		update: IconRefresh
	};

	let { surfaceID }: { surfaceID: string } = $props();

	const Icon = $derived(icons[surfaceID] ?? IconLayoutGrid);
</script>

<span
	data-nav-icon={surfaceID}
	class="inline-flex items-center justify-center text-[var(--gc-ink-2)]"
>
	<Icon aria-hidden="true" size={18} stroke={1.6} />
</span>

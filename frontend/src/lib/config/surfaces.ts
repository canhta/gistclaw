// Section configuration for the 12 left-nav sections.

export type SectionID =
	| 'chat'
	| 'channels'
	| 'instances'
	| 'sessions'
	| 'cron'
	| 'skills'
	| 'nodes'
	| 'approvals'
	| 'config'
	| 'debug'
	| 'logs'
	| 'update';

export interface SectionMeta {
	id: SectionID;
	href: string;
	label: string;
}

export const sections: SectionMeta[] = [
	{ id: 'chat', href: '/chat', label: 'Chat' },
	{ id: 'channels', href: '/channels', label: 'Channels' },
	{ id: 'instances', href: '/instances', label: 'Instances' },
	{ id: 'sessions', href: '/sessions', label: 'Sessions' },
	{ id: 'cron', href: '/cron', label: 'Cron Jobs' },
	{ id: 'skills', href: '/skills', label: 'Skills' },
	{ id: 'nodes', href: '/nodes', label: 'Nodes' },
	{ id: 'approvals', href: '/approvals', label: 'Exec Approvals' },
	{ id: 'config', href: '/config', label: 'Config' },
	{ id: 'debug', href: '/debug', label: 'Debug' },
	{ id: 'logs', href: '/logs', label: 'Logs' },
	{ id: 'update', href: '/update', label: 'Update' }
];

export const sectionMap = Object.fromEntries(sections.map((s) => [s.id, s])) as Record<
	SectionID,
	SectionMeta
>;

export function sectionForPath(path: string): SectionMeta | undefined {
	const key = path.replace(/^\/+/, '').split('/')[0] as SectionID;
	return sectionMap[key];
}

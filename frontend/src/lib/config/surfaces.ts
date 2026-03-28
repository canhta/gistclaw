type Tone = 'default' | 'accent' | 'warning';

export type SurfaceID =
	| 'work'
	| 'team'
	| 'knowledge'
	| 'recover'
	| 'conversations'
	| 'automate'
	| 'history'
	| 'settings';

interface SurfaceInspectorItem {
	label: string;
	value: string;
	tone?: Tone;
}

export interface SurfaceMeta {
	id: SurfaceID;
	inspectorTitle: string;
	inspectorItems: SurfaceInspectorItem[];
}

const surfaces: Record<SurfaceID, SurfaceMeta> = {
	work: {
		id: 'work',
		inspectorTitle: 'At a glance',
		inspectorItems: [
			{ label: 'Best for', value: 'Start and steer work', tone: 'accent' },
			{ label: 'Watch for', value: 'Approvals and blocked runs', tone: 'warning' },
			{ label: 'Next', value: 'Open the run that matters' }
		]
	},
	team: {
		id: 'team',
		inspectorTitle: 'Setup',
		inspectorItems: [
			{ label: 'Best for', value: 'Team shape and roles', tone: 'accent' },
			{ label: 'Watch for', value: 'Unclear ownership', tone: 'warning' },
			{ label: 'Next', value: 'Save the setup you trust' }
		]
	},
	knowledge: {
		id: 'knowledge',
		inspectorTitle: 'Remember',
		inspectorItems: [
			{ label: 'Best for', value: 'Rules and context', tone: 'accent' },
			{ label: 'Watch for', value: 'Stale guidance', tone: 'warning' },
			{ label: 'Next', value: 'Review what still matters' }
		]
	},
	recover: {
		id: 'recover',
		inspectorTitle: 'Needs attention',
		inspectorItems: [
			{ label: 'Best for', value: 'Approvals and retries', tone: 'accent' },
			{ label: 'Check first', value: 'Expired approvals', tone: 'warning' },
			{ label: 'Next', value: 'Clear the queue' }
		]
	},
	conversations: {
		id: 'conversations',
		inspectorTitle: 'Inbox health',
		inspectorItems: [
			{ label: 'Best for', value: 'Sessions and replies', tone: 'accent' },
			{ label: 'Check first', value: 'Failed deliveries', tone: 'warning' },
			{ label: 'Next', value: 'Open the right thread' }
		]
	},
	automate: {
		id: 'automate',
		inspectorTitle: 'Schedule health',
		inspectorItems: [
			{ label: 'Best for', value: 'Recurring tasks', tone: 'accent' },
			{ label: 'Check first', value: 'Next run and drift', tone: 'warning' },
			{ label: 'Next', value: 'Review the schedule list' }
		]
	},
	history: {
		id: 'history',
		inspectorTitle: 'What changed',
		inspectorItems: [
			{ label: 'Best for', value: 'Finished runs and evidence', tone: 'accent' },
			{ label: 'Check first', value: 'Approvals and failures', tone: 'warning' },
			{ label: 'Next', value: 'Open the clearest run' }
		]
	},
	settings: {
		id: 'settings',
		inspectorTitle: 'Machine info',
		inspectorItems: [
			{ label: 'Best for', value: 'Access and limits', tone: 'accent' },
			{ label: 'Check first', value: 'Signed-in browsers', tone: 'warning' },
			{ label: 'Next', value: 'Update the defaults' }
		]
	}
};

export function surfaceByID(id: SurfaceID): SurfaceMeta {
	return surfaces[id];
}

export function surfaceForPath(path: string): SurfaceMeta {
	const key = path.replace(/^\/+/, '').split('/')[0] as SurfaceID;

	return surfaces[key] ?? surfaces.work;
}

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

interface SurfaceCard {
	label: string;
	value: string;
	detail: string;
	tone?: Tone;
}

interface SurfaceInspectorItem {
	label: string;
	value: string;
	tone?: Tone;
}

export interface SurfaceMeta {
	id: SurfaceID;
	title: string;
	description: string;
	workspaceEyebrow: string;
	workspaceTitle: string;
	workspaceBody: string;
	cards: SurfaceCard[];
	inspectorTitle: string;
	inspectorItems: SurfaceInspectorItem[];
}

const surfaces: Record<SurfaceID, SurfaceMeta> = {
	work: {
		id: 'work',
		title: 'Work',
		description: 'Start tasks, watch progress, and step in when a run needs you.',
		workspaceEyebrow: 'Current work',
		workspaceTitle: 'Start the next task and keep it moving',
		workspaceBody:
			'Write the task once, launch it, and jump straight to the run that needs your attention.',
		cards: [
			{
				label: 'Start task',
				value: 'Queue now',
				detail: 'Describe the outcome you want and launch it with the right repo in view.',
				tone: 'accent'
			},
			{
				label: 'Live runs',
				value: 'Follow progress',
				detail: 'See what is moving, what is blocked, and where you may need to step in.'
			},
			{
				label: 'Needs input',
				value: 'Catch blockers',
				detail: 'Bring approvals and stalled work to the top before they slow everything down.',
				tone: 'warning'
			}
		],
		inspectorTitle: 'At a glance',
		inspectorItems: [
			{ label: 'Best for', value: 'Start and steer work', tone: 'accent' },
			{ label: 'Watch for', value: 'Approvals and blocked runs', tone: 'warning' },
			{ label: 'Next', value: 'Open the run that matters' }
		]
	},
	team: {
		id: 'team',
		title: 'Team',
		description: 'Choose who leads, who helps, and how work gets handed off.',
		workspaceEyebrow: 'Team setup',
		workspaceTitle: 'Make responsibility clear before work fans out',
		workspaceBody:
			'Pick the lead role, shape the specialists, and make handoffs obvious before the next run starts.',
		cards: [
			{
				label: 'Lead role',
				value: 'One clear voice',
				detail: 'Choose who speaks first and keeps the working relationship consistent.',
				tone: 'accent'
			},
			{
				label: 'Specialists',
				value: 'Purposeful roles',
				detail: 'Show who researches, writes, reviews, and verifies.'
			},
			{
				label: 'Handoffs',
				value: 'Visible ownership',
				detail: 'Make it clear when work is parallel, blocked, or waiting on review.',
				tone: 'warning'
			}
		],
		inspectorTitle: 'Setup',
		inspectorItems: [
			{ label: 'Best for', value: 'Team shape and roles', tone: 'accent' },
			{ label: 'Watch for', value: 'Unclear ownership', tone: 'warning' },
			{ label: 'Next', value: 'Save the setup you trust' }
		]
	},
	knowledge: {
		id: 'knowledge',
		title: 'Knowledge',
		description: 'Keep the facts and rules future work should follow.',
		workspaceEyebrow: 'Saved context',
		workspaceTitle: 'Store the context future work should respect',
		workspaceBody:
			'Capture facts and rules in plain language so future runs follow them without guesswork.',
		cards: [
			{
				label: 'Key facts',
				value: 'Keep what matters',
				detail: 'Save the rules and project facts that change decisions later.',
				tone: 'accent'
			},
			{
				label: 'Scope',
				value: 'Right level',
				detail: 'Separate project guidance from broader machine defaults.'
			},
			{
				label: 'Impact',
				value: 'Explain why',
				detail: 'Each item should tell the reader what it changes.'
			}
		],
		inspectorTitle: 'Remember',
		inspectorItems: [
			{ label: 'Best for', value: 'Rules and context', tone: 'accent' },
			{ label: 'Watch for', value: 'Stale guidance', tone: 'warning' },
			{ label: 'Next', value: 'Review what still matters' }
		]
	},
	recover: {
		id: 'recover',
		title: 'Recover',
		description: 'Clear approvals, retry failed work, and fix broken routes.',
		workspaceEyebrow: 'Recovery',
		workspaceTitle: 'Fix what is blocked before it piles up',
		workspaceBody:
			'Start with the items waiting on you, then repair routes or retries without digging through logs.',
		cards: [
			{
				label: 'Approvals',
				value: 'Decide now',
				detail: 'Put pending decisions first so work can move again.',
				tone: 'warning'
			},
			{
				label: 'Blocked runs',
				value: 'Open context',
				detail: 'Pair each stalled run with the detail you need to act.'
			},
			{
				label: 'Failed delivery',
				value: 'Retry fast',
				detail: 'Bring retries and route fixes into one clear workflow.',
				tone: 'accent'
			}
		],
		inspectorTitle: 'Needs attention',
		inspectorItems: [
			{ label: 'Best for', value: 'Approvals and retries', tone: 'accent' },
			{ label: 'Check first', value: 'Expired approvals', tone: 'warning' },
			{ label: 'Next', value: 'Clear the queue' }
		]
	},
	conversations: {
		id: 'conversations',
		title: 'Conversations',
		description: 'See active conversations, channel health, and replies that need help.',
		workspaceEyebrow: 'Conversations',
		workspaceTitle: 'Keep incoming and outgoing conversations easy to follow',
		workspaceBody:
			'Open the thread that needs context, check channel health, and catch replies that did not land.',
		cards: [
			{
				label: 'Active threads',
				value: 'Who is talking',
				detail: 'Show live conversations and recent activity.'
			},
			{
				label: 'Channel health',
				value: 'Sending cleanly',
				detail: 'See whether Telegram or WhatsApp is healthy enough to trust.',
				tone: 'accent'
			},
			{
				label: 'Failed replies',
				value: 'Needs follow-up',
				detail: 'Put delivery failures next to the action to retry.',
				tone: 'warning'
			}
		],
		inspectorTitle: 'Inbox health',
		inspectorItems: [
			{ label: 'Best for', value: 'Sessions and replies', tone: 'accent' },
			{ label: 'Check first', value: 'Failed deliveries', tone: 'warning' },
			{ label: 'Next', value: 'Open the right thread' }
		]
	},
	automate: {
		id: 'automate',
		title: 'Automate',
		description: 'Schedule recurring work and catch runs that may slip.',
		workspaceEyebrow: 'Recurring work',
		workspaceTitle: 'Keep repeat work moving on time',
		workspaceBody:
			'Set recurring tasks, see what runs next, and spot schedules that need attention before they drift.',
		cards: [
			{
				label: 'Next runs',
				value: 'Coming soon',
				detail: 'See the next scheduled work in plain language.',
				tone: 'accent'
			},
			{
				label: 'Running now',
				value: 'Current load',
				detail: 'See which recurring tasks already have an active run.'
			},
			{
				label: 'Needs review',
				value: 'Fix drift early',
				detail: 'Call out disabled or unhealthy schedules before they slip.',
				tone: 'warning'
			}
		],
		inspectorTitle: 'Schedule health',
		inspectorItems: [
			{ label: 'Best for', value: 'Recurring tasks', tone: 'accent' },
			{ label: 'Check first', value: 'Next run and drift', tone: 'warning' },
			{ label: 'Next', value: 'Review the schedule list' }
		]
	},
	history: {
		id: 'history',
		title: 'History',
		description: 'Review finished work, approvals, and delivery results.',
		workspaceEyebrow: 'History',
		workspaceTitle: 'See what happened and why',
		workspaceBody:
			'Use finished runs, approvals, and delivery results to understand the last outcome before starting the next move.',
		cards: [
			{
				label: 'Finished runs',
				value: 'Past work',
				detail: 'Open the run that best explains what happened.',
				tone: 'accent'
			},
			{
				label: 'Approvals',
				value: 'Recorded decisions',
				detail: 'See where a human decision changed the path.'
			},
			{
				label: 'Deliveries',
				value: 'Outcome receipts',
				detail: 'Check what reached the user and what failed.'
			}
		],
		inspectorTitle: 'What changed',
		inspectorItems: [
			{ label: 'Best for', value: 'Finished runs and evidence', tone: 'accent' },
			{ label: 'Check first', value: 'Approvals and failures', tone: 'warning' },
			{ label: 'Next', value: 'Open the clearest run' }
		]
	},
	settings: {
		id: 'settings',
		title: 'Settings',
		description: 'Manage browser access, active project, and daily work limits.',
		workspaceEyebrow: 'Settings',
		workspaceTitle: 'Keep access and limits clear',
		workspaceBody:
			'Use settings for browser access, project selection, and machine limits that shape everyday work.',
		cards: [
			{
				label: 'Browser access',
				value: 'Trusted devices',
				detail: 'See who can open this app and remove access when needed.',
				tone: 'accent'
			},
			{
				label: 'Active project',
				value: 'Current repo',
				detail: 'Keep the working project visible and easy to change.'
			},
			{
				label: 'Limits',
				value: 'Spend and permissions',
				detail: 'Adjust the machine rules that affect runs and approvals.'
			}
		],
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

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
		description:
			'Steer current objectives, orchestration, and live machine signal from one control deck.',
		workspaceEyebrow: 'Command workspace',
		workspaceTitle: 'Stage the next objective before it disappears into internals',
		workspaceBody:
			'Work is the front door. The operator should see intake, live orchestration, and intervention signal in one mounted surface instead of jumping between passive lists.',
		cards: [
			{
				label: 'Command intake',
				value: 'Queue now',
				detail:
					'Frame the objective, confirm the repo target, and launch the next run with context attached.',
				tone: 'accent'
			},
			{
				label: 'Run graph',
				value: 'XYFlow',
				detail: 'Mount live orchestration as a hard-edged graph instead of a collapsed event feed.'
			},
			{
				label: 'Lane pressure',
				value: 'Watch hot paths',
				detail: 'Show which specialist lanes are busy, blocked, or waiting on the operator.',
				tone: 'warning'
			}
		],
		inspectorTitle: 'Machine signal',
		inspectorItems: [
			{ label: 'Primary surface', value: 'Live control deck', tone: 'accent' },
			{ label: 'Operator role', value: 'Steer and intervene' },
			{ label: 'Next milestone', value: 'Graph, SSE, command intake' }
		]
	},
	team: {
		id: 'team',
		title: 'Team',
		description:
			'Explain who is helping, how delegation is shaped, and where capacity is open right now.',
		workspaceEyebrow: 'Role topology',
		workspaceTitle: 'Make collaboration visible before the run fan-out gets opaque',
		workspaceBody:
			'Team should describe the working shape of the assistant, not dump implementation files. The operator needs role posture, current assignments, and tool authority in one glance.',
		cards: [
			{
				label: 'Front agent',
				value: 'Single command face',
				detail: 'Keep the human relationship anchored in one primary assistant surface.',
				tone: 'accent'
			},
			{
				label: 'Specialists',
				value: 'Bounded roles',
				detail:
					'Show who handles review, repair, research, and delivery instead of hiding it in runtime detail.'
			},
			{
				label: 'Delegation posture',
				value: 'Visible ownership',
				detail: 'Tell the operator when work is parallel, blocked, or waiting on a handoff.',
				tone: 'warning'
			}
		],
		inspectorTitle: 'Posture',
		inspectorItems: [
			{ label: 'Primary question', value: 'Who is helping right now?' },
			{ label: 'Graph role', value: 'Topology and occupancy', tone: 'accent' },
			{ label: 'Next milestone', value: 'Role cards and capability rails' }
		]
	},
	knowledge: {
		id: 'knowledge',
		title: 'Knowledge',
		description:
			'Surface durable context, project rules, and the facts that will shape future work.',
		workspaceEyebrow: 'Durable context',
		workspaceTitle: 'Turn memory into scoped guidance instead of a hidden table',
		workspaceBody:
			'Knowledge should explain why a fact matters, where it applies, and how it changes behavior. It is not an implementation dump of stored rows.',
		cards: [
			{
				label: 'Promoted memory',
				value: 'Context with intent',
				detail: 'Highlight the memories the machine should actually respect during future work.',
				tone: 'accent'
			},
			{
				label: 'Project rules',
				value: 'Scoped constraints',
				detail: 'Keep repo-specific guidance distinct from machine-wide operating rules.'
			},
			{
				label: 'Why it matters',
				value: 'Visible impact',
				detail: 'Every memory item should explain the behavior it changes.'
			}
		],
		inspectorTitle: 'Retention',
		inspectorItems: [
			{ label: 'Primary question', value: 'What should the machine remember?' },
			{ label: 'View style', value: 'Curated, not tabular', tone: 'accent' },
			{ label: 'Next milestone', value: 'Scoped memory cards and edit actions' }
		]
	},
	recover: {
		id: 'recover',
		title: 'Recover',
		description:
			'Intervene in blocked work, approvals, retries, and replay evidence without leaving the cockpit.',
		workspaceEyebrow: 'Intervention bench',
		workspaceTitle: 'Put pending operator work ahead of historical noise',
		workspaceBody:
			'Recover is where the operator clears approvals, repairs routes, and inspects why work stalled. The page should feel urgent and action-bearing, not archival.',
		cards: [
			{
				label: 'Approval queue',
				value: 'Pending first',
				detail: 'Bring unresolved operator decisions to the top of the bench.',
				tone: 'warning'
			},
			{
				label: 'Blocked runs',
				value: 'Replay with evidence',
				detail: 'Pair every blocked state with the context needed to act.'
			},
			{
				label: 'Route repair',
				value: 'Retry with intent',
				detail: 'Turn delivery repair into a clear operator workflow instead of a hidden control.',
				tone: 'accent'
			}
		],
		inspectorTitle: 'Interventions',
		inspectorItems: [
			{ label: 'Primary question', value: 'What needs my decision now?' },
			{ label: 'Operator mode', value: 'Intervene, retry, inspect', tone: 'accent' },
			{ label: 'Next milestone', value: 'Approval queue and replay rail' }
		]
	},
	conversations: {
		id: 'conversations',
		title: 'Conversations',
		description:
			'Control sessions, connector health, and route authority from the user’s communication surface.',
		workspaceEyebrow: 'External surfaces',
		workspaceTitle: 'Keep route ownership and connector state operator-readable',
		workspaceBody:
			'Conversations should explain who is bound, which connector is healthy, and where outbound delivery is failing without forcing the user to think in transport internals first.',
		cards: [
			{
				label: 'Bound sessions',
				value: 'Who is connected',
				detail: 'Show active conversations, routing ownership, and recent activity.'
			},
			{
				label: 'Connector health',
				value: 'Live signal',
				detail: 'Surface Telegram and WhatsApp state with last-success and last-failure evidence.',
				tone: 'accent'
			},
			{
				label: 'Delivery state',
				value: 'Actionable failures',
				detail: 'Pair failed deliveries with the retry path instead of burying them in logs.',
				tone: 'warning'
			}
		],
		inspectorTitle: 'Routes',
		inspectorItems: [
			{ label: 'Primary question', value: 'Which external surfaces are healthy?' },
			{ label: 'View style', value: 'Sessions and route authority', tone: 'accent' },
			{ label: 'Next milestone', value: 'Connector cards and delivery evidence' }
		]
	},
	automate: {
		id: 'automate',
		title: 'Automate',
		description:
			'Show future wakeups, recurring work, and schedule health as operational load, not calendar chrome.',
		workspaceEyebrow: 'Future work',
		workspaceTitle: 'Treat schedules like live machine commitments',
		workspaceBody:
			'Automate should tell the operator what is queued next, how much load is coming, and whether any scheduled work is drifting out of shape.',
		cards: [
			{
				label: 'Next wakeups',
				value: 'Near-term queue',
				detail: 'Make upcoming runs visible in operator language.',
				tone: 'accent'
			},
			{
				label: 'Lane occupancy',
				value: 'Future pressure',
				detail: 'Show which teams or specialists will be saturated by recurring work.'
			},
			{
				label: 'Schedule health',
				value: 'Repair before drift',
				detail: 'Surface disabled, failing, or starved schedules with the action to fix them.',
				tone: 'warning'
			}
		],
		inspectorTitle: 'Scheduling',
		inspectorItems: [
			{ label: 'Primary question', value: 'What will the machine do next?' },
			{ label: 'View style', value: 'Operational, not calendar', tone: 'accent' },
			{ label: 'Next milestone', value: 'Schedule board and health rail' }
		]
	},
	history: {
		id: 'history',
		title: 'History',
		description:
			'Explain what happened through run evidence, replay, delivery outcomes, and operator interventions.',
		workspaceEyebrow: 'Evidence surface',
		workspaceTitle: 'Make past work explainable instead of merely listable',
		workspaceBody:
			'History is where the operator reconstructs why something happened. The surface should privilege evidence and explanation over passive tables.',
		cards: [
			{
				label: 'Run evidence',
				value: 'Explain outcomes',
				detail: 'Connect state changes, lane movement, and operator actions into one narrative.',
				tone: 'accent'
			},
			{
				label: 'Replay',
				value: 'Reconstruct precisely',
				detail: 'Treat replay as a first-class inspection tool, not an internal debug page.'
			},
			{
				label: 'Delivery outcomes',
				value: 'Human-visible receipts',
				detail: 'Show where the machine succeeded, failed, or needed intervention.'
			}
		],
		inspectorTitle: 'Evidence',
		inspectorItems: [
			{ label: 'Primary question', value: 'What happened and why?' },
			{ label: 'View style', value: 'Replay and outcome evidence', tone: 'accent' },
			{ label: 'Next milestone', value: 'Run ledger and evidence inspector' }
		]
	},
	settings: {
		id: 'settings',
		title: 'Settings',
		description:
			'Keep machine and deployment configuration in one hard-edged service manual surface.',
		workspaceEyebrow: 'Machine configuration',
		workspaceTitle: 'Reserve settings for machine facts, not product navigation',
		workspaceBody:
			'Settings should hold password, device access, project activation, and deployment control without drifting into broader product workflow pages.',
		cards: [
			{
				label: 'Browser access',
				value: 'Trusted devices',
				detail: 'Manage who can operate this machine from the browser.',
				tone: 'accent'
			},
			{
				label: 'Project activation',
				value: 'One active root',
				detail: 'Keep the current workspace explicit and reversible.'
			},
			{
				label: 'Machine posture',
				value: 'Local-first control',
				detail: 'Keep deployment facts and operator access visible in one place.'
			}
		],
		inspectorTitle: 'Machine facts',
		inspectorItems: [
			{ label: 'Primary question', value: 'How is this machine configured?' },
			{ label: 'View style', value: 'Service manual, not dashboard', tone: 'accent' },
			{ label: 'Next milestone', value: 'Access board and machine settings' }
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

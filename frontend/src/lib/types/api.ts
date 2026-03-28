export interface AuthSessionResponse {
	authenticated: boolean;
	password_configured: boolean;
	setup_required: boolean;
	login_reason?: string;
	device_id?: string;
}

export interface AuthLoginResponse {
	authenticated: boolean;
	next: string;
}

export interface BootstrapProjectResponse {
	active_id: string;
	active_name: string;
	active_path: string;
}

export interface BootstrapNavItem {
	id: string;
	label: string;
	href: string;
}

export interface BootstrapResponse {
	auth: AuthSessionResponse;
	project: BootstrapProjectResponse;
	navigation: BootstrapNavItem[];
}

export interface RunGraphSummaryResponse {
	total: number;
	pending: number;
	active: number;
	needs_approval: number;
	completed: number;
	failed: number;
	interrupted: number;
	root_status: string;
}

export interface WorkClusterRunResponse {
	id: string;
	objective: string;
	agent_id: string;
	status: string;
	status_label: string;
	status_class: string;
	model_display: string;
	token_summary: string;
	started_at_short: string;
	started_at_exact: string;
	started_at_iso: string;
	last_activity_short: string;
	last_activity_exact: string;
	last_activity_iso: string;
	depth: number;
}

export interface WorkClusterResponse {
	root: WorkClusterRunResponse;
	children: WorkClusterRunResponse[];
	child_count: number;
	child_count_label: string;
	blocker_label: string;
	has_children: boolean;
}

export interface WorkIndexResponse {
	active_project_name: string;
	active_project_path: string;
	queue_strip: {
		headline: string;
		root_runs: number;
		worker_runs: number;
		recovery_runs: number;
		summary: RunGraphSummaryResponse;
	};
	clusters: WorkClusterResponse[];
}

export interface WorkGraphNodeResponse {
	id: string;
	short_id: string;
	short_label: string;
	parent_run_id: string;
	agent_id: string;
	objective: string;
	objective_preview: string;
	status: string;
	status_label: string;
	status_class: string;
	kind: string;
	lane_id: string;
	model_display: string;
	trigger_label?: string;
	executor_label?: string;
	token_summary: string;
	time_label: string;
	started_at_label: string;
	updated_at_label: string;
	depth: number;
	is_root: boolean;
	is_active_path: boolean;
	branch_root_id?: string;
	child_count: number;
	parent_label?: string;
}

export interface WorkGraphEdgeResponse {
	id: string;
	from: string;
	to: string;
	kind: string;
	label: string;
	status_class?: string;
}

export interface WorkGraphResponse {
	root_run_id: string;
	headline: string;
	summary: RunGraphSummaryResponse;
	nodes: WorkGraphNodeResponse[];
	edges: WorkGraphEdgeResponse[];
	active_path: string[];
}

export interface WorkDetailResponse {
	run: {
		id: string;
		short_id: string;
		objective_text: string;
		trigger_label: string;
		status: string;
		status_label: string;
		status_class: string;
		state_label: string;
		started_at_label: string;
		last_activity_label: string;
		model_display: string;
		token_summary: string;
		event_count: number;
		turn_count: number;
		stream_url: string;
		graph_url: string;
		node_detail_url_template: string;
	};
	graph: WorkGraphResponse;
	inspector_seed?: {
		id: string;
		agent_id: string;
		status: string;
	};
}

export interface WorkCreateResponse {
	run_id: string;
	objective: string;
}

export interface TeamProfileResponse {
	id: string;
	label: string;
	active: boolean;
	save_path?: string;
}

export interface TeamMemberResponse {
	id: string;
	role: string;
	soul_file: string;
	base_profile: string;
	tool_families: string[];
	delegation_kinds: string[];
	can_message: string[];
	specialist_summary_visibility: string;
	soul_extra: Record<string, unknown>;
	is_front: boolean;
}

export interface TeamConfigResponse {
	name: string;
	front_agent_id: string;
	member_count: number;
	members: TeamMemberResponse[];
}

export interface TeamResponse {
	notice?: string;
	active_profile: TeamProfileResponse;
	profiles: TeamProfileResponse[];
	team: TeamConfigResponse;
}

export interface KnowledgeFilterResponse {
	scope: string;
	agent_id: string;
	query: string;
	limit: number;
}

export interface KnowledgeItemResponse {
	id: string;
	agent_id: string;
	scope: string;
	content: string;
	source: string;
	provenance: string;
	confidence: number;
	created_at_label: string;
	updated_at_label: string;
}

export interface KnowledgeResponse {
	headline: string;
	filters: KnowledgeFilterResponse;
	summary: {
		visible_count: number;
	};
	items: KnowledgeItemResponse[];
	paging: {
		next_cursor?: string;
		prev_cursor?: string;
		has_next: boolean;
		has_prev: boolean;
	};
}

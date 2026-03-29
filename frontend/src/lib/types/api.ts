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

export interface BootstrapOnboardingResponse {
	completed: boolean;
	entry_href: string;
}

export interface BootstrapNavItem {
	id: string;
	label: string;
	href: string;
}

export interface BootstrapResponse {
	auth: AuthSessionResponse;
	onboarding: BootstrapOnboardingResponse;
	project: BootstrapProjectResponse | null;
	navigation: BootstrapNavItem[];
}

export interface OnboardingTaskCandidateResponse {
	kind: string;
	description: string;
	signal: string;
}

export interface OnboardingResponse {
	completed: boolean;
	entry_href: string;
	project: BootstrapProjectResponse | null;
	suggested_tasks: OnboardingTaskCandidateResponse[];
}

export interface OnboardingPreviewResponse {
	run_id: string;
	next_href: string;
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
	paging: PageLinksResponse;
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

export interface WorkStructuredTextBlockResponse {
	kind: string;
	text?: string;
	items?: string[];
	start?: number;
}

export interface WorkStructuredTextResponse {
	plain_text?: string;
	preview_text?: string;
	has_overflow: boolean;
	blocks?: WorkStructuredTextBlockResponse[];
}

export interface WorkNodeChainStepResponse {
	run_id: string;
	short_id: string;
	agent_id: string;
	status: string;
	status_label: string;
}

export interface WorkNodeChainResponse {
	path: WorkNodeChainStepResponse[];
	children?: WorkNodeChainStepResponse[];
}

export interface WorkNodeApprovalResponse {
	id: string;
	tool_name: string;
	binding_summary?: string;
	reason?: string;
	status: string;
	status_label: string;
	status_class: string;
	requested_at_label?: string;
	resolved_at_label?: string;
	resolve_url?: string;
	view_url?: string;
	can_resolve: boolean;
}

export interface WorkNodeLogEntryResponse {
	title: string;
	body: string;
	stream: string;
	tool_name: string;
	tool_call_id?: string;
	entry_key?: string;
	created_at_label: string;
}

export interface WorkNodeDetailResponse {
	id: string;
	short_id: string;
	parent_run_id?: string;
	parent_short_id?: string;
	agent_id: string;
	session_id?: string;
	session_short_id?: string;
	session_url?: string;
	status: string;
	status_label: string;
	status_class: string;
	model_display: string;
	token_summary: string;
	token_exact_summary: string;
	started_at_label: string;
	last_activity_label: string;
	task: WorkStructuredTextResponse;
	output: WorkStructuredTextResponse;
	chain: WorkNodeChainResponse;
	approval?: WorkNodeApprovalResponse;
	logs?: WorkNodeLogEntryResponse[];
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
		dismissible: boolean;
		dismiss_url?: string;
	};
	graph: WorkGraphResponse;
	inspector_seed?: {
		id: string;
		agent_id: string;
		status: string;
	};
}

export interface WorkDismissResponse {
	dismissed: boolean;
	run_id: string;
	status: string;
	next_href: string;
}

export interface WorkCreateResponse {
	run_id: string;
	objective: string;
}

export interface LogEntryResponse {
	id: number;
	source: string;
	level: string;
	level_label: string;
	message: string;
	raw: string;
	created_at_label: string;
}

export interface LogsResponse {
	summary: {
		buffered_entries: number;
		visible_entries: number;
		error_entries: number;
		warning_entries: number;
	};
	filters: {
		query: string;
		level: string;
		source: string;
		limit: number;
	};
	sources: string[];
	stream_url: string;
	entries: LogEntryResponse[];
}

export interface UpdateStatusResponse {
	notice?: string;
	release: {
		version: string;
		commit: string;
		build_date: string;
		build_date_label: string;
	};
	runtime: {
		started_at: string;
		started_at_label: string;
		uptime_label: string;
		active_runs: number;
		interrupted_runs: number;
		pending_approvals: number;
	};
	install: {
		config_path: string;
		state_dir: string;
		database_dir: string;
		storage_root: string;
		binary_path: string;
		working_directory: string;
		service_unit_path: string;
	};
	service: {
		restart_policy: string;
		unit_preview: string;
	};
	storage: {
		database_bytes: number;
		wal_bytes: number;
		free_disk_bytes: number;
		backup_status: string;
		latest_backup_at_label: string;
		latest_backup_path: string;
		warnings: string[];
	};
	guides: {
		release_notes_url: string;
		ubuntu_doc_path: string;
		macos_doc_path: string;
		recovery_doc_path: string;
		changelog_path: string;
	};
}

export interface ExtensionSurfaceResponse {
	id: string;
	name: string;
	kind: string;
	configured: boolean;
	active: boolean;
	credential_state: string;
	credential_state_label: string;
	summary: string;
	detail: string;
}

export interface ExtensionToolResponse {
	name: string;
	family: string;
	risk: string;
	approval: string;
	side_effect: string;
	description: string;
}

export interface ExtensionStatusResponse {
	notice?: string;
	summary: {
		shipped_surfaces: number;
		configured_surfaces: number;
		installed_tools: number;
		ready_credentials: number;
		missing_credentials: number;
	};
	surfaces: ExtensionSurfaceResponse[];
	tools: ExtensionToolResponse[];
}

export interface NodeInventoryResponse {
	notice?: string;
	summary: {
		connectors: number;
		healthy_connectors: number;
		run_nodes: number;
		approval_nodes: number;
		capabilities: number;
	};
	connectors: Array<{
		id: string;
		aliases: string[];
		exposure: string;
		state: string;
		state_label: string;
		summary: string;
		checked_at_label: string;
		restart_suggested: boolean;
	}>;
	runs: Array<{
		id: string;
		short_id: string;
		parent_run_id: string;
		kind: string;
		agent_id: string;
		status: string;
		status_label: string;
		objective_preview: string;
		started_at_label: string;
		updated_at_label: string;
	}>;
	capabilities: Array<{
		name: string;
		family: string;
		description: string;
	}>;
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
	notice?: string;
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
		nextHref?: string;
		prevHref?: string;
	};
}

export interface PageLinksResponse {
	next_url?: string;
	prev_url?: string;
	has_next: boolean;
	has_prev: boolean;
}

export interface RecoverApprovalResponse {
	id: string;
	run_id: string;
	tool_name: string;
	binding_summary: string;
	status: string;
	status_label: string;
	status_class: string;
	resolved_by?: string;
	resolved_at_label?: string;
}

export interface ApprovalPolicyNodeResponse {
	agent_id: string;
	role: string;
	base_profile: string;
	is_front: boolean;
	tool_families: string[];
	delegation_kinds: string[];
	can_message: string[];
	allow_tools: string[];
	deny_tools: string[];
	pending_approvals: number;
	recent_runs: number;
	override_runs: number;
	observed_approval_mode: string;
	observed_approval_mode_label: string;
	observed_host_access_mode: string;
	observed_host_access_mode_label: string;
}

export interface ApprovalPolicyAllowlistResponse {
	agent_id: string;
	role: string;
	tool_name: string;
	direction: string;
	direction_label: string;
}

export interface ApprovalPolicyResponse {
	summary: {
		node_count: number;
		allowlist_count: number;
		pending_agents: number;
		override_agents: number;
	};
	gateway: {
		approval_mode: string;
		approval_mode_label: string;
		host_access_mode: string;
		host_access_mode_label: string;
		team_name: string;
		front_agent_id: string;
	};
	nodes: ApprovalPolicyNodeResponse[];
	allowlists: ApprovalPolicyAllowlistResponse[];
}

export interface RecoverRepairFiltersResponse {
	query: string;
	connector_id: string;
	route_status: string;
	delivery_status: string;
	active_limit: number;
	history_limit: number;
	delivery_limit: number;
}

export interface RecoverDeliveryHealthResponse {
	connector_id: string;
	pending_count: number;
	retrying_count: number;
	terminal_count: number;
	state_class: string;
}

export interface RecoverRuntimeHealthResponse {
	connector_id: string;
	state: string;
	state_label: string;
	state_class: string;
	summary: string;
	checked_at_label?: string;
	restart_suggested: boolean;
}

export interface RecoverRouteResponse {
	id: string;
	connector_id: string;
	external_id: string;
	thread_id: string;
	session_id: string;
	conversation_id: string;
	agent_id: string;
	role_label: string;
	status_label: string;
	deactivated_label?: string;
	deactivation_note?: string;
	replaced_by_route_id?: string;
}

export interface RecoverDeliveryResponse {
	id: string;
	run_id: string;
	session_id: string;
	connector_id: string;
	chat_id: string;
	message: {
		plain_text: string;
		html: string;
	};
	status: string;
	status_label: string;
	attempts_label: string;
}

export interface RecoverResponse {
	summary: {
		open_approvals: number;
		pending_approvals: number;
		connector_count: number;
		active_routes: number;
		terminal_deliveries: number;
	};
	approvals: RecoverApprovalResponse[];
	approval_paging: PageLinksResponse;
	repair: {
		connector_count: number;
		filters: RecoverRepairFiltersResponse;
		health: RecoverDeliveryHealthResponse[];
		runtime_connectors: RecoverRuntimeHealthResponse[];
		active_routes: RecoverRouteResponse[];
		active_paging: PageLinksResponse;
		route_history: RecoverRouteResponse[];
		history_paging: PageLinksResponse;
		deliveries: RecoverDeliveryResponse[];
		delivery_paging: PageLinksResponse;
	};
}

export interface ConversationIndexItemResponse {
	id: string;
	conversation_id: string;
	agent_id: string;
	role_label: string;
	status_label: string;
	updated_at_label: string;
}

export interface ConversationsResponse {
	summary: {
		session_count: number;
		connector_count: number;
		terminal_deliveries: number;
	};
	filters: {
		query: string;
		agent_id: string;
		role: string;
		status: string;
		connector_id: string;
		binding: string;
	};
	sessions: ConversationIndexItemResponse[];
	paging: PageLinksResponse;
	health: RecoverDeliveryHealthResponse[];
	runtime_connectors: RecoverRuntimeHealthResponse[];
}

export interface ConversationDeliveryQueueItemResponse {
	id: string;
	run_id: string;
	session_id: string;
	conversation_id: string;
	connector_id: string;
	chat_id: string;
	status: string;
	status_label: string;
	attempts_label: string;
	message_preview: string;
}

export interface ConversationDeliveryQueueResponse {
	filters: {
		query: string;
		status: string;
		limit: number;
	};
	items: ConversationDeliveryQueueItemResponse[];
	paging: {
		has_next: boolean;
		has_prev: boolean;
		nextHref?: string;
		prevHref?: string;
	};
}

export interface AutomateScheduleResponse {
	id: string;
	name: string;
	objective: string;
	kind: string;
	kind_label: string;
	cadence_label: string;
	enabled: boolean;
	enabled_label: string;
	status_label: string;
	status_class: string;
	next_run_at_label: string;
	last_run_at_label: string;
	last_error: string;
	project_id: string;
	cwd: string;
	consecutive_failures: number;
	schedule_error_count: number;
}

export interface AutomateOccurrenceResponse {
	id: string;
	schedule_id: string;
	schedule_name: string;
	status: string;
	status_label: string;
	status_class: string;
	slot_at_label: string;
	updated_at_label: string;
	run_id?: string;
	conversation_id?: string;
	error?: string;
	skip_reason?: string;
}

export interface AutomateResponse {
	summary: {
		total_schedules: number;
		enabled_schedules: number;
		due_schedules: number;
		active_occurrences: number;
		next_wake_at_label: string;
	};
	health: {
		invalid_schedules: number;
		stuck_dispatching: number;
		missing_next_run: number;
	};
	schedules: AutomateScheduleResponse[];
	open_occurrences: AutomateOccurrenceResponse[];
	recent_occurrences: AutomateOccurrenceResponse[];
}

export interface HistoryApprovalResponse {
	id: string;
	run_id: string;
	tool_name: string;
	status: string;
	status_label: string;
	resolved_by: string;
	resolved_at_label: string;
}

export interface HistoryDeliveryResponse {
	id: string;
	run_id: string;
	connector_id: string;
	chat_id: string;
	status: string;
	status_label: string;
	attempts_label: string;
	last_attempt_at_label: string;
	message_preview: string;
}

export interface HistoryResponse {
	summary: {
		run_count: number;
		completed_runs: number;
		recovery_runs: number;
		approval_events: number;
		delivery_outcomes: number;
	};
	filters: {
		query: string;
		status: string;
		scope: string;
		limit: number;
	};
	paging: PageLinksResponse;
	runs: WorkClusterResponse[];
	approvals: HistoryApprovalResponse[];
	deliveries: HistoryDeliveryResponse[];
}

export interface SettingsDeviceResponse {
	id: string;
	primary_label: string;
	secondary_line: string;
	current: boolean;
	blocked: boolean;
	active_sessions: number;
	details_ip: string;
	details_user_agent: string;
}

export interface SettingsMachineResponse {
	storage_root: string;
	approval_mode: string;
	approval_mode_label: string;
	host_access_mode: string;
	host_access_mode_label: string;
	admin_token: string;
	per_run_token_budget: string;
	daily_cost_cap_usd: string;
	rolling_cost_usd: number;
	rolling_cost_label: string;
	telegram_token: string;
	active_project_name: string;
	active_project_path: string;
	active_project_summary: string;
}

export interface SettingsResponse {
	machine: SettingsMachineResponse;
	access: {
		password_configured: boolean;
		current_device?: SettingsDeviceResponse;
		other_active_devices: SettingsDeviceResponse[];
		blocked_devices: SettingsDeviceResponse[];
	};
}

export interface SettingsActionResponse {
	notice?: string;
	logged_out?: boolean;
	next?: string;
	settings?: SettingsResponse;
}

export interface ConversationDetailResponse {
	session: {
		id: string;
		agent_id: string;
		role_label: string;
		status_label: string;
	};
	messages: Array<{
		kind: string;
		kind_label: string;
		body: {
			plain_text: string;
			html: string;
		};
		sender_label: string;
		sender_is_mono: boolean;
		source_run_id?: string;
	}>;
	route?: {
		id: string;
		connector_id: string;
		external_id: string;
		thread_id: string;
		status_label: string;
		created_at_label: string;
		deactivated_label?: string;
	};
	active_run_id?: string;
	deliveries: Array<{
		id: string;
		connector_id: string;
		chat_id: string;
		message: {
			plain_text: string;
			html: string;
		};
		status: string;
		status_label: string;
		attempts_label: string;
	}>;
	delivery_failures: Array<{
		id: string;
		connector_id: string;
		chat_id: string;
		event_kind_label: string;
		error: string;
		created_at_label: string;
	}>;
}

export interface DebugRPCProbeResponse {
	name: string;
	label: string;
	description: string;
}

export interface DebugRPCResultResponse {
	probe: string;
	label: string;
	summary: string;
	executed_at: string;
	executed_at_label: string;
	data?: Record<string, unknown>;
}

export interface DebugRPCStatusResponse {
	notice?: string;
	summary: {
		probe_count: number;
		read_only: boolean;
		default_probe: string;
		selected_probe: string;
	};
	probes: DebugRPCProbeResponse[];
	result: DebugRPCResultResponse;
}

export interface DebugEventsSourceResponse {
	run_id: string;
	objective: string;
	agent_id: string;
	status: string;
	status_label: string;
	event_count: number;
	latest_event_at_label: string;
	stream_url: string;
}

export interface DebugEventsEntryResponse {
	id: string;
	run_id: string;
	run_short_id: string;
	objective: string;
	agent_id: string;
	kind: string;
	kind_label: string;
	payload_preview: string;
	occurred_at: string;
	occurred_at_label: string;
}

export interface DebugEventsResponse {
	summary: {
		source_count: number;
		event_count: number;
		selected_run_id?: string;
		latest_event_label?: string;
		latest_event_at_label?: string;
	};
	filters: {
		run_id?: string;
		limit: number;
	};
	sources: DebugEventsSourceResponse[];
	events: DebugEventsEntryResponse[];
}

<script lang="ts">
	import { browser } from '$app/environment';
	import { resolve } from '$app/paths';
	import SurfaceMetricCard from '$lib/components/common/SurfaceMetricCard.svelte';
	import DeviceAccessCard from '$lib/components/common/DeviceAccessCard.svelte';
	import SettingsField from '$lib/components/config/SettingsField.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import { requestJSON } from '$lib/http/client';
	import {
		cloneTeamProfile,
		createTeamProfile,
		deleteTeamProfile,
		importTeamYAML,
		saveTeamConfig,
		selectTeamProfile
	} from '$lib/team/actions';
	import type {
		SettingsActionResponse,
		SettingsDeviceResponse,
		SettingsResponse,
		TeamResponse
	} from '$lib/types/api';
	import { summarizeModelUsage } from '$lib/work/models';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	type TabID = 'general' | 'agents' | 'models' | 'channels' | 'raw' | 'apply';

	const tabs: Array<{ id: TabID; label: string }> = [
		{ id: 'general', label: 'General' },
		{ id: 'agents', label: 'Agents & Routing' },
		{ id: 'models', label: 'Models' },
		{ id: 'channels', label: 'Channels' },
		{ id: 'raw', label: 'Raw JSON5' },
		{ id: 'apply', label: 'Apply' }
	];

	const approvalModeOptions = [
		{ value: 'prompt', label: 'Prompt' },
		{ value: 'auto_approve', label: 'Auto approve' }
	];

	const hostAccessModeOptions = [
		{ value: 'standard', label: 'Standard' },
		{ value: 'elevated', label: 'Elevated' }
	];

	const knowledgeScopeOptions = [
		{ value: '', label: 'All scopes' },
		{ value: 'local', label: 'Local' },
		{ value: 'team', label: 'Team' }
	];

	let activeTabOverride = $state<TabID | null>(null);
	let savedSettings = $state<SettingsResponse | null>(null);
	let savedTeam = $state<TeamResponse | null>(null);
	let approvalMode = $state('');
	let hostAccessMode = $state('');
	let perRunTokenBudget = $state('');
	let dailyCostCapUSD = $state('');
	let telegramBotToken = $state('');
	let lastMachineSignature = $state('');
	let lastTeamSignature = $state('');
	let saving = $state(false);
	let passwordSaving = $state(false);
	let teamSaving = $state(false);
	let deviceMutationID = $state<string | null>(null);
	let saveMessage = $state('');
	let saveError = $state('');
	let rawEditorEl = $state<HTMLDivElement | null>(null);
	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');
	let selectedProfileID = $state('');
	let createProfileID = $state('');
	let cloneSourceProfileID = $state('');
	let cloneProfileID = $state('');
	let deleteProfileID = $state('');
	let importYAML = $state('');

	function isTabID(value: string | null): value is TabID {
		return (
			value === 'general' ||
			value === 'agents' ||
			value === 'models' ||
			value === 'channels' ||
			value === 'raw' ||
			value === 'apply'
		);
	}

	function setActiveTab(id: string): void {
		if (isTabID(id)) {
			activeTabOverride = id;
		}
	}

	function listOrNone(values: string[]): string {
		return values.length > 0 ? values.join(', ') : 'None';
	}

	function formatKnowledgeScope(scope: string): string {
		if (scope === 'local') {
			return 'Local';
		}

		if (scope === 'team') {
			return 'Team';
		}

		return scope.trim() === '' ? 'Project' : scope;
	}

	function formatKnowledgeConfidence(confidence: number): string {
		const normalized = Number.isFinite(confidence) ? confidence : 0;
		return `${Math.round(normalized * 100)}% confidence`;
	}

	const requestedTab = $derived.by<TabID>(() => {
		const tab = new URLSearchParams(data.currentSearch).get('tab');
		return isTabID(tab) ? tab : 'general';
	});
	const activeTab = $derived(activeTabOverride ?? requestedTab);
	const settings = $derived(savedSettings ?? data.config?.settings ?? null);
	const machine = $derived(settings?.machine ?? null);
	const access = $derived(settings?.access ?? null);
	const rawDocument = $derived(JSON.stringify(settings ?? {}, null, 2));
	const teamConfig = $derived(savedTeam ?? data.config?.team ?? null);
	const team = $derived(teamConfig?.team ?? null);
	const activeProfile = $derived(teamConfig?.active_profile ?? null);
	const profiles = $derived(teamConfig?.profiles ?? []);
	const inactiveProfiles = $derived(profiles.filter((profile) => !profile.active));
	const members = $derived(team?.members ?? []);
	const work = $derived(data.config?.work ?? null);
	const knowledge = $derived(data.config?.knowledge ?? null);
	const modelUsage = $derived(summarizeModelUsage(work?.clusters));
	const currentDevice = $derived(access?.current_device ?? null);
	const otherActiveDevices = $derived(access?.other_active_devices ?? []);
	const blockedDevices = $derived(access?.blocked_devices ?? []);
	const frontAgent = $derived.by(() => {
		if (!team) {
			return null;
		}

		return (
			members.find((member) => member.id === team.front_agent_id) ??
			members.find((member) => member.is_front) ??
			null
		);
	});

	$effect(() => {
		const nextSignature = machine
			? [
					machine.approval_mode,
					machine.host_access_mode,
					machine.per_run_token_budget,
					machine.daily_cost_cap_usd,
					machine.telegram_token
				].join('|')
			: '';

		if (nextSignature === lastMachineSignature) {
			return;
		}

		lastMachineSignature = nextSignature;
		approvalMode = machine?.approval_mode ?? approvalModeOptions[0].value;
		hostAccessMode = machine?.host_access_mode ?? hostAccessModeOptions[0].value;
		perRunTokenBudget = machine?.per_run_token_budget ?? '';
		dailyCostCapUSD = machine?.daily_cost_cap_usd ?? '';
		telegramBotToken = '';
	});

	$effect(() => {
		const nextSignature = [
			activeProfile?.id ?? '',
			profiles.map((profile) => `${profile.id}:${profile.active ? '1' : '0'}`).join('|')
		].join('|');

		if (nextSignature === lastTeamSignature) {
			return;
		}

		lastTeamSignature = nextSignature;
		selectedProfileID = activeProfile?.id ?? profiles[0]?.id ?? '';
		cloneSourceProfileID = activeProfile?.id ?? profiles[0]?.id ?? '';
		deleteProfileID = inactiveProfiles[0]?.id ?? '';
	});

	$effect(() => {
		if (!browser || activeTab !== 'raw' || !rawEditorEl) {
			return;
		}

		let cancelled = false;
		let editorView: { destroy(): void } | null = null;
		const doc = rawDocument;

		void (async () => {
			const [{ EditorView, basicSetup }, { json }] = await Promise.all([
				import('codemirror'),
				import('@codemirror/lang-json')
			]);

			if (cancelled || !rawEditorEl) {
				return;
			}

			editorView = new EditorView({
				doc,
				extensions: [basicSetup, json(), EditorView.editable.of(false)],
				parent: rawEditorEl
			});
		})();

		return () => {
			cancelled = true;
			editorView?.destroy();
		};
	});

	async function handleSaveGeneral(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		saveMessage = '';
		saveError = '';
		saving = true;

		try {
			const response = await requestJSON<SettingsActionResponse>(
				globalThis.fetch.bind(globalThis),
				'/api/settings',
				{
					method: 'POST',
					headers: {
						'content-type': 'application/json'
					},
					body: JSON.stringify({
						approval_mode: approvalMode,
						host_access_mode: hostAccessMode,
						per_run_token_budget: perRunTokenBudget,
						daily_cost_cap_usd: dailyCostCapUSD,
						telegram_bot_token: telegramBotToken
					})
				}
			);

			savedSettings = response.settings ?? settings;
			saveMessage = response.notice ?? 'Machine settings updated.';
			telegramBotToken = '';
		} catch {
			saveError = 'Failed to save settings.';
		} finally {
			saving = false;
		}
	}

	async function handlePasswordChange(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		saveMessage = '';
		saveError = '';
		passwordSaving = true;

		try {
			const response = await requestJSON<SettingsActionResponse>(
				globalThis.fetch.bind(globalThis),
				'/api/settings/password',
				{
					method: 'POST',
					headers: {
						'content-type': 'application/json'
					},
					body: JSON.stringify({
						current_password: currentPassword,
						new_password: newPassword,
						confirm_password: confirmPassword
					})
				}
			);

			savedSettings = response.settings ?? settings;
			saveMessage = response.notice ?? 'Password updated.';
			currentPassword = '';
			newPassword = '';
			confirmPassword = '';

			if (browser && response.logged_out && response.next) {
				globalThis.location.assign(response.next);
			}
		} catch {
			saveError = 'Failed to update password.';
		} finally {
			passwordSaving = false;
		}
	}

	async function mutateDevice(
		device: SettingsDeviceResponse,
		action: 'revoke' | 'block' | 'unblock'
	): Promise<void> {
		saveMessage = '';
		saveError = '';
		deviceMutationID = device.id;

		try {
			const response = await requestJSON<SettingsActionResponse>(
				globalThis.fetch.bind(globalThis),
				`/api/settings/devices/${encodeURIComponent(device.id)}/${action}`,
				{
					method: 'POST'
				}
			);

			savedSettings = response.settings ?? settings;
			saveMessage = response.notice ?? 'Browser access updated.';

			if (browser && response.logged_out && response.next) {
				globalThis.location.assign(response.next);
			}
		} catch {
			saveError =
				action === 'revoke'
					? 'Failed to revoke device.'
					: action === 'block'
						? 'Failed to block device.'
						: 'Failed to unblock device.';
		} finally {
			deviceMutationID = null;
		}
	}

	function setTeamMutationResult(response: TeamResponse | null, fallbackMessage: string): void {
		savedTeam = response;
		saveMessage = response?.notice ?? fallbackMessage;
		saveError = '';
	}

	function mutationErrorMessage(action: string, err: unknown): string {
		if (err instanceof Error && err.message.trim() !== '') {
			return err.message;
		}

		return `Failed to ${action}.`;
	}

	async function handleSwitchProfile(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		saveMessage = '';
		saveError = '';
		teamSaving = true;

		try {
			const response = await selectTeamProfile(
				globalThis.fetch.bind(globalThis),
				selectedProfileID
			);
			setTeamMutationResult(response, `Active profile switched to ${selectedProfileID}.`);
		} catch (err) {
			saveError = mutationErrorMessage('switch profile', err);
		} finally {
			teamSaving = false;
		}
	}

	async function handleCreateProfile(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		saveMessage = '';
		saveError = '';
		teamSaving = true;

		try {
			const nextProfileID = createProfileID.trim();
			const response = await createTeamProfile(globalThis.fetch.bind(globalThis), nextProfileID);
			setTeamMutationResult(response, `Profile ${nextProfileID} created.`);
			createProfileID = '';
		} catch (err) {
			saveError = mutationErrorMessage('create profile', err);
		} finally {
			teamSaving = false;
		}
	}

	async function handleCloneProfile(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		saveMessage = '';
		saveError = '';
		teamSaving = true;

		try {
			const nextProfileID = cloneProfileID.trim();
			const response = await cloneTeamProfile(
				globalThis.fetch.bind(globalThis),
				cloneSourceProfileID,
				nextProfileID
			);
			setTeamMutationResult(response, `Profile ${nextProfileID} cloned.`);
			cloneProfileID = '';
		} catch (err) {
			saveError = mutationErrorMessage('clone profile', err);
		} finally {
			teamSaving = false;
		}
	}

	async function handleDeleteProfile(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		saveMessage = '';
		saveError = '';
		teamSaving = true;

		try {
			const response = await deleteTeamProfile(globalThis.fetch.bind(globalThis), deleteProfileID);
			setTeamMutationResult(response, `Profile ${deleteProfileID} deleted.`);
		} catch (err) {
			saveError = mutationErrorMessage('delete profile', err);
		} finally {
			teamSaving = false;
		}
	}

	async function handleImportTeam(): Promise<void> {
		saveMessage = '';
		saveError = '';
		teamSaving = true;

		try {
			const response = await importTeamYAML(globalThis.fetch.bind(globalThis), importYAML.trim());
			setTeamMutationResult(response, 'Imported file loaded. Save Team to apply the change.');
		} catch (err) {
			saveError = mutationErrorMessage('import team file', err);
		} finally {
			teamSaving = false;
		}
	}

	async function handleSaveTeam(): Promise<void> {
		if (!team) {
			saveError = 'Failed to save team.';
			return;
		}

		saveMessage = '';
		saveError = '';
		teamSaving = true;

		try {
			const response = await saveTeamConfig(globalThis.fetch.bind(globalThis), team);
			setTeamMutationResult(response, 'Team saved.');
		} catch (err) {
			saveError = mutationErrorMessage('save team', err);
		} finally {
			teamSaving = false;
		}
	}
</script>

<svelte:head>
	<title>Config | GistClaw</title>
</svelte:head>

<div class="flex h-full flex-col overflow-hidden">
	<div
		class="shrink-0 border-b border-b-[1.5px] border-[var(--gc-border-strong)] bg-[var(--gc-surface)]"
	>
		<div class="px-6 pt-4 pb-0">
			<h1 class="gc-panel-title text-[var(--gc-ink)]">Config</h1>
		</div>
		<SectionTabs {tabs} {activeTab} onchange={setActiveTab} />
	</div>

	<div class="flex min-h-0 flex-1 flex-col overflow-y-auto">
		{#if !machine}
			<div class="border-b border-[var(--gc-border)] px-5 py-4">
				<SurfaceMessage
					label="LOAD FAILED"
					message="Failed to load settings. Please reload."
					tone="error"
				/>
			</div>
		{/if}

		{#if saveMessage}
			<div class="border-b border-[var(--gc-border)] px-5 py-4">
				<SurfaceMessage label="UPDATED" message={saveMessage} />
			</div>
		{/if}

		{#if saveError}
			<div class="border-b border-[var(--gc-border)] px-5 py-4">
				<SurfaceMessage label="SAVE FAILED" message={saveError} tone="error" />
			</div>
		{/if}

		{#if activeTab === 'general'}
			<div class="mx-auto w-full max-w-6xl px-6 py-6">
				<div class="flex flex-col gap-6 lg:flex-row">
					<div class="flex-1">
						<p class="gc-stamp text-[var(--gc-ink-3)]">GENERAL</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Machine settings</h2>
						<p class="gc-copy mt-3 max-w-xl text-[var(--gc-ink-2)]">
							Core approval, host access, and budget settings. This mirrors the gateway settings
							surface in OpenClaw, but scoped to the backend fields GistClaw exposes today.
						</p>

						{#if machine}
							<form class="mt-6 flex max-w-xl flex-col gap-5" onsubmit={handleSaveGeneral}>
								<SettingsField
									id="approval-mode"
									label="Approval Mode"
									type="select"
									bind:value={approvalMode}
									options={approvalModeOptions}
									hint="Choose whether exec requests stop for approval or auto-approve."
								/>
								<SettingsField
									id="host-access-mode"
									label="Host Access Mode"
									type="select"
									bind:value={hostAccessMode}
									options={hostAccessModeOptions}
									hint="Standard keeps tool execution constrained. Elevated unlocks wider host access."
								/>
								<SettingsField
									id="token-budget"
									label="Per-Run Token Budget"
									bind:value={perRunTokenBudget}
									placeholder="50000"
									hint="Maximum tokens allowed for a single run."
								/>
								<SettingsField
									id="daily-cost-cap"
									label="Daily Cost Cap (USD)"
									bind:value={dailyCostCapUSD}
									placeholder="5.00"
									hint="Stop new work when the gateway hits this daily cost ceiling."
								/>
								<SettingsField
									id="telegram-token"
									label="Telegram Bot Token"
									type="password"
									bind:value={telegramBotToken}
									placeholder="Leave blank to keep the current token"
									hint="This writes the masked telegram bot token field exposed by /api/settings."
								/>
								<div class="flex justify-end">
									<button
										type="submit"
										disabled={saving}
										class="gc-action gc-action-warning px-4 py-2 disabled:opacity-50"
									>
										{saving ? 'SAVING…' : 'SAVE'}
									</button>
								</div>
							</form>
						{/if}
					</div>

					<div class="w-full shrink-0 lg:max-w-sm">
						<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-1">
							<section class="gc-panel-soft px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Rolling Cost</p>
								<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">
									{machine?.rolling_cost_label ?? '—'}
								</p>
								<p class="gc-copy mt-3 text-[var(--gc-ink-3)]">
									Current tracked spend for the active billing window.
								</p>
							</section>

							<section class="gc-panel-soft px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Active Project</p>
								<p class="gc-copy mt-3 text-[var(--gc-ink)]">
									{machine?.active_project_name ?? '—'}
								</p>
								<p class="gc-copy mt-2 font-mono text-sm text-[var(--gc-ink-3)]">
									{machine?.active_project_path ?? '—'}
								</p>
								<p class="gc-copy mt-3 text-[var(--gc-ink-3)]">
									{machine?.active_project_summary ?? 'No project summary'}
								</p>
							</section>

							<section class="gc-panel-soft px-4 py-4 sm:col-span-2 lg:col-span-1">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Admin Token</p>
								<p class="gc-copy mt-3 font-mono text-sm text-[var(--gc-ink-2)]">
									{machine?.admin_token ?? '—'}
								</p>
								<p class="gc-copy mt-3 text-[var(--gc-ink-3)]">
									Masked in the API response. Use the login flow to rotate it if needed.
								</p>
							</section>
						</div>
					</div>
				</div>

				<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">BROWSER ACCESS</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Browser access</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							The current runtime already exposes the full browser access board through
							`/api/settings`. Use it to see which browsers are signed in, revoke stale sessions,
							and unblock a device after review.
						</p>

						<div class="mt-5 grid gap-4 xl:grid-cols-2">
							{#if currentDevice}
								<DeviceAccessCard label="Current Browser" device={currentDevice} />
							{/if}

							{#each otherActiveDevices as device (device.id)}
								<DeviceAccessCard
									label="Signed In Browser"
									{device}
									busy={deviceMutationID === device.id}
									onrevoke={() => {
										void mutateDevice(device, 'revoke');
									}}
									onblock={() => {
										void mutateDevice(device, 'block');
									}}
								/>
							{/each}

							{#each blockedDevices as device (device.id)}
								<DeviceAccessCard
									label="Blocked Browser"
									{device}
									busy={deviceMutationID === device.id}
									onunblock={() => {
										void mutateDevice(device, 'unblock');
									}}
								/>
							{/each}
						</div>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">PASSWORD</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
							{access?.password_configured ? 'Password set' : 'Password required'}
						</h2>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							Changing the browser password signs out other active browsers. Keep the current
							password handy if you are rotating it from this session.
						</p>

						<form class="mt-5 flex flex-col gap-4" onsubmit={handlePasswordChange}>
							<SettingsField
								id="current-password"
								label="Current Password"
								type="password"
								bind:value={currentPassword}
								placeholder="Current browser password"
							/>
							<SettingsField
								id="new-password"
								label="New Password"
								type="password"
								bind:value={newPassword}
								placeholder="New browser password"
							/>
							<SettingsField
								id="confirm-password"
								label="Confirm Password"
								type="password"
								bind:value={confirmPassword}
								placeholder="Repeat the new password"
							/>
							<div class="flex justify-end">
								<button
									type="submit"
									disabled={passwordSaving}
									class="gc-action px-4 py-2 disabled:opacity-50"
								>
									{passwordSaving ? 'UPDATING…' : 'UPDATE PASSWORD'}
								</button>
							</div>
						</form>
					</section>
				</div>

				<div class="mt-6">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">SAVED KNOWLEDGE</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Saved knowledge</h2>

						{#if knowledge}
							<p class="gc-copy mt-3 max-w-3xl text-[var(--gc-ink-2)]">
								{knowledge.headline}
							</p>

							<form method="GET" class="mt-5 grid gap-4 xl:grid-cols-4">
								<input type="hidden" name="tab" value="general" />

								<div class="flex flex-col gap-2">
									<label for="knowledge-search" class="gc-stamp text-[var(--gc-ink-3)]">
										Search knowledge
									</label>
									<input
										id="knowledge-search"
										name="knowledge_q"
										value={knowledge.filters.query}
										placeholder="Search saved knowledge"
										class="gc-control min-h-[2.75rem]"
									/>
								</div>

								<div class="flex flex-col gap-2">
									<label for="knowledge-scope" class="gc-stamp text-[var(--gc-ink-3)]">
										Knowledge scope
									</label>
									<select
										id="knowledge-scope"
										name="knowledge_scope"
										class="gc-control min-h-[2.75rem]"
									>
										{#each knowledgeScopeOptions as option (option.value)}
											<option
												value={option.value}
												selected={knowledge.filters.scope === option.value}
											>
												{option.label}
											</option>
										{/each}
									</select>
								</div>

								<div class="flex flex-col gap-2">
									<label for="knowledge-agent" class="gc-stamp text-[var(--gc-ink-3)]">
										Agent
									</label>
									<input
										id="knowledge-agent"
										name="knowledge_agent_id"
										value={knowledge.filters.agent_id}
										placeholder="assistant"
										class="gc-control min-h-[2.75rem]"
									/>
								</div>

								<div class="flex flex-col gap-2">
									<label for="knowledge-limit" class="gc-stamp text-[var(--gc-ink-3)]">
										Knowledge limit
									</label>
									<input
										id="knowledge-limit"
										type="number"
										min="1"
										max="100"
										name="knowledge_limit"
										value={String(knowledge.filters.limit)}
										class="gc-control min-h-[2.75rem]"
									/>
								</div>

								<div class="flex flex-wrap justify-end gap-3 xl:col-span-4">
									<a
										href={resolve('/config?tab=general')}
										class="gc-action gc-action-accent px-4 py-2"
									>
										Clear filters
									</a>
									<button type="submit" class="gc-action gc-action-solid px-4 py-2">
										Apply filters
									</button>
								</div>
							</form>

							<div class="mt-5 grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(16rem,0.85fr)]">
								<div class="grid gap-3">
									{#if knowledge.items.length > 0}
										{#each knowledge.items as item (item.id)}
											<article class="border border-[var(--gc-border)] px-4 py-4">
												<div class="flex flex-wrap items-start justify-between gap-3">
													<div>
														<p class="gc-copy text-[var(--gc-ink)]">{item.content}</p>
														<p class="gc-copy mt-2 text-sm text-[var(--gc-ink-3)]">
															{item.provenance}
														</p>
													</div>
													<div class="flex flex-wrap gap-2 text-xs text-[var(--gc-ink-3)]">
														<span class="gc-chip">{formatKnowledgeScope(item.scope)}</span>
														<span class="gc-chip">{item.agent_id}</span>
														<span class="gc-chip">{item.source}</span>
													</div>
												</div>

												<div class="mt-4 flex flex-wrap gap-x-5 gap-y-2">
													<p class="gc-copy text-sm text-[var(--gc-ink-3)]">
														{formatKnowledgeConfidence(item.confidence)}
													</p>
													<p class="gc-copy text-sm text-[var(--gc-ink-3)]">
														Created {item.created_at_label}
													</p>
													<p class="gc-copy text-sm text-[var(--gc-ink-3)]">
														Updated {item.updated_at_label}
													</p>
												</div>
											</article>
										{/each}
									{:else}
										<div class="border border-dashed border-[var(--gc-border)] px-4 py-5">
											<p class="gc-copy text-[var(--gc-ink)]">
												No saved knowledge matched the current filters.
											</p>
										</div>
									{/if}
								</div>

								<div class="flex flex-col gap-4">
									<section class="border border-[var(--gc-border)] px-4 py-4">
										<p class="gc-stamp text-[var(--gc-ink-3)]">Visible items</p>
										<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">
											{knowledge.summary.visible_count}
										</p>
										<p class="gc-copy mt-3 text-[var(--gc-ink-3)]">
											Knowledge entries currently shaping this project view.
										</p>
									</section>

									<section class="border border-[var(--gc-border)] px-4 py-4">
										<p class="gc-stamp text-[var(--gc-ink-3)]">Page controls</p>
										<div class="mt-4 flex flex-wrap gap-3">
											{#if knowledge.paging.prev_cursor}
												<form method="GET" action={resolve('/config')}>
													<input type="hidden" name="tab" value="general" />
													<input type="hidden" name="knowledge_q" value={knowledge.filters.query} />
													<input
														type="hidden"
														name="knowledge_scope"
														value={knowledge.filters.scope}
													/>
													<input
														type="hidden"
														name="knowledge_agent_id"
														value={knowledge.filters.agent_id}
													/>
													<input
														type="hidden"
														name="knowledge_limit"
														value={String(knowledge.filters.limit)}
													/>
													<input
														type="hidden"
														name="knowledge_cursor"
														value={knowledge.paging.prev_cursor}
													/>
													<input type="hidden" name="knowledge_direction" value="prev" />
													<button type="submit" class="gc-action gc-action-accent px-4 py-2">
														Previous knowledge page
													</button>
												</form>
											{/if}
											{#if knowledge.paging.next_cursor}
												<form method="GET" action={resolve('/config')}>
													<input type="hidden" name="tab" value="general" />
													<input type="hidden" name="knowledge_q" value={knowledge.filters.query} />
													<input
														type="hidden"
														name="knowledge_scope"
														value={knowledge.filters.scope}
													/>
													<input
														type="hidden"
														name="knowledge_agent_id"
														value={knowledge.filters.agent_id}
													/>
													<input
														type="hidden"
														name="knowledge_limit"
														value={String(knowledge.filters.limit)}
													/>
													<input
														type="hidden"
														name="knowledge_cursor"
														value={knowledge.paging.next_cursor}
													/>
													<input type="hidden" name="knowledge_direction" value="next" />
													<button type="submit" class="gc-action gc-action-solid px-4 py-2">
														Next knowledge page
													</button>
												</form>
											{/if}
											{#if !knowledge.paging.prev_cursor && !knowledge.paging.next_cursor}
												<p class="gc-copy text-sm text-[var(--gc-ink-3)]">
													No additional knowledge pages are available.
												</p>
											{/if}
										</div>
									</section>
								</div>
							</div>
						{:else}
							<div class="mt-5">
								<SurfaceMessage
									label="UNAVAILABLE"
									message="Knowledge surface unavailable. The current browser UI expects /api/knowledge."
									tone="error"
								/>
							</div>
						{/if}
					</section>
				</div>
			</div>
		{:else if activeTab === 'agents'}
			<div class="mx-auto w-full max-w-6xl px-6 py-6">
				{#if team}
					{#if teamConfig?.notice}
						<div class="mb-6">
							<SurfaceMessage label="TEAM FILE" message={teamConfig.notice} />
						</div>
					{/if}

					<div class="grid gap-4 xl:grid-cols-4">
						<SurfaceMetricCard
							label="Team"
							value={team.name}
							detail={`Front agent ${frontAgent?.id ?? team.front_agent_id}`}
							tone="accent"
						/>
						<SurfaceMetricCard
							label="Members"
							value={String(team.member_count)}
							detail={`${members.length} runtime role${members.length === 1 ? '' : 's'} exposed by /api/team.`}
						/>
						<SurfaceMetricCard
							label="Front Agent"
							value={frontAgent?.id ?? team.front_agent_id}
							detail={frontAgent?.role ?? 'Front role not described'}
							tone="accent"
						/>
						<SurfaceMetricCard
							label="Active Profile"
							value={activeProfile?.label ?? 'None'}
							detail={activeProfile?.save_path ?? 'Profile save path not exposed'}
						/>
					</div>

					<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
						<section class="gc-panel-soft px-5 py-5">
							<p class="gc-stamp text-[var(--gc-ink-3)]">ROUTING</p>
							<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
								Route work through the front agent
							</h2>
							<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
								This follows OpenClaw's operator flow, but only shows the team and routing facts
								GistClaw actually ships through `/api/team`.
							</p>

							<div class="mt-5 grid gap-3 md:grid-cols-2">
								<div class="border border-[var(--gc-border)] px-4 py-4">
									<p class="gc-stamp text-[var(--gc-ink-3)]">Front Agent</p>
									<p class="gc-copy mt-2 text-[var(--gc-ink)]">
										{frontAgent?.id ?? team.front_agent_id}
									</p>
									<p class="gc-copy mt-2 text-[var(--gc-ink-2)]">
										{frontAgent?.role ?? 'No front role description'}
									</p>
								</div>

								<div class="border border-[var(--gc-border)] px-4 py-4">
									<p class="gc-stamp text-[var(--gc-ink-3)]">Base Profile</p>
									<p class="gc-copy mt-2 text-[var(--gc-ink)]">
										{frontAgent?.base_profile ?? activeProfile?.label ?? 'None'}
									</p>
									<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
										Front agent defaults before specialist handoff.
									</p>
								</div>

								<div class="border border-[var(--gc-border)] px-4 py-4">
									<p class="gc-stamp text-[var(--gc-ink-3)]">Delegation</p>
									<p class="gc-copy mt-2 text-[var(--gc-ink)]">
										{listOrNone(frontAgent?.delegation_kinds ?? [])}
									</p>
									<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
										Specialist lanes the front agent may hand work to directly.
									</p>
								</div>

								<div class="border border-[var(--gc-border)] px-4 py-4">
									<p class="gc-stamp text-[var(--gc-ink-3)]">Can Message</p>
									<p class="gc-copy mt-2 text-[var(--gc-ink)]">
										{listOrNone(frontAgent?.can_message ?? [])}
									</p>
									<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
										Direct peers the front agent is allowed to wake or consult.
									</p>
								</div>
							</div>
						</section>

						<div class="flex flex-col gap-4">
							<section class="gc-panel-soft px-5 py-5">
								<p class="gc-stamp text-[var(--gc-ink-3)]">PROFILES</p>
								<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Saved profiles</h2>
								<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
									Profile defaults still live in checked-in runtime files, but the active selection
									and lifecycle controls are live here.
								</p>

								<div class="mt-5 grid gap-3">
									{#each profiles as profile (profile.id)}
										<div class="border border-[var(--gc-border)] px-4 py-4">
											<div class="flex items-start justify-between gap-3">
												<div>
													<p class="gc-copy text-[var(--gc-ink)]">{profile.label}</p>
													<p class="gc-copy mt-2 font-mono text-sm text-[var(--gc-ink-3)]">
														{profile.save_path ?? 'Runtime default'}
													</p>
												</div>
												{#if profile.active}
													<span class="gc-stamp text-[var(--gc-primary)]">ACTIVE</span>
												{/if}
											</div>
										</div>
									{/each}
								</div>
							</section>

							<section class="gc-panel-soft px-5 py-5">
								<div class="flex items-start justify-between gap-3">
									<div>
										<p class="gc-stamp text-[var(--gc-ink-3)]">PROFILE ACTIONS</p>
										<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Profile control board</h2>
									</div>
									<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -->
									<a href="/api/team/export" class="gc-action gc-action-accent">
										Export team file
									</a>
								</div>
								<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
									Use the shipped `/api/team` actions here. Profile switching is live, and team file
									import/save now lets you preview broader changes before you apply them.
								</p>

								<div class="mt-5 grid gap-4">
									<form
										class="border border-[var(--gc-border)] px-4 py-4"
										onsubmit={handleSwitchProfile}
									>
										<p class="gc-stamp text-[var(--gc-ink-3)]">Switch profile</p>
										<div class="mt-4 flex flex-col gap-4">
											<SettingsField
												id="team-profile-select"
												label="Profile"
												type="select"
												bind:value={selectedProfileID}
												options={profiles.map((profile) => ({
													value: profile.id,
													label: profile.label
												}))}
											/>
											<div class="flex justify-end">
												<button
													type="submit"
													disabled={teamSaving || selectedProfileID.trim() === ''}
													class="gc-action gc-action-solid px-4 py-2 disabled:opacity-50"
												>
													Switch profile
												</button>
											</div>
										</div>
									</form>

									<form
										class="border border-[var(--gc-border)] px-4 py-4"
										onsubmit={handleCreateProfile}
									>
										<p class="gc-stamp text-[var(--gc-ink-3)]">Create profile</p>
										<div class="mt-4 flex flex-col gap-4">
											<SettingsField
												id="team-profile-create"
												label="New profile ID"
												bind:value={createProfileID}
												placeholder="ops"
											/>
											<div class="flex justify-end">
												<button
													type="submit"
													disabled={teamSaving || createProfileID.trim() === ''}
													class="gc-action gc-action-solid px-4 py-2 disabled:opacity-50"
												>
													Create profile
												</button>
											</div>
										</div>
									</form>

									<form
										class="border border-[var(--gc-border)] px-4 py-4"
										onsubmit={handleCloneProfile}
									>
										<p class="gc-stamp text-[var(--gc-ink-3)]">Clone profile</p>
										<div class="mt-4 flex flex-col gap-4">
											<SettingsField
												id="team-profile-clone-source"
												label="Source profile"
												type="select"
												bind:value={cloneSourceProfileID}
												options={profiles.map((profile) => ({
													value: profile.id,
													label: profile.label
												}))}
											/>
											<SettingsField
												id="team-profile-clone-target"
												label="Clone to profile"
												bind:value={cloneProfileID}
												placeholder="ops"
											/>
											<div class="flex justify-end">
												<button
													type="submit"
													disabled={teamSaving ||
														cloneSourceProfileID.trim() === '' ||
														cloneProfileID.trim() === ''}
													class="gc-action gc-action-solid px-4 py-2 disabled:opacity-50"
												>
													Clone profile
												</button>
											</div>
										</div>
									</form>

									<form
										class="border border-[var(--gc-border)] px-4 py-4"
										onsubmit={handleDeleteProfile}
									>
										<p class="gc-stamp text-[var(--gc-ink-3)]">Delete profile</p>
										{#if inactiveProfiles.length === 0}
											<p class="gc-copy mt-4 text-[var(--gc-ink-3)]">
												No inactive profiles are available to delete.
											</p>
										{:else}
											<div class="mt-4 flex flex-col gap-4">
												<SettingsField
													id="team-profile-delete"
													label="Inactive profile"
													type="select"
													bind:value={deleteProfileID}
													options={inactiveProfiles.map((profile) => ({
														value: profile.id,
														label: profile.label
													}))}
												/>
												<div class="flex justify-end">
													<button
														type="submit"
														disabled={teamSaving || deleteProfileID.trim() === ''}
														class="gc-action gc-action-warning px-4 py-2 disabled:opacity-50"
													>
														Delete profile
													</button>
												</div>
											</div>
										{/if}
									</form>

									<section class="border border-[var(--gc-border)] px-4 py-4">
										<div class="flex items-start justify-between gap-3">
											<div>
												<p class="gc-stamp text-[var(--gc-ink-3)]">TEAM FILE</p>
												<p class="gc-copy mt-2 text-[var(--gc-ink)]">Imported YAML</p>
											</div>
											<button
												type="button"
												disabled={teamSaving || !team}
												class="gc-action gc-action-warning px-4 py-2 disabled:opacity-50"
												onclick={() => void handleSaveTeam()}
											>
												Save team
											</button>
										</div>
										<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
											Import lets you preview the imported team here until you save it.
										</p>

										<label class="mt-4 flex flex-col gap-2">
											<span class="gc-stamp text-[var(--gc-ink-3)]">Imported YAML</span>
											<textarea
												bind:value={importYAML}
												class="gc-control min-h-[12rem] font-mono text-sm"
												placeholder="name: Repo Task Team
front_agent: assistant
agents:
  - id: assistant"
											></textarea>
										</label>

										<div class="mt-4 flex justify-end">
											<button
												type="button"
												disabled={teamSaving || importYAML.trim() === ''}
												class="gc-action gc-action-solid px-4 py-2 disabled:opacity-50"
												onclick={() => void handleImportTeam()}
											>
												Import team file
											</button>
										</div>
									</section>
								</div>
							</section>
						</div>
					</div>

					<section class="gc-panel-soft mt-6 px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">TEAM MEMBERS</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">
							Specialists exposed by the runtime
						</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							Profile lifecycle and YAML import/save are live in this tab. Inline member editing is
							still deferred, so broader structural changes should flow through the exported team
							file.
						</p>

						<div class="mt-5 grid gap-3">
							{#each members as member (member.id)}
								<article class="border border-[var(--gc-border)] px-4 py-4">
									<div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
										<div>
											<p class="gc-copy text-[var(--gc-ink)]">{member.id}</p>
											<p class="gc-copy mt-1 text-[var(--gc-ink-2)]">{member.role}</p>
											<p class="gc-copy mt-2 font-mono text-sm text-[var(--gc-ink-3)]">
												{member.soul_file}
											</p>
										</div>

										<div class="flex flex-wrap gap-2">
											{#if member.is_front}
												<span
													class="gc-stamp border border-[var(--gc-primary)] px-2 py-1 text-[var(--gc-primary)]"
												>
													Front agent
												</span>
											{/if}
											<span
												class="gc-stamp border border-[var(--gc-border)] px-2 py-1 text-[var(--gc-ink-3)]"
											>
												Profile {member.base_profile}
											</span>
											<span
												class="gc-stamp border border-[var(--gc-border)] px-2 py-1 text-[var(--gc-ink-3)]"
											>
												Summary {member.specialist_summary_visibility}
											</span>
										</div>
									</div>

									<div class="mt-4 grid gap-3 md:grid-cols-3">
										<div>
											<p class="gc-stamp text-[var(--gc-ink-3)]">Tools</p>
											<p class="gc-copy mt-2 text-[var(--gc-ink)]">
												{listOrNone(member.tool_families)}
											</p>
										</div>
										<div>
											<p class="gc-stamp text-[var(--gc-ink-3)]">Delegation</p>
											<p class="gc-copy mt-2 text-[var(--gc-ink)]">
												{listOrNone(member.delegation_kinds)}
											</p>
										</div>
										<div>
											<p class="gc-stamp text-[var(--gc-ink-3)]">Can Message</p>
											<p class="gc-copy mt-2 text-[var(--gc-ink)]">
												{listOrNone(member.can_message)}
											</p>
										</div>
									</div>
								</article>
							{/each}
						</div>
					</section>
				{:else}
					<div class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">AGENTS &amp; ROUTING</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Team surface unavailable</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							The runtime did not return `/api/team`. Use the checked-in team files until that
							surface is available again.
						</p>
					</div>
				{/if}
			</div>
		{:else if activeTab === 'models'}
			<div class="mx-auto w-full max-w-6xl px-6 py-6">
				<div class="grid gap-4 xl:grid-cols-4">
					<SurfaceMetricCard
						label="Shipped Providers"
						value="Anthropic + OpenAI-compatible"
						detail="Those are the provider seams wired in the runtime today."
						tone="accent"
					/>
					<SurfaceMetricCard
						label="Selection"
						value="Runtime-owned"
						detail="Per-role defaults still live in checked-in config and team files."
					/>
					<SurfaceMetricCard
						label="Recent Models"
						value={String(modelUsage.length)}
						detail={`${work?.clusters.length ?? 0} visible run cluster${work?.clusters.length === 1 ? '' : 's'}.`}
					/>
					<SurfaceMetricCard
						label="Active Project"
						value={machine?.active_project_name ?? 'None'}
						detail={machine?.active_project_path ?? 'Project path unavailable'}
						tone="accent"
					/>
				</div>

				<div class="mt-6 grid gap-4 xl:grid-cols-[minmax(0,1.1fr)_minmax(18rem,0.9fr)]">
					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">MODELS</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Model posture stays explicit</h2>
						<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
							OpenClaw separates model inventory from live usage. In GistClaw, the browser can show
							evidence about current model usage, but model defaults and provider wiring are still
							runtime-owned rather than browser-editable.
						</p>

						<div class="mt-5 grid gap-3 md:grid-cols-2">
							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Provider Seams</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">Anthropic + OpenAI-compatible</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
									The shipped adapters already plug in behind the provider seam.
								</p>
							</div>

							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Selection</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">Runtime-owned</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
									Use checked-in config and team files when you need to change defaults.
								</p>
							</div>

							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Operator Flow</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">
									Check Chat and Debug when you need live run-level evidence.
								</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
									This tab keeps the machine posture and recent usage in one place.
								</p>
							</div>

							<div class="border border-[var(--gc-border)] px-4 py-4">
								<p class="gc-stamp text-[var(--gc-ink-3)]">Browser Editing</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink)]">Not wired yet</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
									No live model catalog or save API ships in GistClaw today.
								</p>
							</div>
						</div>
					</section>

					<section class="gc-panel-soft px-5 py-5">
						<p class="gc-stamp text-[var(--gc-ink-3)]">RECENT MODEL USAGE</p>
						<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Recent model usage</h2>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							Summary derived from the visible `/api/work` run clusters.
						</p>

						{#if modelUsage.length === 0}
							<div class="mt-5 border border-dashed border-[var(--gc-border)] px-4 py-5">
								<p class="gc-copy text-[var(--gc-ink)]">No recent model evidence</p>
								<p class="gc-copy mt-2 text-[var(--gc-ink-3)]">
									Run work from Chat to populate model usage here.
								</p>
							</div>
						{:else}
							<div class="mt-5 grid gap-3">
								{#each modelUsage as entry (entry.model)}
									<div class="border border-[var(--gc-border)] px-4 py-4">
										<div class="flex items-center justify-between gap-4">
											<p class="gc-copy text-[var(--gc-ink)]">{entry.model}</p>
											<span class="gc-stamp text-[var(--gc-primary)]">
												{entry.count} run{entry.count === 1 ? '' : 's'}
											</span>
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</section>
				</div>
			</div>
		{:else if activeTab === 'channels'}
			<div class="mx-auto w-full max-w-4xl px-6 py-6">
				<p class="gc-stamp text-[var(--gc-ink-3)]">CHANNELS</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Channel configuration</h2>
				<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
					OpenClaw splits live channel status from deeper token/config work. GistClaw only exposes
					the Telegram bot token through settings today, so live connectivity stays in Channels and
					the actual token edit lives in General.
				</p>

				<div class="mt-6 grid gap-4 lg:grid-cols-2">
					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Telegram</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink)]">{machine?.telegram_token ?? '—'}</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink-3)]">
							Masked token from the current runtime settings.
						</p>
					</section>
					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Workflow</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink-2)]">
							Use the General tab to change the token, then restart the runtime if the connector
							does not reconnect on its own.
						</p>
					</section>
				</div>
			</div>
		{:else if activeTab === 'raw'}
			<div class="mx-auto flex min-h-0 w-full max-w-6xl flex-1 flex-col px-6 py-6">
				<p class="gc-stamp text-[var(--gc-ink-3)]">RAW JSON5</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Runtime settings snapshot</h2>
				<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
					Read-only JSON view for the current settings payload. This mirrors the raw mode concept in
					OpenClaw without pretending GistClaw has a full raw config save/apply API yet.
				</p>

				<div class="raw-editor gc-panel-soft mt-6 min-h-[24rem] flex-1 overflow-hidden px-0 py-0">
					<div bind:this={rawEditorEl} class="h-full min-h-[24rem] overflow-auto"></div>
				</div>
			</div>
		{:else}
			<div class="mx-auto w-full max-w-4xl px-6 py-6">
				<p class="gc-stamp text-[var(--gc-ink-3)]">APPLY</p>
				<h2 class="gc-panel-title mt-3 text-[var(--gc-ink)]">Apply notes</h2>
				<p class="gc-copy mt-3 max-w-2xl text-[var(--gc-ink-2)]">
					Settings save immediately through `/api/settings`. Some changes still need a runtime
					restart before connectors or elevated host access behavior fully reflect the new values.
				</p>

				<div class="mt-6 grid gap-4 lg:grid-cols-2">
					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Storage Root</p>
						<p class="gc-copy mt-3 font-mono text-sm text-[var(--gc-ink-2)]">
							{machine?.storage_root ?? '—'}
						</p>
					</section>
					<section class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp text-[var(--gc-ink-3)]">Active Project</p>
						<p class="gc-copy mt-3 text-[var(--gc-ink)]">{machine?.active_project_name ?? '—'}</p>
						<p class="gc-copy mt-2 font-mono text-sm text-[var(--gc-ink-3)]">
							{machine?.active_project_path ?? '—'}
						</p>
					</section>
				</div>
			</div>
		{/if}
	</div>
</div>

<style>
	.raw-editor :global(.cm-editor) {
		height: 100%;
		background: var(--gc-surface);
	}

	.raw-editor :global(.cm-scroller) {
		overflow: auto;
		font-family: 'SFMono-Regular', ui-monospace, 'JetBrains Mono', monospace;
	}
</style>

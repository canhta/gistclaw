<script lang="ts">
	import { beforeNavigate, invalidateAll } from '$app/navigation';
	import SurfaceActionButton from '$lib/components/common/SurfaceActionButton.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import type { TeamConfigResponse, TeamMemberResponse, TeamResponse } from '$lib/types/api';
	import { teamConfigMatches, teamDraftFromConfig } from './team-state';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let editMode = $state(false);

	let noticeOverride = $state<string | null>(null);
	let errorMessage = $state('');
	let saving = $state(false);
	let busyAction = $state('');
	let nameOverride = $state<string | null>(null);
	let frontAgentOverride = $state<string | null>(null);
	let roleOverrides = $state<Record<string, string>>({});
	let baseProfileOverrides = $state<Record<string, string>>({});
	let toolFamilyOverrides = $state<Record<string, string[]>>({});
	let delegationKindOverrides = $state<Record<string, string[]>>({});
	let memberDrafts = $state<TeamMemberResponse[] | null>(null);
	let createProfileID = $state('');
	let cloneSourceProfileID = $state('');
	let cloneProfileID = $state('');
	let deleteProfileID = $state('');
	let importFile = $state<File | null>(null);

	const notice = $derived(noticeOverride ?? data.team.notice ?? '');

	const baseProfiles = ['operator', 'research', 'write', 'review', 'verify'];
	const toolFamilies = [
		'repo_read',
		'repo_write',
		'runtime_capability',
		'connector_capability',
		'web_read',
		'delegate',
		'verification',
		'diff_review'
	];
	const delegationKinds = ['research', 'write', 'review', 'verify'];

	function resetDraft(): void {
		nameOverride = null;
		frontAgentOverride = null;
		roleOverrides = {};
		baseProfileOverrides = {};
		toolFamilyOverrides = {};
		delegationKindOverrides = {};
		memberDrafts = null;
		errorMessage = '';
	}

	function loadDraft(team: TeamConfigResponse): void {
		const draft = teamDraftFromConfig(team);
		nameOverride = draft.name;
		frontAgentOverride = draft.front_agent_id;
		memberDrafts = draft.members;
		roleOverrides = {};
		baseProfileOverrides = {};
		toolFamilyOverrides = {};
		delegationKindOverrides = {};
		errorMessage = '';
	}

	function hasDraft(): boolean {
		return !teamConfigMatches(buildTeamConfig(), data.team.team);
	}

	function confirmDiscardDraft(): boolean {
		if (!hasDraft()) return true;

		return confirm('Discard unsaved changes?');
	}

	beforeNavigate(({ cancel }) => {
		if (hasDraft()) {
			if (!confirm('You have unsaved changes. Leave without saving?')) {
				cancel();
			}
		}
	});

	function teamMembers(): TeamMemberResponse[] {
		return memberDrafts ?? data.team.team.members;
	}

	function teamName(): string {
		return nameOverride ?? data.team.team.name;
	}

	function activeProfileLabel(): string {
		return data.team.active_profile.label;
	}

	function frontAgentID(): string {
		const selected = frontAgentOverride ?? data.team.team.front_agent_id;
		if (teamMembers().some((member) => member.id === selected)) {
			return selected;
		}
		return teamMembers()[0]?.id ?? '';
	}

	function memberRole(member: TeamMemberResponse): string {
		return roleOverrides[member.id] ?? member.role;
	}

	function memberBaseProfile(member: TeamMemberResponse): string {
		return baseProfileOverrides[member.id] ?? member.base_profile;
	}

	function memberToolFamilies(member: TeamMemberResponse): string[] {
		return toolFamilyOverrides[member.id] ?? member.tool_families;
	}

	function memberDelegationKinds(member: TeamMemberResponse): string[] {
		return delegationKindOverrides[member.id] ?? member.delegation_kinds;
	}

	function toggleListValue(values: string[], value: string): string[] {
		return values.includes(value) ? values.filter((item) => item !== value) : [...values, value];
	}

	function nextMemberID(): string {
		const seen = new Set(teamMembers().map((member) => member.id));
		for (let index = 1; ; index += 1) {
			const candidate = `agent_${index}`;
			if (!seen.has(candidate)) {
				return candidate;
			}
		}
	}

	function addMember(): void {
		const id = nextMemberID();
		memberDrafts = [
			...teamMembers(),
			{
				id,
				role: 'research specialist',
				soul_file: `${id}.soul.yaml`,
				base_profile: 'research',
				tool_families: ['repo_read', 'web_read'],
				delegation_kinds: [],
				can_message: [],
				specialist_summary_visibility: 'basic',
				soul_extra: {},
				is_front: false
			}
		];
	}

	function removeMember(memberID: string): void {
		if (teamMembers().length === 1) {
			errorMessage = 'Team must keep at least one role.';
			return;
		}
		const filtered = teamMembers()
			.filter((member) => member.id !== memberID)
			.map((member) => ({
				...member,
				can_message: member.can_message.filter((linkedID) => linkedID !== memberID),
				is_front: member.id === frontAgentID() && member.id !== memberID
			}));
		memberDrafts = filtered;
		if (frontAgentID() === memberID) {
			frontAgentOverride = filtered[0]?.id ?? '';
		}
		delete roleOverrides[memberID];
		delete baseProfileOverrides[memberID];
		delete toolFamilyOverrides[memberID];
		delete delegationKindOverrides[memberID];
	}

	function buildTeamConfig(): TeamConfigResponse {
		const members = teamMembers().map((member) => ({
			...member,
			role: memberRole(member),
			base_profile: memberBaseProfile(member),
			tool_families: memberToolFamilies(member),
			delegation_kinds: memberDelegationKinds(member),
			is_front: member.id === frontAgentID()
		}));
		return {
			name: teamName(),
			front_agent_id: frontAgentID(),
			member_count: members.length,
			members
		};
	}

	async function useProfile(profileID: string): Promise<void> {
		if (!confirmDiscardDraft()) return;
		busyAction = `use:${profileID}`;
		errorMessage = '';

		try {
			const response = await requestJSON<TeamResponse>(fetch, '/api/team/select', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ profile_id: profileID })
			});
			resetDraft();
			noticeOverride = response.notice ?? null;
			await invalidateAll();
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to switch the setup right now.';
		} finally {
			busyAction = '';
		}
	}

	async function saveTeam(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		saving = true;
		errorMessage = '';

		try {
			const response = await requestJSON<TeamResponse>(fetch, '/api/team/save', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ team: buildTeamConfig() })
			});
			resetDraft();
			noticeOverride = response.notice ?? 'Setup saved.';
			await invalidateAll();
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to save the setup right now.';
		} finally {
			saving = false;
		}
	}

	async function createProfile(): Promise<void> {
		const profileID = createProfileID.trim();
		if (profileID === '') {
			errorMessage = 'Create setup needs a name.';
			return;
		}

		busyAction = 'create';
		errorMessage = '';
		try {
			const response = await requestJSON<TeamResponse>(fetch, '/api/team/create', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ profile_id: profileID })
			});
			createProfileID = '';
			resetDraft();
			noticeOverride = response.notice ?? null;
			await invalidateAll();
		} catch (error) {
			errorMessage = error instanceof HTTPError ? error.message : 'Unable to create this setup.';
		} finally {
			busyAction = '';
		}
	}

	async function cloneProfile(): Promise<void> {
		const sourceProfileID = (cloneSourceProfileID || data.team.active_profile.id).trim();
		const profileID = cloneProfileID.trim();
		if (sourceProfileID === '' || profileID === '') {
			errorMessage = 'Copy setup needs a source and a new name.';
			return;
		}

		busyAction = 'clone';
		errorMessage = '';
		try {
			const response = await requestJSON<TeamResponse>(fetch, '/api/team/clone', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({
					source_profile_id: sourceProfileID,
					profile_id: profileID
				})
			});
			cloneProfileID = '';
			resetDraft();
			noticeOverride = response.notice ?? null;
			await invalidateAll();
		} catch (error) {
			errorMessage = error instanceof HTTPError ? error.message : 'Unable to copy this setup.';
		} finally {
			busyAction = '';
		}
	}

	async function deleteProfile(): Promise<void> {
		const profileID = deleteProfileID.trim();
		if (profileID === '') {
			errorMessage = 'Delete setup needs a target profile.';
			return;
		}

		busyAction = 'delete';
		errorMessage = '';
		try {
			const response = await requestJSON<TeamResponse>(fetch, '/api/team/delete', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ profile_id: profileID })
			});
			deleteProfileID = '';
			resetDraft();
			noticeOverride = response.notice ?? null;
			await invalidateAll();
		} catch (error) {
			errorMessage = error instanceof HTTPError ? error.message : 'Unable to delete this setup.';
		} finally {
			busyAction = '';
		}
	}

	async function importSetup(): Promise<void> {
		if (!importFile) {
			errorMessage = 'Import setup file needs a YAML file.';
			return;
		}

		busyAction = 'import';
		errorMessage = '';
		try {
			const yaml = await importFile.text();
			const response = await requestJSON<TeamResponse>(fetch, '/api/team/import', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({ yaml })
			});
			importFile = null;
			loadDraft(response.team);
			noticeOverride = response.notice ?? null;
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to import this setup file.';
		} finally {
			busyAction = '';
		}
	}

	function exportSetup(): void {
		window.location.assign('/api/team/export');
	}
</script>

<svelte:head>
	<title>Team | GistClaw</title>
</svelte:head>

<div class="grid gap-6">
	{#if hasDraft()}
		<div
			class="gc-panel-soft flex items-center justify-between gap-4 border-[var(--gc-orange)] px-4 py-4"
		>
			<p class="gc-stamp text-[var(--gc-orange)]">Unsaved changes</p>
			<div class="flex gap-3">
				<SurfaceActionButton
					tone="solid"
					onclick={() => document.querySelector('form')?.requestSubmit()}
				>
					Save now
				</SurfaceActionButton>
				<SurfaceActionButton onclick={resetDraft}>Discard</SurfaceActionButton>
			</div>
		</div>
	{/if}

	{#if !editMode}
		<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<div class="flex items-start justify-between gap-4">
				<div>
					<p class="gc-stamp">Current setup</p>
					<h2 class="gc-section-title mt-3">{activeProfileLabel()}</h2>
					<p class="gc-machine mt-2">Team name: {data.team.team.name}</p>
				</div>
				<SurfaceActionButton onclick={() => (editMode = true)}>Edit setup</SurfaceActionButton>
			</div>
			<div class="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
				{#each data.team.team.members as member (member.id)}
					<div class="gc-panel-soft px-4 py-4">
						<p class="gc-stamp">{member.role}</p>
						<p class="gc-panel-title mt-2 text-[1rem]">{member.base_profile}</p>
						<p class="gc-machine mt-2">{member.tool_families.length} tool families</p>
					</div>
				{/each}
			</div>
		</section>
	{/if}

	{#if editMode}
		<section class="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
			<form class="gc-panel px-5 py-5 lg:px-6 lg:py-6" onsubmit={saveTeam}>
				<p class="gc-stamp">Active profile</p>
				<h2 class="gc-section-title mt-3">{activeProfileLabel()}</h2>
				<p class="gc-machine mt-2">Team name: {teamName()}</p>
				<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">
					Change the team name, who leads the work, and what each role is allowed to do.
				</p>

				{#if notice}
					<SurfaceMessage label="Setup notice" message={notice} className="mt-5" />
				{/if}

				{#if errorMessage}
					<SurfaceMessage
						label="Setup error"
						message={errorMessage}
						tone="error"
						className="mt-5"
					/>
				{/if}

				<div class="mt-6 grid gap-4 md:grid-cols-2">
					<label class="grid gap-2">
						<span class="gc-stamp">Team name</span>
						<input
							value={teamName()}
							oninput={(event) => {
								nameOverride = event.currentTarget.value;
							}}
							class="gc-control"
						/>
					</label>

					<label class="grid gap-2">
						<span class="gc-stamp">Front agent</span>
						<select
							value={frontAgentID()}
							onchange={(event) => {
								frontAgentOverride = event.currentTarget.value;
							}}
							class="gc-control"
						>
							{#each teamMembers() as member (member.id)}
								<option value={member.id}>{member.id}</option>
							{/each}
						</select>
					</label>
				</div>

				<div class="mt-5 border-t-2 border-[var(--gc-border)] pt-4">
					<p class="gc-stamp">Save path</p>
					<p class="gc-machine mt-2 break-all">{data.team.active_profile.save_path}</p>
				</div>

				<SurfaceActionButton type="submit" tone="solid" className="mt-6" disabled={saving}>
					{saving ? 'Saving setup' : 'Save setup'}
				</SurfaceActionButton>
			</form>

			<div class="grid gap-4">
				<div class="gc-panel px-4 py-4">
					<p class="gc-stamp">Available setups</p>
					<div class="mt-4 grid gap-3">
						{#each data.team.profiles as profile (profile.id)}
							<button
								type="button"
								class={`flex items-center justify-between border-2 px-4 py-3 text-left transition-colors ${profile.active ? 'border-[var(--gc-orange)] bg-[rgba(255,105,34,0.08)]' : 'border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] hover:border-[var(--gc-cyan)]'}`}
								onclick={() => useProfile(profile.id)}
								disabled={busyAction !== '' && busyAction !== `use:${profile.id}`}
							>
								<div>
									<p class="gc-stamp">{profile.active ? 'Active setup' : 'Available setup'}</p>
									<p class="gc-panel-title mt-2 text-[1rem]">{profile.label}</p>
								</div>
								<p class="gc-machine">
									{busyAction === `use:${profile.id}`
										? 'Switching'
										: profile.active
											? 'Current'
											: 'Use setup'}
								</p>
							</button>
						{/each}
					</div>
				</div>

				<div class="gc-panel px-4 py-4">
					<p class="gc-stamp">Setup actions</p>
					<div class="mt-4 grid gap-4">
						<div class="grid gap-2">
							<p class="gc-stamp">Create setup</p>
							<input bind:value={createProfileID} placeholder="review" class="gc-control" />
							<SurfaceActionButton onclick={createProfile} disabled={busyAction !== ''}>
								Create setup
							</SurfaceActionButton>
						</div>

						<div class="grid gap-2 border-t-2 border-[var(--gc-border)] pt-4">
							<p class="gc-stamp">Copy setup</p>
							<select bind:value={cloneSourceProfileID} class="gc-control">
								<option value="">Use active setup</option>
								{#each data.team.profiles as profile (profile.id)}
									<option value={profile.id}>{profile.label}</option>
								{/each}
							</select>
							<input bind:value={cloneProfileID} placeholder="review-copy" class="gc-control" />
							<SurfaceActionButton onclick={cloneProfile} disabled={busyAction !== ''}>
								Copy setup
							</SurfaceActionButton>
						</div>

						<div class="grid gap-2 border-t-2 border-[var(--gc-border)] pt-4">
							<p class="gc-stamp">Delete setup</p>
							<select bind:value={deleteProfileID} class="gc-control">
								<option value="">Pick an inactive setup</option>
								{#each data.team.profiles.filter((profile) => !profile.active) as profile (profile.id)}
									<option value={profile.id}>{profile.label}</option>
								{/each}
							</select>
							<SurfaceActionButton
								tone="warning"
								onclick={deleteProfile}
								disabled={busyAction !== '' || deleteProfileID.trim() === ''}
							>
								Delete setup
							</SurfaceActionButton>
						</div>

						<div class="grid gap-2 border-t-2 border-[var(--gc-border)] pt-4">
							<p class="gc-stamp">Import setup file</p>
							<input
								type="file"
								accept=".yaml,.yml,text/yaml"
								onchange={(event) => {
									importFile = event.currentTarget.files?.[0] ?? null;
								}}
								class="gc-control"
							/>
							<SurfaceActionButton
								onclick={importSetup}
								disabled={busyAction !== '' || !importFile}
							>
								Import setup file
							</SurfaceActionButton>
						</div>

						<div class="flex flex-wrap gap-3 border-t-2 border-[var(--gc-border)] pt-4">
							<SurfaceActionButton onclick={exportSetup}>Export YAML</SurfaceActionButton>
						</div>
					</div>
				</div>

				<div class="gc-panel-soft px-4 py-4">
					<p class="gc-stamp">Team summary</p>
					<div class="mt-4 grid gap-3 sm:grid-cols-2">
						<div>
							<p class="gc-stamp">Front agent</p>
							<p class="gc-value mt-2">{frontAgentID()}</p>
						</div>
						<div>
							<p class="gc-stamp">Members</p>
							<p class="gc-value mt-2">{teamMembers().length}</p>
						</div>
					</div>
				</div>
			</div>
		</section>

		<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<div class="flex flex-wrap items-end justify-between gap-4">
				<div>
					<p class="gc-stamp">Role topology</p>
					<h2 class="gc-section-title mt-3">See who is carrying the work right now</h2>
				</div>
				<div class="flex flex-wrap gap-3">
					<p class="gc-machine">{teamMembers().length} visible roles</p>
					<SurfaceActionButton onclick={addMember}>Add another role</SurfaceActionButton>
				</div>
			</div>

			<div class="mt-6 grid gap-4 xl:grid-cols-3">
				{#each teamMembers() as member, index (member.id)}
					<article
						class={`gc-panel-soft px-4 py-4 ${frontAgentID() === member.id ? 'border-[var(--gc-orange)]' : ''}`}
					>
						<div class="flex items-start justify-between gap-3">
							<div>
								<p class="gc-stamp">
									{frontAgentID() === member.id ? 'Front role' : 'Specialist role'}
								</p>
								<h3 class="gc-panel-title mt-3 text-[1rem]">{member.id}</h3>
							</div>
							<div class="flex flex-col items-end gap-2">
								<p class="gc-machine">{memberBaseProfile(member)}</p>
								{#if teamMembers().length > 1}
									<SurfaceActionButton
										tone="warning"
										className="px-3 py-2 text-[11px]"
										onclick={() => removeMember(member.id)}
									>
										Remove role
									</SurfaceActionButton>
								{/if}
							</div>
						</div>

						<label class="mt-4 grid gap-2">
							<span class="gc-stamp">Role</span>
							<input
								value={memberRole(member)}
								oninput={(event) => {
									roleOverrides = {
										...roleOverrides,
										[member.id]: event.currentTarget.value
									};
								}}
								class="gc-control"
							/>
						</label>

						<label class="mt-4 grid gap-2">
							<span class="gc-stamp">Base profile</span>
							<select
								value={memberBaseProfile(member)}
								onchange={(event) => {
									baseProfileOverrides = {
										...baseProfileOverrides,
										[member.id]: event.currentTarget.value
									};
								}}
								class="gc-control"
							>
								{#each baseProfiles as baseProfile (baseProfile)}
									<option value={baseProfile}>{baseProfile}</option>
								{/each}
							</select>
						</label>

						<div class="mt-4 border-t-2 border-[var(--gc-border)] pt-4">
							<p class="gc-stamp">Tool authority</p>
							<div class="mt-3 flex flex-wrap gap-2">
								{#each toolFamilies as toolFamily (toolFamily)}
									<button
										type="button"
										class={`gc-chip ${memberToolFamilies(member).includes(toolFamily) ? 'gc-chip-accent' : ''}`}
										onclick={() => {
											toolFamilyOverrides = {
												...toolFamilyOverrides,
												[member.id]: toggleListValue(memberToolFamilies(member), toolFamily)
											};
										}}
									>
										{toolFamily}
									</button>
								{/each}
							</div>
						</div>

						<div class="mt-4 border-t-2 border-[var(--gc-border)] pt-4">
							<p class="gc-stamp">Delegation posture</p>
							<div class="mt-3 flex flex-wrap gap-2">
								{#each delegationKinds as delegationKind (delegationKind)}
									<button
										type="button"
										class={`gc-chip ${memberDelegationKinds(member).includes(delegationKind) ? 'gc-chip-warning' : ''}`}
										onclick={() => {
											delegationKindOverrides = {
												...delegationKindOverrides,
												[member.id]: toggleListValue(memberDelegationKinds(member), delegationKind)
											};
										}}
									>
										{delegationKind}
									</button>
								{/each}
							</div>
						</div>

						<div class="mt-4 border-t-2 border-[var(--gc-border)] pt-4">
							<p class="gc-stamp">Can message</p>
							<p class="gc-copy mt-2 text-[var(--gc-ink)]">
								{member.can_message.join(', ') || 'No direct links'}
							</p>
							<p class="gc-machine mt-2">{member.soul_file}</p>
						</div>

						{#if index === teamMembers().length - 1}
							<div class="mt-4 border-t-2 border-[var(--gc-border)] pt-4">
								<p class="gc-stamp">Active front agent</p>
								<p class="gc-copy mt-2 text-[var(--gc-text-secondary)]">
									Choose which role speaks first, then save the setup so handoffs stay clear.
								</p>
							</div>
						{/if}
					</article>
				{/each}
			</div>
		</section>
	{/if}
</div>

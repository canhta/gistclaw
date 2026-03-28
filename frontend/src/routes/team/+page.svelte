<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import type { TeamConfigResponse, TeamMemberResponse, TeamResponse } from '$lib/types/api';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let noticeOverride = $state<string | null>(null);
	let errorMessage = $state('');
	let saving = $state(false);
	let switchingProfile = $state('');
	let nameOverride = $state<string | null>(null);
	let frontAgentOverride = $state<string | null>(null);
	let roleOverrides = $state<Record<string, string>>({});
	let baseProfileOverrides = $state<Record<string, string>>({});
	let toolFamilyOverrides = $state<Record<string, string[]>>({});
	let delegationKindOverrides = $state<Record<string, string[]>>({});

	const notice = $derived(noticeOverride ?? data.team.notice ?? '');

	async function useProfile(profileID: string): Promise<void> {
		switchingProfile = profileID;
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
			switchingProfile = '';
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

	function buildTeamConfig(): TeamConfigResponse {
		return {
			name: teamName(),
			front_agent_id: frontAgentID(),
			member_count: data.team.team.member_count,
			members: data.team.team.members.map((member) => ({
				...member,
				role: memberRole(member),
				base_profile: memberBaseProfile(member),
				tool_families: memberToolFamilies(member),
				delegation_kinds: memberDelegationKinds(member)
			}))
		};
	}

	function resetDraft(): void {
		nameOverride = null;
		frontAgentOverride = null;
		roleOverrides = {};
		baseProfileOverrides = {};
		toolFamilyOverrides = {};
		delegationKindOverrides = {};
		errorMessage = '';
	}

	function teamName(): string {
		return nameOverride ?? data.team.team.name;
	}

	function frontAgentID(): string {
		return frontAgentOverride ?? data.team.team.front_agent_id;
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
</script>

<svelte:head>
	<title>Team | GistClaw</title>
</svelte:head>

<div class="grid gap-6">
	<section class="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
		<form class="gc-panel px-5 py-5 lg:px-6 lg:py-6" onsubmit={saveTeam}>
			<p class="gc-stamp">Active setup</p>
			<h2 class="gc-section-title mt-3">{teamName()}</h2>
			<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">
				Keep the human relationship anchored in one front agent, then make the specialist shape
				explicit enough that delegation stops feeling magical.
			</p>

			{#if notice}
				<SurfaceMessage label="Setup notice" message={notice} className="mt-5" />
			{/if}

			{#if errorMessage}
				<SurfaceMessage label="Setup error" message={errorMessage} tone="error" className="mt-5" />
			{/if}

			<div class="mt-6 grid gap-4 md:grid-cols-2">
				<label class="grid gap-2">
					<span class="gc-stamp">Setup name</span>
					<input
						value={teamName()}
						oninput={(event) => {
							nameOverride = event.currentTarget.value;
						}}
						class="border-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] px-4 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-orange)]"
					/>
				</label>

				<label class="grid gap-2">
					<span class="gc-stamp">Front agent</span>
					<select
						value={frontAgentID()}
						onchange={(event) => {
							frontAgentOverride = event.currentTarget.value;
						}}
						class="border-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] px-4 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-orange)]"
					>
						{#each data.team.team.members as member (member.id)}
							<option value={member.id}>{member.id}</option>
						{/each}
					</select>
				</label>
			</div>

			<div class="mt-5 border-t-2 border-[var(--gc-border)] pt-4">
				<p class="gc-stamp">Save path</p>
				<p class="gc-machine mt-2 break-all">{data.team.active_profile.save_path}</p>
			</div>

			<button
				type="submit"
				class="mt-6 border-2 border-[var(--gc-orange)] bg-[var(--gc-orange)] px-4 py-3 text-left text-sm font-[var(--gc-font-mono)] font-bold tracking-[0.18em] text-[var(--gc-canvas)] uppercase transition-colors hover:border-[var(--gc-orange-hover)] hover:bg-[var(--gc-orange-hover)] disabled:cursor-not-allowed disabled:opacity-60"
				disabled={saving}
			>
				{saving ? 'Saving setup' : 'Save setup'}
			</button>
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
							disabled={switchingProfile !== '' && switchingProfile !== profile.id}
						>
							<div>
								<p class="gc-stamp">{profile.active ? 'Active setup' : 'Available setup'}</p>
								<p class="gc-panel-title mt-2 text-[1rem]">{profile.label}</p>
							</div>
							<p class="gc-machine">
								{switchingProfile === profile.id
									? 'Switching'
									: profile.active
										? 'Current'
										: 'Use setup'}
							</p>
						</button>
					{/each}
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
						<p class="gc-value mt-2">{data.team.team.member_count}</p>
					</div>
				</div>
			</div>
		</div>
	</section>

	<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
		<div class="flex flex-wrap items-end justify-between gap-4">
			<div>
				<p class="gc-stamp">Role topology</p>
				<h2 class="gc-section-title mt-3">See who carries the work before the runtime fans out</h2>
			</div>
			<p class="gc-machine">{data.team.team.member_count} visible roles</p>
		</div>

		<div class="mt-6 grid gap-4 xl:grid-cols-3">
			{#each data.team.team.members as member, index (member.id)}
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
						<p class="gc-machine">{memberBaseProfile(member)}</p>
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
							class="border-2 border-[var(--gc-border)] bg-[var(--gc-surface)] px-3 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-cyan)]"
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
							class="border-2 border-[var(--gc-border)] bg-[var(--gc-surface)] px-3 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-cyan)]"
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
									class={`border px-3 py-2 text-xs font-[var(--gc-font-mono)] tracking-[0.16em] uppercase ${memberToolFamilies(member).includes(toolFamily) ? 'border-[var(--gc-cyan)] bg-[rgba(83,199,240,0.08)] text-[var(--gc-ink)]' : 'border-[var(--gc-border)] text-[var(--gc-text-secondary)]'}`}
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
									class={`border px-3 py-2 text-xs font-[var(--gc-font-mono)] tracking-[0.16em] uppercase ${memberDelegationKinds(member).includes(delegationKind) ? 'border-[var(--gc-orange)] bg-[rgba(255,105,34,0.08)] text-[var(--gc-ink)]' : 'border-[var(--gc-border)] text-[var(--gc-text-secondary)]'}`}
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

					{#if index === data.team.team.members.length - 1}
						<div class="mt-4 border-t-2 border-[var(--gc-border)] pt-4">
							<p class="gc-stamp">Active front agent</p>
							<p class="gc-copy mt-2 text-[var(--gc-text-secondary)]">
								Choose which role speaks for the system first, then save the setup so runtime
								fan-out stays coherent.
							</p>
						</div>
					{/if}
				</article>
			{/each}
		</div>
	</section>
</div>

<script lang="ts">
	import { goto } from '$app/navigation';
	import SurfaceActionButton from '$lib/components/common/SurfaceActionButton.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import type { OnboardingPreviewResponse, OnboardingResponse } from '$lib/types/api';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let onboardingOverride = $state<OnboardingResponse | null>(null);
	let existingRepoPath = $state('');
	let newRepoPath = $state('');
	let errorMessage = $state('');
	let noticeMessage = $state('');
	let bindingExisting = $state(false);
	let bindingStarter = $state(false);
	let bindingNew = $state(false);
	let launchingTask = $state('');

	const onboardingState = $derived(onboardingOverride ?? data.onboarding);
	const activeProject = $derived(onboardingState.project);
	const previewState = $derived(onboardingState.preview);
	const starterAvailable = $derived(!onboardingState.completed && !!onboardingState.project);

	async function bindProject(source: 'starter' | 'existing_repo' | 'new_project'): Promise<void> {
		errorMessage = '';
		noticeMessage = '';
		bindingStarter = source === 'starter';
		bindingExisting = source === 'existing_repo';
		bindingNew = source === 'new_project';

		try {
			onboardingOverride = await requestJSON<OnboardingResponse>(fetch, '/api/onboarding/project', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({
					source,
					project_path:
						source === 'existing_repo'
							? existingRepoPath
							: source === 'new_project'
								? newRepoPath
								: undefined
				})
			});
			noticeMessage =
				source === 'starter'
					? 'Starter project is active. Launch a preview run or head into Work.'
					: 'Project bound. Launch a preview run or move straight into Work.';
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to bind that project right now.';
		} finally {
			bindingStarter = false;
			bindingExisting = false;
			bindingNew = false;
		}
	}

	async function startPreview(task: string): Promise<void> {
		errorMessage = '';
		noticeMessage = '';
		launchingTask = task;

		try {
			const result = await requestJSON<OnboardingPreviewResponse>(
				fetch,
				'/api/onboarding/preview',
				{
					method: 'POST',
					headers: { 'content-type': 'application/json' },
					body: JSON.stringify({ task })
				}
			);
			// eslint-disable-next-line svelte/no-navigation-without-resolve
			await goto(result.next_href, { invalidateAll: true });
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to start the preview run right now.';
		} finally {
			launchingTask = '';
		}
	}
</script>

<svelte:head>
	<title>Onboarding | GistClaw</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center px-4 py-8 lg:px-8">
	<div class="grid w-full max-w-7xl gap-6 xl:grid-cols-[minmax(0,1.15fr)_minmax(20rem,0.85fr)]">
		<section class="gc-panel px-6 py-6 lg:px-8 lg:py-8">
			<p class="gc-stamp">First session</p>
			<h1 class="gc-page-title mt-4">Bind a repo and stage the first task</h1>
			<p class="gc-copy mt-5 max-w-3xl text-[var(--gc-text-secondary)]">
				GistClaw should open from the user’s job, not from runtime internals. Connect the repo you
				actually want to steer, then kick off one preview run to make the control deck useful
				immediately.
			</p>

			{#if noticeMessage}
				<SurfaceMessage
					label="Project ready"
					message={noticeMessage}
					className="mt-6 border-[var(--gc-orange)]"
				/>
			{/if}

			{#if errorMessage}
				<SurfaceMessage
					label="Onboarding error"
					message={errorMessage}
					tone="error"
					className="mt-6"
				/>
			{/if}

			<div class="mt-8 grid gap-4 lg:grid-cols-3">
				{#if starterAvailable && activeProject}
					<div class="gc-panel-soft gc-card-accent px-4 py-4">
						<p class="gc-stamp">Use the local starter project</p>
						<p class="gc-panel-title mt-3">{activeProject.active_name}</p>
						<p class="gc-copy mt-3 break-all text-[var(--gc-text-secondary)]">
							{activeProject.active_path}
						</p>
						<SurfaceActionButton
							className="mt-5 w-full"
							tone="solid"
							disabled={bindingStarter}
							onclick={() => {
								void bindProject('starter');
							}}
						>
							{bindingStarter ? 'Binding starter' : 'Use starter repo'}
						</SurfaceActionButton>
					</div>
				{/if}

				<form
					class="gc-panel-soft px-4 py-4"
					onsubmit={(event) => {
						event.preventDefault();
						void bindProject('existing_repo');
					}}
				>
					<p class="gc-stamp">Bind existing repo</p>
					<label class="mt-4 grid gap-2">
						<span class="gc-stamp text-[var(--gc-text-tertiary)]">Repo path</span>
						<input
							bind:value={existingRepoPath}
							name="project_path"
							class="gc-control"
							placeholder="/Users/canh/Projects/repo"
							required
						/>
					</label>
					<SurfaceActionButton
						className="mt-5 w-full"
						tone="accent"
						type="submit"
						disabled={bindingExisting}
					>
						{bindingExisting ? 'Binding repo' : 'Bind existing repo'}
					</SurfaceActionButton>
				</form>

				<form
					class="gc-panel-soft px-4 py-4"
					onsubmit={(event) => {
						event.preventDefault();
						void bindProject('new_project');
					}}
				>
					<p class="gc-stamp">Create a fresh repo</p>
					<label class="mt-4 grid gap-2">
						<span class="gc-stamp text-[var(--gc-text-tertiary)]">New path</span>
						<input
							bind:value={newRepoPath}
							name="new_project_path"
							class="gc-control"
							placeholder="/Users/canh/Projects/new-repo"
							required
						/>
					</label>
					<SurfaceActionButton
						className="mt-5 w-full"
						tone="warning"
						type="submit"
						disabled={bindingNew}
					>
						{bindingNew ? 'Creating repo' : 'Create a fresh repo'}
					</SurfaceActionButton>
				</form>
			</div>
		</section>

		<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<p class="gc-stamp">Preview tasks</p>
			<h2 class="gc-section-title mt-3">Start preview run</h2>
			<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">
				Once a repo is active, use one of these starter moves to open the graph and inspect how the
				machine responds.
			</p>

			<div class="gc-panel-soft mt-6 px-4 py-4">
				<div class="flex flex-wrap items-start justify-between gap-4">
					<div class="min-w-0 flex-1">
						<p class="gc-stamp">Preview readiness</p>
						<p class="gc-panel-title mt-3">{previewState.status_label}</p>
						<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{previewState.detail}</p>
					</div>
					<p class="gc-stamp text-[var(--gc-text-tertiary)]">
						{previewState.available ? 'Ready' : 'Blocked'}
					</p>
				</div>
			</div>

			{#if activeProject}
				<div class="gc-panel-soft mt-6 px-4 py-4">
					<p class="gc-stamp">{onboardingState.completed ? 'Active project' : 'Starter project'}</p>
					<p class="gc-panel-title mt-3">{activeProject.active_name}</p>
					<p class="gc-copy mt-3 break-all text-[var(--gc-text-secondary)]">
						{activeProject.active_path}
					</p>
				</div>
			{/if}

			{#if onboardingState.suggested_tasks.length === 0}
				<div class="gc-panel-soft mt-6 px-4 py-4">
					<p class="gc-stamp">No project yet</p>
					<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
						Bind a repo on the left first. Suggested preview tasks will appear here as soon as
						GistClaw can scan the working tree.
					</p>
				</div>
			{:else}
				<div class="mt-6 grid gap-4">
					{#each onboardingState.suggested_tasks as task (task.kind + task.description)}
						<div class="gc-panel-soft px-4 py-4">
							<div class="flex flex-wrap items-start justify-between gap-4">
								<div class="min-w-0 flex-1">
									<p class="gc-stamp">{task.kind}</p>
									<h3 class="gc-panel-title mt-3">{task.description}</h3>
									<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{task.signal}</p>
								</div>
								<SurfaceActionButton
									className="min-w-52"
									tone="solid"
									disabled={launchingTask === task.description || !previewState.available}
									onclick={() => {
										void startPreview(task.description);
									}}
								>
									{launchingTask === task.description
										? 'Launching preview'
										: previewState.available
											? 'Start preview run'
											: 'Preview unavailable'}
								</SurfaceActionButton>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</section>
	</div>
</div>

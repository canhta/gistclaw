<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolveEntryHref } from '$lib/bootstrap/load';
	import SurfaceActionButton from '$lib/components/common/SurfaceActionButton.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import logo from '$lib/assets/logo.svg';
	import type { AuthLoginResponse } from '$lib/types/api';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	let password = $state('');
	let errorMessage = $state('');
	let submitting = $state(false);

	const params = $derived(new URLSearchParams(data.currentSearch));
	const requestedNext = $derived(params.get('next') ?? '');
	const reason = $derived(params.get('reason') ?? data.auth.login_reason ?? '');
	const reasonMessage = $derived.by(() => {
		switch (reason) {
			case 'expired':
				return 'Your session expired. Sign in again to continue.';
			case 'logged_out':
				return 'You signed out of this browser.';
			case 'blocked':
				return 'This device has been blocked. Use another authorized browser or reset access locally.';
			default:
				return '';
		}
	});

	$effect(() => {
		if (data.auth.authenticated) {
			// eslint-disable-next-line svelte/no-navigation-without-resolve
			void goto(resolveEntryHref(data), { replaceState: true });
		}
	});

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		errorMessage = '';
		submitting = true;

		try {
			const result = await requestJSON<AuthLoginResponse>(fetch, '/api/auth/login', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({
					password,
					next: requestedNext || undefined
				})
			});

			// eslint-disable-next-line svelte/no-navigation-without-resolve
			await goto(result.next, { replaceState: true, invalidateAll: true });
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to sign in right now. Try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Login | GistClaw</title>
</svelte:head>

<div class="flex min-h-screen items-center justify-center px-4 py-8 lg:px-8">
	<div class="grid w-full max-w-6xl gap-6 xl:grid-cols-[minmax(0,1.2fr)_28rem]">
		<section class="gc-panel px-6 py-6 lg:px-8 lg:py-8">
			<div class="flex items-start gap-4">
				<img
					src={logo}
					alt="GistClaw logo"
					class="h-16 w-16 border-2 border-[var(--gc-border-strong)] bg-[var(--gc-canvas)] p-1"
				/>
				<div>
					<p class="gc-stamp">Browser access</p>
					<p class="gc-machine mt-2">GistClaw identity</p>
				</div>
			</div>
			<h1 class="gc-page-title mt-4">Bring the local machine under operator control</h1>
			<p class="gc-copy mt-5 max-w-2xl text-[var(--gc-text-secondary)]">
				Sign in to open the GistClaw control deck. The runtime stays local-first, while this browser
				becomes a hard-edged cockpit for work, recovery, conversations, and history.
			</p>

			<div class="mt-8 grid gap-4 md:grid-cols-3">
				<div class="gc-panel-soft gc-card-accent px-4 py-4">
					<p class="gc-stamp">Work</p>
					<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
						Steer live objectives, graph topology, and active machine signal.
					</p>
				</div>
				<div class="gc-panel-soft px-4 py-4">
					<p class="gc-stamp">Recover</p>
					<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
						Clear approvals, inspect replay evidence, and repair stalled routes.
					</p>
				</div>
				<div class="gc-panel-soft gc-card-warning px-4 py-4">
					<p class="gc-stamp">History</p>
					<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
						Explain what happened without reading through raw machine internals.
					</p>
				</div>
			</div>
		</section>

		<section class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<p class="gc-stamp">Access gate</p>
			<h2 class="gc-panel-title mt-3">Authenticate this browser</h2>

			{#if reasonMessage}
				<div class="gc-panel-soft gc-card-warning mt-5 px-4 py-4">
					<p class="gc-stamp">Session notice</p>
					<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">{reasonMessage}</p>
				</div>
			{/if}

			{#if data.auth.setup_required}
				<div class="gc-panel-soft gc-card-warning mt-5 px-4 py-4">
					<p class="gc-stamp">Setup required</p>
					<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
						Set the browser password from the local CLI before signing in.
					</p>
					<pre
						class="gc-code mt-4 overflow-x-auto border-t-2 border-[var(--gc-border)] pt-4 text-sm text-[var(--gc-ink)]">gistclaw auth set-password</pre>
				</div>
			{:else}
				<form class="mt-5 grid gap-4" onsubmit={submit}>
					<label class="grid gap-2">
						<span class="gc-stamp">Password</span>
						<input
							bind:value={password}
							type="password"
							name="password"
							autocomplete="current-password"
							class="gc-control"
							placeholder="Enter the browser password"
							required
						/>
					</label>

					{#if errorMessage}
						<div class="gc-panel-soft border-[var(--gc-error)] px-4 py-4">
							<p class="gc-stamp text-[var(--gc-error)]">Access error</p>
							<p class="gc-copy mt-3 text-[var(--gc-ink)]">{errorMessage}</p>
						</div>
					{/if}

					<SurfaceActionButton
						type="submit"
						tone="solid"
						className="mt-2 w-full"
						disabled={submitting}
					>
						{submitting ? 'Authenticating' : 'Open control deck'}
					</SurfaceActionButton>
				</form>
			{/if}
		</section>
	</div>
</div>

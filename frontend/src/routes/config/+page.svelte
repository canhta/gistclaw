<script lang="ts">
	import { browser } from '$app/environment';
	import SettingsField from '$lib/components/config/SettingsField.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import SectionTabs from '$lib/components/shell/SectionTabs.svelte';
	import { requestJSON } from '$lib/http/client';
	import type { SettingsActionResponse, SettingsResponse } from '$lib/types/api';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	const tabs = [
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

	let activeTab = $state('general');
	let savedSettings = $state<SettingsResponse | null>(null);
	let approvalMode = $state('');
	let hostAccessMode = $state('');
	let perRunTokenBudget = $state('');
	let dailyCostCapUSD = $state('');
	let telegramBotToken = $state('');
	let lastMachineSignature = $state('');
	let saving = $state(false);
	let saveMessage = $state('');
	let saveError = $state('');
	let rawEditorEl = $state<HTMLDivElement | null>(null);

	const settings = $derived(savedSettings ?? data.config?.settings ?? null);
	const machine = $derived(settings?.machine ?? null);
	const rawDocument = $derived(JSON.stringify(settings ?? {}, null, 2));

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
		<SectionTabs {tabs} bind:activeTab />
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
			<div class="mx-auto flex w-full max-w-5xl flex-col gap-6 px-6 py-6 lg:flex-row">
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
		{:else if activeTab === 'agents'}
			<div class="flex flex-1 items-center justify-center p-10">
				<div class="text-center">
					<p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
					<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Agents &amp; Routing</p>
					<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
						Agent profiles, defaults, and routing rules are not exposed by the web API yet.
					</p>
				</div>
			</div>
		{:else if activeTab === 'models'}
			<div class="flex flex-1 items-center justify-center p-10">
				<div class="text-center">
					<p class="gc-stamp text-[var(--gc-ink-3)]">COMING SOON</p>
					<p class="gc-panel-title mt-3 text-[var(--gc-ink)]">Models</p>
					<p class="gc-copy mt-3 max-w-xs text-[var(--gc-ink-2)]">
						Model inventories and per-role model selection will land here once their API arrives.
					</p>
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

<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import { resolve } from '$app/paths';
	import DeviceAccessCard from '$lib/components/common/DeviceAccessCard.svelte';
	import SurfaceActionButton from '$lib/components/common/SurfaceActionButton.svelte';
	import SurfaceMessage from '$lib/components/common/SurfaceMessage.svelte';
	import { HTTPError, requestJSON } from '$lib/http/client';
	import type {
		SettingsActionResponse,
		SettingsMachineResponse,
		SettingsResponse
	} from '$lib/types/api';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	function cloneInitialSettings(): SettingsResponse {
		return structuredClone(data.settings) as SettingsResponse;
	}

	const initialSettings = cloneInitialSettings();

	let settings = $state<SettingsResponse>(initialSettings);
	let notice = $state('');
	let errorMessage = $state('');
	let savingMachine = $state(false);
	let changingPassword = $state(false);
	let deviceActionKey = $state('');

	let approvalMode = $state(initialSettings.machine.approval_mode);
	let hostAccessMode = $state(initialSettings.machine.host_access_mode);
	let perRunTokenBudget = $state(initialSettings.machine.per_run_token_budget);
	let dailyCostCapUSD = $state(initialSettings.machine.daily_cost_cap_usd);
	let telegramBotToken = $state('');

	let currentPassword = $state('');
	let newPassword = $state('');
	let confirmPassword = $state('');

	function syncMachineDraft(machine: SettingsMachineResponse): void {
		approvalMode = machine.approval_mode;
		hostAccessMode = machine.host_access_mode;
		perRunTokenBudget = machine.per_run_token_budget;
		dailyCostCapUSD = machine.daily_cost_cap_usd;
		telegramBotToken = '';
	}

	function resetPasswordDraft(): void {
		currentPassword = '';
		newPassword = '';
		confirmPassword = '';
	}

	function resolveNavigationTarget(target: string): string {
		if (target === '/login' || target.startsWith('/login?')) {
			return `${resolve('/login')}${target.slice('/login'.length)}`;
		}
		return target;
	}

	async function applyActionResult(result: SettingsActionResponse): Promise<void> {
		if (result.settings) {
			settings = result.settings;
			syncMachineDraft(result.settings.machine);
		}
		if (result.notice) {
			notice = result.notice;
		}
		errorMessage = '';
		if (result.logged_out && result.next) {
			if (typeof window !== 'undefined') {
				window.location.assign(resolveNavigationTarget(result.next));
			}
			return;
		}
		await invalidateAll();
	}

	async function saveMachineSettings(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		savingMachine = true;
		errorMessage = '';

		const payload: Record<string, string> = {
			approval_mode: approvalMode,
			host_access_mode: hostAccessMode,
			per_run_token_budget: perRunTokenBudget,
			daily_cost_cap_usd: dailyCostCapUSD
		};
		if (telegramBotToken.trim() !== '') {
			payload.telegram_bot_token = telegramBotToken.trim();
		}

		try {
			const result = await requestJSON<SettingsActionResponse>(fetch, '/api/settings', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify(payload)
			});
			await applyActionResult(result);
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to update machine posture right now.';
		} finally {
			savingMachine = false;
		}
	}

	async function changePassword(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		changingPassword = true;
		errorMessage = '';

		try {
			const result = await requestJSON<SettingsActionResponse>(fetch, '/api/settings/password', {
				method: 'POST',
				headers: { 'content-type': 'application/json' },
				body: JSON.stringify({
					current_password: currentPassword,
					new_password: newPassword,
					confirm_password: confirmPassword
				})
			});
			resetPasswordDraft();
			await applyActionResult(result);
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to change the password right now.';
		} finally {
			changingPassword = false;
		}
	}

	async function mutateDevice(
		deviceID: string,
		action: 'revoke' | 'block' | 'unblock'
	): Promise<void> {
		deviceActionKey = `${action}:${deviceID}`;
		errorMessage = '';

		try {
			const result = await requestJSON<SettingsActionResponse>(
				fetch,
				`/api/settings/devices/${deviceID}/${action}`,
				{
					method: 'POST'
				}
			);
			await applyActionResult(result);
		} catch (error) {
			errorMessage =
				error instanceof HTTPError ? error.message : 'Unable to update browser access right now.';
		} finally {
			deviceActionKey = '';
		}
	}
</script>

<svelte:head>
	<title>Settings | GistClaw</title>
</svelte:head>

<div class="grid gap-6">
	<section class="grid gap-6 xl:grid-cols-[minmax(0,1.2fr)_minmax(18rem,0.8fr)]">
		<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
			<p class="gc-stamp">Machine posture</p>
			<h2 class="gc-section-title mt-3">
				Operate the machine without forgetting who can still reach it
			</h2>
			<p class="gc-copy mt-4 max-w-3xl text-[var(--gc-text-secondary)]">
				Keep trusted browsers, password posture, budget limits, and the active project explicit in
				one service-manual surface instead of hiding them behind system nouns.
			</p>

			{#if notice}
				<SurfaceMessage label="Settings notice" message={notice} className="mt-5" />
			{/if}

			{#if errorMessage}
				<SurfaceMessage
					label="Settings error"
					message={errorMessage}
					tone="error"
					className="mt-5"
				/>
			{/if}

			<div class="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
				<div class="gc-panel-soft px-4 py-4">
					<p class="gc-stamp">Trusted browsers</p>
					<p class="gc-value mt-3">
						{(settings.access.current_device ? 1 : 0) + settings.access.other_active_devices.length}
					</p>
				</div>
				<div class="gc-panel-soft px-4 py-4">
					<p class="gc-stamp">Blocked browsers</p>
					<p class="gc-value mt-3">{settings.access.blocked_devices.length}</p>
				</div>
				<div class="gc-panel-soft px-4 py-4">
					<p class="gc-stamp">Current root</p>
					<p class="gc-value mt-3">{settings.machine.active_project_name}</p>
				</div>
				<div class="gc-panel-soft px-4 py-4">
					<p class="gc-stamp">24h spend</p>
					<p class="gc-value mt-3">{settings.machine.rolling_cost_label}</p>
				</div>
			</div>
		</div>

		<div class="grid gap-4">
			<div class="gc-panel px-4 py-4">
				<p class="gc-stamp">Active root</p>
				<h3 class="gc-panel-title mt-3 text-[1rem]">{settings.machine.active_project_name}</h3>
				<p class="gc-machine mt-3 break-all">{settings.machine.active_project_path}</p>
			</div>

			<div class="gc-panel-soft px-4 py-4">
				<p class="gc-stamp">Machine posture</p>
				<div class="mt-4 grid gap-3">
					<div>
						<p class="gc-stamp">Approval mode</p>
						<p class="gc-value mt-2">{settings.machine.approval_mode_label}</p>
					</div>
					<div>
						<p class="gc-stamp">Host access</p>
						<p class="gc-value mt-2">{settings.machine.host_access_mode_label}</p>
					</div>
					<div>
						<p class="gc-stamp">Storage root</p>
						<p class="gc-machine mt-2 break-all">{settings.machine.storage_root}</p>
					</div>
				</div>
			</div>
		</div>
	</section>

	<section class="grid gap-6 xl:grid-cols-[minmax(0,1.15fr)_minmax(20rem,0.85fr)]">
		<div class="grid gap-4">
			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<div class="flex flex-wrap items-end justify-between gap-4">
					<div>
						<p class="gc-stamp">Browser access</p>
						<h2 class="gc-section-title mt-3">This browser</h2>
					</div>
					<p class="gc-machine">
						{settings.access.password_configured ? 'Password set' : 'Password missing'}
					</p>
				</div>

				<div class="mt-6">
					{#if settings.access.current_device}
						<DeviceAccessCard
							label="This browser"
							device={settings.access.current_device}
							busy={deviceActionKey.endsWith(`:${settings.access.current_device.id}`)}
							onrevoke={() => {
								void mutateDevice(settings.access.current_device!.id, 'revoke');
							}}
							onblock={() => {
								void mutateDevice(settings.access.current_device!.id, 'block');
							}}
						/>
					{:else}
						<div class="gc-panel-soft px-4 py-4">
							<p class="gc-stamp">This browser</p>
							<p class="gc-copy mt-3 text-[var(--gc-text-secondary)]">
								The current session is not tied to a trusted browser cookie.
							</p>
						</div>
					{/if}
				</div>
			</div>

			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Other signed-in browsers</p>
				<div class="mt-5 grid gap-4">
					{#if settings.access.other_active_devices.length > 0}
						{#each settings.access.other_active_devices as device (device.id)}
							<DeviceAccessCard
								label="Signed-in browser"
								{device}
								busy={deviceActionKey.endsWith(`:${device.id}`)}
								onrevoke={() => {
									void mutateDevice(device.id, 'revoke');
								}}
								onblock={() => {
									void mutateDevice(device.id, 'block');
								}}
							/>
						{/each}
					{:else}
						<div class="gc-panel-soft px-4 py-4">
							<p class="gc-copy text-[var(--gc-text-secondary)]">
								No other browser currently holds a live session.
							</p>
						</div>
					{/if}
				</div>
			</div>

			<div class="gc-panel px-5 py-5 lg:px-6 lg:py-6">
				<p class="gc-stamp">Blocked browsers</p>
				<div class="mt-5 grid gap-4">
					{#if settings.access.blocked_devices.length > 0}
						{#each settings.access.blocked_devices as device (device.id)}
							<DeviceAccessCard
								label="Blocked browser"
								{device}
								busy={deviceActionKey.endsWith(`:${device.id}`)}
								onunblock={() => {
									void mutateDevice(device.id, 'unblock');
								}}
							/>
						{/each}
					{:else}
						<div class="gc-panel-soft px-4 py-4">
							<p class="gc-copy text-[var(--gc-text-secondary)]">
								No blocked browser is waiting for review.
							</p>
						</div>
					{/if}
				</div>
			</div>
		</div>

		<div class="grid gap-4">
			<form class="gc-panel px-5 py-5 lg:px-6 lg:py-6" onsubmit={changePassword}>
				<p class="gc-stamp">Change password</p>
				<h2 class="gc-section-title mt-3">Rotate browser access without touching the daemon</h2>
				<p class="gc-copy mt-4 text-[var(--gc-text-secondary)]">
					Changing the password signs out every other browser and keeps the current machine
					relationship explicit.
				</p>

				<div class="mt-6 grid gap-4">
					<label class="grid gap-2">
						<span class="gc-stamp">Current password</span>
						<input
							type="password"
							bind:value={currentPassword}
							class="border-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] px-4 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-orange)]"
						/>
					</label>
					<label class="grid gap-2">
						<span class="gc-stamp">New password</span>
						<input
							type="password"
							bind:value={newPassword}
							class="border-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] px-4 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-orange)]"
						/>
					</label>
					<label class="grid gap-2">
						<span class="gc-stamp">Confirm password</span>
						<input
							type="password"
							bind:value={confirmPassword}
							class="border-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] px-4 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-orange)]"
						/>
					</label>
				</div>

				<SurfaceActionButton
					type="submit"
					tone="solid"
					className="mt-6"
					disabled={changingPassword}
				>
					{changingPassword ? 'Updating password' : 'Change password'}
				</SurfaceActionButton>
			</form>

			<form class="gc-panel px-5 py-5 lg:px-6 lg:py-6" onsubmit={saveMachineSettings}>
				<p class="gc-stamp">Machine posture</p>
				<h2 class="gc-section-title mt-3">
					Keep deployment facts editable without turning them into navigation
				</h2>

				<div class="mt-6 grid gap-4">
					<label class="grid gap-2">
						<span class="gc-stamp">Approval mode</span>
						<select
							bind:value={approvalMode}
							class="border-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] px-4 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-orange)]"
						>
							<option value="prompt">Prompt</option>
							<option value="auto_approve">Auto approve</option>
						</select>
					</label>

					<label class="grid gap-2">
						<span class="gc-stamp">Host access</span>
						<select
							bind:value={hostAccessMode}
							class="border-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] px-4 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-orange)]"
						>
							<option value="standard">Standard</option>
							<option value="elevated">Elevated</option>
						</select>
					</label>

					<label class="grid gap-2">
						<span class="gc-stamp">Per-run token budget</span>
						<input
							bind:value={perRunTokenBudget}
							class="border-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] px-4 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-orange)]"
						/>
					</label>

					<label class="grid gap-2">
						<span class="gc-stamp">Daily cost cap (USD)</span>
						<input
							bind:value={dailyCostCapUSD}
							class="border-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] px-4 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-orange)]"
						/>
					</label>

					<label class="grid gap-2">
						<span class="gc-stamp">Telegram token</span>
						<input
							bind:value={telegramBotToken}
							placeholder={settings.machine.telegram_token ||
								'Leave blank to keep the current token'}
							class="border-2 border-[var(--gc-border-strong)] bg-[var(--gc-surface-soft)] px-4 py-3 text-[var(--gc-ink)] outline-none focus:border-[var(--gc-orange)]"
						/>
					</label>
				</div>

				<div class="mt-6 grid gap-3 border-t-2 border-[var(--gc-border)] pt-4 sm:grid-cols-2">
					<div>
						<p class="gc-stamp">Admin token</p>
						<p class="gc-machine mt-2 break-all">{settings.machine.admin_token}</p>
					</div>
					<div>
						<p class="gc-stamp">Telegram token</p>
						<p class="gc-machine mt-2 break-all">
							{settings.machine.telegram_token || 'Not configured'}
						</p>
					</div>
				</div>

				<SurfaceActionButton type="submit" tone="solid" className="mt-6" disabled={savingMachine}>
					{savingMachine ? 'Saving machine posture' : 'Save machine posture'}
				</SurfaceActionButton>
			</form>
		</div>
	</section>
</div>

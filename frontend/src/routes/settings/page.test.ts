import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SettingsPage from './+page.svelte';

describe('Settings page', () => {
	it('renders browser access controls and machine posture from the settings surface', () => {
		const { body } = render(SettingsPage, {
			props: {
				data: {
					auth: {
						authenticated: true,
						password_configured: true,
						setup_required: false
					},
					project: {
						active_id: 'proj-primary',
						active_name: 'starter-project',
						active_path: '/tmp/starter-project'
					},
					navigation: [{ id: 'settings', label: 'Settings', href: '/settings' }],
					currentPath: '/settings',
					currentSearch: '',
					settings: {
						machine: {
							storage_root: '/tmp/gistclaw-storage',
							approval_mode: 'auto_approve',
							approval_mode_label: 'Auto approve',
							host_access_mode: 'elevated',
							host_access_mode_label: 'Elevated',
							admin_token: 'test-adm********',
							per_run_token_budget: '50000',
							daily_cost_cap_usd: '3.25',
							rolling_cost_usd: 2.75,
							rolling_cost_label: '$2.75 in the last 24h',
							telegram_token: '87654321***************',
							active_project_name: 'starter-project',
							active_project_path: '/tmp/starter-project',
							active_project_summary: 'starter-project at /tmp/starter-project'
						},
						access: {
							password_configured: true,
							current_device: {
								id: 'dev-current',
								primary_label: 'Chrome on macOS',
								secondary_line: 'Last seen 2026-03-28 10:00 UTC · 1 session active',
								current: true,
								blocked: false,
								active_sessions: 1,
								details_ip: '127.0.0.1',
								details_user_agent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)'
							},
							other_active_devices: [
								{
									id: 'dev-other',
									primary_label: 'Chrome on Windows',
									secondary_line: 'Last seen 2026-03-28 09:30 UTC · 1 session active',
									current: false,
									blocked: false,
									active_sessions: 1,
									details_ip: '127.0.0.2',
									details_user_agent: 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)'
								}
							],
							blocked_devices: [
								{
									id: 'dev-blocked',
									primary_label: 'Firefox on Linux',
									secondary_line: 'Last seen 2026-03-28 08:00 UTC',
									current: false,
									blocked: true,
									active_sessions: 0,
									details_ip: '127.0.0.3',
									details_user_agent: 'Mozilla/5.0 (X11; Linux x86_64)'
								}
							]
						}
					}
				}
			}
		});

		expect(body).toContain('Operate the machine without forgetting who can still reach it');
		expect(body).toContain('This browser');
		expect(body).toContain('Chrome on macOS');
		expect(body).toContain('Other signed-in browsers');
		expect(body).toContain('Change password');
		expect(body).toContain('Machine posture');
		expect(body).toContain('starter-project');
		expect(body).toContain('87654321***************');
	});
});

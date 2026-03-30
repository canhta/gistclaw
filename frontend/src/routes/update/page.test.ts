import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import UpdatePage from './+page.svelte';

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'update', label: 'Update', href: '/update' }],
	onboarding: null,
	currentPath: '/update',
	currentSearch: '',
	updateLoadError: '',
	update: {
		release: {
			version: 'v1.2.3',
			commit: 'abcdef1234567890',
			build_date: '2026-03-29T09:15:00Z',
			build_date_label: '2026-03-29 09:15:00 UTC'
		},
		runtime: {
			started_at: '2026-03-29T09:30:00Z',
			started_at_label: '2026-03-29 09:30:00 UTC',
			uptime_label: '47m',
			active_runs: 2,
			interrupted_runs: 1,
			pending_approvals: 3
		},
		install: {
			config_path: '/etc/gistclaw/config.yaml',
			state_dir: '/var/lib/gistclaw',
			database_dir: '/var/lib/gistclaw',
			storage_root: '/var/lib/gistclaw/storage',
			binary_path: '/usr/local/bin/gistclaw',
			working_directory: '/var/lib/gistclaw',
			service_unit_path: '/etc/systemd/system/gistclaw.service'
		},
		service: {
			restart_policy: 'on-failure',
			unit_preview: '[Unit]\nDescription=GistClaw service\n'
		},
		commands: {
			run_update: [
				{
					id: 'binary-version',
					label: 'Installed binary',
					detail: 'Confirm the installed binary reports the expected release metadata.',
					command: '/usr/local/bin/gistclaw version'
				},
				{
					id: 'service-unit',
					label: 'Inspect service unit',
					detail: 'Confirm the active service unit before restarting the daemon.',
					command: 'systemctl cat gistclaw --no-pager'
				},
				{
					id: 'restart-daemon',
					label: 'Restart daemon',
					detail: 'Restart the shipped service after replacing the binary or config.',
					command: 'sudo systemctl restart gistclaw'
				}
			],
			restart_report: [
				{
					id: 'service-status',
					label: 'Service status',
					detail: 'Verify the service came back cleanly after the restart.',
					command: 'systemctl status gistclaw --no-pager'
				},
				{
					id: 'recent-journal',
					label: 'Recent journal',
					detail: 'Review the most recent daemon boot logs.',
					command: 'journalctl -u gistclaw -n 100 --no-pager'
				},
				{
					id: 'storage-footprint',
					label: 'Storage footprint',
					detail: 'Review state and storage usage after the restart.',
					command: 'du -sh /var/lib/gistclaw/storage'
				}
			]
		},
		storage: {
			database_bytes: 4096,
			wal_bytes: 256,
			free_disk_bytes: 1048576,
			backup_status: 'healthy',
			latest_backup_at_label: '2026-03-29 09:10:00 UTC',
			latest_backup_path: '/var/lib/gistclaw/backups/backup-2026-03-29.db',
			warnings: ['low_disk_space']
		},
		guides: {
			release_notes_url: 'https://github.com/canhta/gistclaw/releases',
			ubuntu_doc_path: 'docs/install-ubuntu.md',
			macos_doc_path: 'docs/install-macos.md',
			recovery_doc_path: 'docs/recovery.md',
			changelog_path: 'CHANGELOG.md'
		}
	}
};

describe('Update page', () => {
	it('renders the Update heading', () => {
		const { body } = render(UpdatePage, { props: { data: baseData } });
		expect(body).toContain('Update');
	});

	it('renders Run Update and Restart Report tabs', () => {
		const { body } = render(UpdatePage, { props: { data: baseData } });
		expect(body).toContain('Run Update');
		expect(body).toContain('Restart Report');
	});

	it('renders maintenance summary cards and project context', () => {
		const { body } = render(UpdatePage, { props: { data: baseData } });
		expect(body).toContain('Release Version');
		expect(body).toContain('v1.2.3');
		expect(body).toContain('Runtime Uptime');
		expect(body).toContain('47m');
		expect(body).toContain('Project');
		expect(body).toContain('my-project');
		expect(body).toContain('/home/user/my-project');
	});

	it('renders the run update maintenance board by default', () => {
		const { body } = render(UpdatePage, { props: { data: baseData } });
		expect(body).toContain('Run the shipped update path');
		expect(body).toContain('GitHub Releases');
		expect(body).toContain('/etc/gistclaw/config.yaml');
		expect(body).toContain('/etc/systemd/system/gistclaw.service');
		expect(body).toContain('Restart policy');
		expect(body).toContain('Operator commands');
		expect(body).toContain('/usr/local/bin/gistclaw version');
		expect(body).toContain('sudo systemctl restart gistclaw');
	});

	it('renders the restart report when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=restart-report' };
		const { body } = render(UpdatePage, { props: { data } });
		expect(body).toContain('Runtime boot report');
		expect(body).toContain('2026-03-29 09:30:00 UTC');
		expect(body).toContain('Pending approvals');
		expect(body).toContain('/var/lib/gistclaw/backups/backup-2026-03-29.db');
		expect(body).toContain('low_disk_space');
		expect(body).toContain('Verification commands');
		expect(body).toContain('journalctl -u gistclaw -n 100 --no-pager');
	});

	it('renders fallback operator commands instead of guidance-only empty states', () => {
		const data = {
			...baseData,
			update: {
				...baseData.update,
				notice: 'Maintenance status source is not wired into this daemon.',
				install: {
					config_path: 'Unavailable',
					state_dir: 'Unavailable',
					database_dir: 'Unavailable',
					storage_root: 'Unavailable',
					binary_path: 'Unavailable',
					working_directory: 'Unavailable',
					service_unit_path: 'Unavailable'
				},
				service: {
					restart_policy: 'unknown',
					unit_preview: 'Unavailable'
				},
				commands: {
					run_update: [
						{
							id: 'path-version',
							label: 'Binary on PATH',
							detail: 'Confirm a gistclaw binary is available before applying an update.',
							command: 'gistclaw version'
						},
						{
							id: 'ubuntu-service-unit',
							label: 'Ubuntu service unit',
							detail:
								'Inspect the shipped systemd unit when this machine uses the Ubuntu install path.',
							command: 'systemctl cat gistclaw --no-pager'
						},
						{
							id: 'homebrew-service-info',
							label: 'Homebrew service info',
							detail:
								'Inspect the managed Homebrew service when this machine uses the macOS install path.',
							command: 'brew services info gistclaw'
						}
					],
					restart_report: [
						{
							id: 'ubuntu-status',
							label: 'Ubuntu service status',
							detail: 'Verify the systemd service came back cleanly after the restart.',
							command: 'systemctl status gistclaw --no-pager'
						},
						{
							id: 'homebrew-status',
							label: 'Homebrew service info',
							detail: 'Verify the Homebrew service state after the restart.',
							command: 'brew services info gistclaw'
						},
						{
							id: 'ubuntu-inspect',
							label: 'Ubuntu runtime inspect',
							detail: 'Inspect the shipped Ubuntu config path after the restart.',
							command: 'gistclaw inspect status --config /etc/gistclaw/config.yaml'
						}
					]
				}
			}
		};
		const { body } = render(UpdatePage, { props: { data } });
		expect(body).toContain('Maintenance status source is not wired into this daemon.');
		expect(body).toContain('Binary on PATH');
		expect(body).toContain('Ubuntu service unit');
		expect(body).toContain('brew services info gistclaw');
		expect(body).not.toContain(
			'Operator commands will appear here when the daemon can report its install paths.'
		);

		const restartData = { ...data, currentSearch: 'tab=restart-report' };
		const { body: restartBody } = render(UpdatePage, { props: { data: restartData } });
		expect(restartBody).toContain('Ubuntu service status');
		expect(restartBody).toContain('gistclaw inspect status --config /etc/gistclaw/config.yaml');
		expect(restartBody).not.toContain(
			'Verification commands will appear here when the daemon can report its runtime'
		);
	});

	it('renders a load error panel instead of fake fallback values', () => {
		const data = {
			...baseData,
			update: null,
			updateLoadError: 'Update status could not be loaded. Reload to retry.'
		};
		const { body } = render(UpdatePage, { props: { data } });
		expect(body).toContain('Update status could not be loaded. Reload to retry.');
		expect(body).toContain('Update board unavailable');
		expect(body).not.toContain('Runtime Uptime');
	});
});

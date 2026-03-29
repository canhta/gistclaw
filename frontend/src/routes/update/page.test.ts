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

	it('renders the fallback notice without hiding the update board', () => {
		const data = {
			...baseData,
			update: {
				...baseData.update,
				notice: 'Maintenance status source is not wired into this daemon.'
			}
		};
		const { body } = render(UpdatePage, { props: { data } });
		expect(body).toContain('Maintenance status source is not wired into this daemon.');
		expect(body).toContain('Run the shipped update path');
	});
});

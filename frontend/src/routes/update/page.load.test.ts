import { describe, expect, it, vi } from 'vitest';
import { load } from './+page';

function makeLoadEvent(fetcher: typeof fetch): Parameters<typeof load>[0] {
	return {
		fetch: fetcher,
		url: new URL('http://localhost/update')
	} as unknown as Parameters<typeof load>[0];
}

describe('update load', () => {
	it('loads the maintenance status snapshot', async () => {
		const fetcher = vi.fn<typeof fetch>(async (input) => {
			expect(String(input)).toBe('/api/update');

			return new Response(
				JSON.stringify({
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
						unit_preview: '[Unit]\\nDescription=GistClaw service\\n'
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
				}),
				{ status: 200, headers: { 'content-type': 'application/json' } }
			);
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected update load to return data');
		}

		expect(result.update.release.version).toBe('v1.2.3');
		expect(result.update.runtime.pending_approvals).toBe(3);
		expect(result.update.install.config_path).toBe('/etc/gistclaw/config.yaml');
		expect(result.update.commands.run_update[0]?.command).toBe('/usr/local/bin/gistclaw version');
		expect(result.update.commands.restart_report[0]?.label).toBe('Service status');
	});

	it('returns a load error when the update request fails', async () => {
		const fetcher = vi.fn<typeof fetch>(async () => {
			throw new Error('boom');
		});

		const result = await load(makeLoadEvent(fetcher));

		if (!result) {
			throw new Error('expected update load to return error data');
		}

		expect(result.update).toBeNull();
		expect(result.updateLoadError).toBe('Update status could not be loaded. Reload to retry.');
	});
});

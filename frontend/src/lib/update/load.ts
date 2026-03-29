import { requestJSON } from '$lib/http/client';
import type { UpdateStatusResponse } from '$lib/types/api';

export async function loadUpdateStatus(fetcher: typeof fetch): Promise<UpdateStatusResponse> {
	return requestJSON<UpdateStatusResponse>(fetcher, '/api/update');
}

export function fallbackUpdateStatus(
	notice = 'Update status could not be loaded. Reload to retry.'
): UpdateStatusResponse {
	return {
		notice,
		release: {
			version: 'unknown',
			commit: 'unknown',
			build_date: 'unknown',
			build_date_label: 'unknown'
		},
		runtime: {
			started_at: '',
			started_at_label: 'Unavailable',
			uptime_label: 'Unavailable',
			active_runs: 0,
			interrupted_runs: 0,
			pending_approvals: 0
		},
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
			run_update: [],
			restart_report: []
		},
		storage: {
			database_bytes: 0,
			wal_bytes: 0,
			free_disk_bytes: 0,
			backup_status: 'unknown',
			latest_backup_at_label: '',
			latest_backup_path: '',
			warnings: []
		},
		guides: {
			release_notes_url: 'https://github.com/canhta/gistclaw/releases',
			ubuntu_doc_path: 'docs/install-ubuntu.md',
			macos_doc_path: 'docs/install-macos.md',
			recovery_doc_path: 'docs/recovery.md',
			changelog_path: 'CHANGELOG.md'
		}
	};
}

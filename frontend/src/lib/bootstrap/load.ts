import { requestJSON } from '$lib/http/client';
import type {
	AuthSessionResponse,
	BootstrapNavItem,
	BootstrapProjectResponse,
	BootstrapResponse
} from '$lib/types/api';

export interface AppShellState {
	auth: AuthSessionResponse;
	project: BootstrapProjectResponse | null;
	navigation: BootstrapNavItem[];
}

export async function loadAppShell(fetcher: typeof fetch): Promise<AppShellState> {
	const auth = await requestJSON<AuthSessionResponse>(fetcher, '/api/auth/session');

	if (!auth.authenticated) {
		return {
			auth,
			project: null,
			navigation: []
		};
	}

	const bootstrap = await requestJSON<BootstrapResponse>(fetcher, '/api/bootstrap');

	return {
		auth: bootstrap.auth,
		project: bootstrap.project,
		navigation: bootstrap.navigation
	};
}

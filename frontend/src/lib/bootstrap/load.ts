import { requestJSON } from '$lib/http/client';
import type {
	AuthSessionResponse,
	BootstrapNavItem,
	BootstrapOnboardingResponse,
	BootstrapProjectResponse,
	BootstrapResponse
} from '$lib/types/api';

export interface AppShellState {
	auth: AuthSessionResponse;
	onboarding: BootstrapOnboardingResponse | null;
	project: BootstrapProjectResponse | null;
	navigation: BootstrapNavItem[];
}

export function resolveEntryHref(state: Pick<AppShellState, 'auth' | 'onboarding'>): string {
	if (!state.auth.authenticated) {
		return '/login';
	}
	return state.onboarding?.entry_href ?? '/chat';
}

export async function loadAppShell(fetcher: typeof fetch): Promise<AppShellState> {
	const auth = await requestJSON<AuthSessionResponse>(fetcher, '/api/auth/session');

	if (!auth.authenticated) {
		return {
			auth,
			onboarding: null,
			project: null,
			navigation: []
		};
	}

	const bootstrap = await requestJSON<BootstrapResponse>(fetcher, '/api/bootstrap');

	return {
		auth: bootstrap.auth,
		onboarding: bootstrap.onboarding,
		project: bootstrap.project,
		navigation: bootstrap.navigation
	};
}

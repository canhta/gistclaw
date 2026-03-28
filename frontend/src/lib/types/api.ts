export interface AuthSessionResponse {
	authenticated: boolean;
	password_configured: boolean;
	setup_required: boolean;
	login_reason?: string;
	device_id?: string;
}

export interface AuthLoginResponse {
	authenticated: boolean;
	next: string;
}

export interface BootstrapProjectResponse {
	active_id: string;
	active_name: string;
	active_path: string;
}

export interface BootstrapNavItem {
	id: string;
	label: string;
	href: string;
}

export interface BootstrapResponse {
	auth: AuthSessionResponse;
	project: BootstrapProjectResponse;
	navigation: BootstrapNavItem[];
}

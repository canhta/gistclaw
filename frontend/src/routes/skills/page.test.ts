import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SkillsPage from './+page.svelte';

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: [{ id: 'skills', label: 'Skills', href: '/skills' }],
	onboarding: null,
	currentPath: '/skills',
	currentSearch: '',
	skillsLoadError: '',
	skills: {
		summary: {
			shipped_surfaces: 6,
			configured_surfaces: 4,
			installed_tools: 18,
			ready_credentials: 4,
			missing_credentials: 1
		},
		surfaces: [
			{
				id: 'anthropic',
				name: 'Anthropic',
				kind: 'provider',
				configured: true,
				active: true,
				credential_state: 'ready',
				credential_state_label: 'ready',
				summary: 'Primary provider is configured.',
				detail: 'cheap claude-3-haiku · strong claude-sonnet'
			},
			{
				id: 'openai',
				name: 'OpenAI-compatible',
				kind: 'provider',
				configured: false,
				active: false,
				credential_state: 'missing',
				credential_state_label: 'missing',
				summary: 'Available in this build.',
				detail: 'Switch provider.name to openai to use compatible endpoints.'
			},
			{
				id: 'telegram',
				name: 'Telegram',
				kind: 'connector',
				configured: true,
				active: true,
				credential_state: 'ready',
				credential_state_label: 'ready',
				summary: 'Bot token configured and connector booted.',
				detail: 'Front agent assistant'
			},
			{
				id: 'tavily',
				name: 'Tavily research',
				kind: 'research',
				configured: true,
				active: true,
				credential_state: 'ready',
				credential_state_label: 'ready',
				summary: 'web_search is registered.',
				detail: 'Timeout 20s'
			},
			{
				id: 'filesystem',
				name: 'filesystem',
				kind: 'mcp',
				configured: true,
				active: true,
				credential_state: 'operator_managed',
				credential_state_label: 'operator managed',
				summary: '2 MCP tools enabled.',
				detail: 'stdio · uvx mcp-server-filesystem'
			}
		],
		tools: [
			{
				name: 'list_dir',
				family: 'repo',
				risk: 'low',
				approval: '',
				side_effect: 'read',
				description: 'List directories in the repo.'
			},
			{
				name: 'web_search',
				family: 'research',
				risk: 'low',
				approval: '',
				side_effect: 'read',
				description: 'Search the web through Tavily.'
			},
			{
				name: 'connector_send',
				family: 'connector',
				risk: 'medium',
				approval: 'required',
				side_effect: 'connector_send',
				description: 'Send a direct message through a registered connector target.'
			}
		]
	}
};

describe('Skills page', () => {
	it('renders the Skills heading', () => {
		const { body } = render(SkillsPage, { props: { data: baseData } });
		expect(body).toContain('Skills');
	});

	it('renders Installed, Available, Credentials tabs', () => {
		const { body } = render(SkillsPage, { props: { data: baseData } });
		expect(body).toContain('Installed');
		expect(body).toContain('Available');
		expect(body).toContain('Credentials');
	});

	it('renders extension summary cards and project context', () => {
		const { body } = render(SkillsPage, { props: { data: baseData } });
		expect(body).toContain('Shipped Surfaces');
		expect(body).toContain('6');
		expect(body).toContain('Installed Tools');
		expect(body).toContain('18');
		expect(body).toContain('Project');
		expect(body).toContain('my-project');
		expect(body).toContain('/home/user/my-project');
	});

	it('renders installed extension inventory by default', () => {
		const { body } = render(SkillsPage, { props: { data: baseData } });
		expect(body).toContain('Configured extension inventory');
		expect(body).toContain('Anthropic');
		expect(body).toContain('Telegram');
		expect(body).toContain('connector_send');
	});

	it('renders shipped availability when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=available' };
		const { body } = render(SkillsPage, { props: { data } });
		expect(body).toContain('Available in this build');
		expect(body).toContain('OpenAI-compatible');
		expect(body).toContain('filesystem');
		expect(body).toContain('Switch provider.name to openai');
	});

	it('renders credential readiness when selected through search', () => {
		const data = { ...baseData, currentSearch: 'tab=credentials' };
		const { body } = render(SkillsPage, { props: { data } });
		expect(body).toContain('Credential readiness');
		expect(body).toContain('Telegram');
		expect(body).toContain('ready');
		expect(body).toContain('OpenAI-compatible');
		expect(body).toContain('missing');
	});

	it('renders a load error panel when skills status is unavailable', () => {
		const data = {
			...baseData,
			skills: null,
			skillsLoadError: 'Skills status could not be loaded. Reload to retry.'
		};
		const { body } = render(SkillsPage, { props: { data } });
		expect(body).toContain('Skills status could not be loaded. Reload to retry.');
		expect(body).toContain('Skills board unavailable');
		expect(body).not.toContain('Configured extension inventory');
	});
});

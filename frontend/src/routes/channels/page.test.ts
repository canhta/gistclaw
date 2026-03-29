import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import ChannelsPage from './+page.svelte';

const nav = [{ id: 'channels', label: 'Channels', href: '/channels' }];

const baseData = {
	auth: { authenticated: true, password_configured: true, setup_required: false },
	project: { active_id: 'p1', active_name: 'my-project', active_path: '/home/user/my-project' },
	navigation: nav,
	onboarding: null,
	currentPath: '/channels',
	currentSearch: '',
	channels: { connectors: [], deliveryHealth: [] }
};

describe('Channels page', () => {
	it('renders the Channels heading', () => {
		const { body } = render(ChannelsPage, { props: { data: baseData } });
		expect(body).toContain('Channels');
	});

	it('renders Status, Login, Settings tabs', () => {
		const { body } = render(ChannelsPage, { props: { data: baseData } });
		expect(body).toContain('Status');
		expect(body).toContain('Login');
		expect(body).toContain('Settings');
	});

	it('renders empty state when no connectors', () => {
		const { body } = render(ChannelsPage, { props: { data: baseData } });
		expect(body).toContain('No channels connected');
	});

	it('renders connector row when connectors are provided', () => {
		const data = {
			...baseData,
			channels: {
				connectors: [
					{
						connector_id: 'telegram',
						state: 'active',
						state_label: 'Active',
						state_class: 'is-success',
						summary: 'Bot is connected',
						checked_at_label: '2 min ago',
						restart_suggested: false
					}
				],
				deliveryHealth: []
			}
		};
		const { body } = render(ChannelsPage, { props: { data } });
		expect(body).toContain('telegram');
		expect(body).toContain('Bot is connected');
	});
});

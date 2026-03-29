import { render } from 'svelte/server';
import { describe, expect, it } from 'vitest';
import SettingsField from './SettingsField.svelte';

describe('SettingsField', () => {
	it('renders the label', () => {
		const { body } = render(SettingsField, {
			props: { id: 'f1', label: 'Token Budget', value: '50000', type: 'text' }
		});
		expect(body).toContain('Token Budget');
	});

	it('renders the current value in the input', () => {
		const { body } = render(SettingsField, {
			props: { id: 'f1', label: 'Token Budget', value: '50000', type: 'text' }
		});
		expect(body).toContain('50000');
	});

	it('renders a hint when provided', () => {
		const { body } = render(SettingsField, {
			props: {
				id: 'f1',
				label: 'Token Budget',
				value: '',
				type: 'text',
				hint: 'Max tokens per run'
			}
		});
		expect(body).toContain('Max tokens per run');
	});

	it('renders a select element when options are provided', () => {
		const { body } = render(SettingsField, {
			props: {
				id: 'f1',
				label: 'Approval Mode',
				value: 'on_request',
				type: 'select',
				options: [
					{ value: 'on_request', label: 'On Request' },
					{ value: 'always', label: 'Always' }
				]
			}
		});
		expect(body).toContain('On Request');
		expect(body).toContain('Always');
	});
});

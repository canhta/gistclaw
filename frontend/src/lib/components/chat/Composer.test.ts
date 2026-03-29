import { render } from 'svelte/server';
import { describe, expect, it, vi } from 'vitest';
import Composer from './Composer.svelte';

describe('Composer', () => {
	it('renders the textarea and Send button when idle', () => {
		const { body } = render(Composer, {
			props: {
				runStatus: 'idle',
				canInject: false,
				onSend: vi.fn(),
				onInject: vi.fn(),
				onStop: vi.fn()
			}
		});
		expect(body).toContain('textarea');
		expect(body).toContain('SEND');
		expect(body).not.toContain('STOP');
	});

	it('renders STOP button instead of SEND when run is active', () => {
		const { body } = render(Composer, {
			props: {
				runStatus: 'active',
				canInject: true,
				onSend: vi.fn(),
				onInject: vi.fn(),
				onStop: vi.fn()
			}
		});
		expect(body).toContain('STOP');
		expect(body).not.toContain('>SEND<');
	});

	it('disables the Send button when input is empty and idle', () => {
		const { body } = render(Composer, {
			props: {
				runStatus: 'idle',
				canInject: false,
				onSend: vi.fn(),
				onInject: vi.fn(),
				onStop: vi.fn()
			}
		});
		// disabled attr or aria-disabled on the send button
		expect(body).toMatch(/disabled|aria-disabled="true"/);
	});

	it('keeps the textarea enabled for active runs so notes can be injected', () => {
		const { body } = render(Composer, {
			props: {
				runStatus: 'active',
				canInject: true,
				onSend: vi.fn(),
				onInject: vi.fn(),
				onStop: vi.fn()
			}
		});
		expect(body).toContain('Type a message');
		expect(body).toContain('INJECT');
		expect(body).not.toMatch(/Inject notes coming soon/);
		expect(body).toContain('Inject note into the selected run');
		expect(body).not.toMatch(/textarea[^>]*disabled/);
	});
});

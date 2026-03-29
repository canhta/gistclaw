import { describe, expect, it } from 'vitest';
import config from '../../../vite.config';

describe('dev proxy', () => {
	it('forwards the original browser host for api requests', () => {
		const proxy = config.server?.proxy?.['/api'];

		expect(proxy).toBeTypeOf('object');
		expect(proxy).toMatchObject({
			changeOrigin: true,
			xfwd: true
		});
	});
});

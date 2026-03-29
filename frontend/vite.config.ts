import { defineConfig } from 'vitest/config';
import tailwindcss from '@tailwindcss/vite';
import { sveltekit } from '@sveltejs/kit/vite';

const apiOrigin = process.env.VITE_GISTCLAW_API_ORIGIN ?? 'http://127.0.0.1:8080';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		host: '127.0.0.1',
		port: 5173,
		strictPort: true,
		proxy: {
			'/api': {
				target: apiOrigin,
				changeOrigin: true,
				xfwd: true
			}
		}
	},
	test: {
		expect: { requireAssertions: true },
		projects: [
			{
				extends: './vite.config.ts',
				test: {
					name: 'server',
					environment: 'node',
					include: ['src/**/*.{test,spec}.{js,ts}'],
					exclude: ['src/**/*.svelte.{test,spec}.{js,ts}']
				}
			}
		]
	}
});

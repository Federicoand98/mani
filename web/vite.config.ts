/// <reference types="vitest/config" />
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

// The console is a single-page app served by mani itself: no server-side
// rendering, every route falls back to index.html, and mani serves build/.
export default defineConfig(({ mode }) => ({
	resolve: mode === 'test' ? { conditions: ['browser'] } : undefined,
	plugins: [sveltekit({ adapter: adapter({ fallback: 'index.html' }) })],
	server: {
		// In development, start `mani web --no-open`, open its link once to get the
		// session cookie, then work on http://127.0.0.1:5173.
		proxy: { '/api': 'http://127.0.0.1:7878' }
	},
	test: {
		include: ['src/**/*.test.ts'],
		environment: 'jsdom'
	}
}));

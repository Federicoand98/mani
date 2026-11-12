import { defineConfig } from '@playwright/test';

// End-to-end tests run against the real mani binary, with the console embedded:
// build it first with `make mani` (or point MANI_BIN at one).
export default defineConfig({
	testDir: 'e2e',
	fullyParallel: true,
	reporter: process.env.CI ? 'github' : 'list',
	use: {
		// Locally, the installed Chrome; in CI, the Chromium that `playwright install` fetched.
		channel: process.env.CI ? undefined : 'chrome',
		trace: 'retain-on-failure'
	}
});

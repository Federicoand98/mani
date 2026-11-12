// Screenshots for a visual pass, not a test: node e2e/screens.mjs <link> <ollama> <outdir>
import { chromium } from '@playwright/test';
const [link, ollama, out] = process.argv.slice(2);
const base = new URL(link).origin;
const browser = await chromium.launch({ channel: 'chrome' });
for (const [scheme, vw] of [['light', { width: 1360, height: 860 }], ['dark', { width: 1360, height: 860 }], ['light', { width: 390, height: 800 }]]) {
	const ctx = await browser.newContext({ viewport: vw, colorScheme: scheme });
	const page = await ctx.newPage();
	await page.goto(scheme === 'light' && vw.width > 400 ? link : base);
	if (scheme === 'light' && vw.width > 400) {
		const cookies = await ctx.cookies();
		globalThis.cookies = cookies;
	} else await ctx.addCookies(globalThis.cookies);
	await page.goto(base + '/');
	await page.waitForTimeout(300);
	await page.screenshot({ path: `${out}/home-${scheme}-${vw.width}.png`, fullPage: true });
	await page.goto(base + '/providers');
	await page.waitForTimeout(300);
	await page.screenshot({ path: `${out}/providers-${scheme}-${vw.width}.png`, fullPage: true });
	await page.getByRole('button', { name: 'Add provider' }).first().click();
	await page.waitForTimeout(200);
	await page.screenshot({ path: `${out}/add-${scheme}-${vw.width}.png` });
	if (scheme === 'light' && vw.width > 400) {
		await page.getByRole('button', { name: /Local · Ollama/ }).click();
		await page.getByLabel('Address').fill(ollama);
		await page.getByRole('button', { name: 'Find Ollama' }).click();
		await page.waitForTimeout(300);
		await page.screenshot({ path: `${out}/add-ollama-found.png` });
		await page.getByRole('button', { name: 'Add', exact: true }).click();
		await page.waitForTimeout(500);
		await page.screenshot({ path: `${out}/providers-filled.png`, fullPage: true });
	}
	await page.goto(base + '/settings');
	await page.waitForTimeout(200);
	await page.screenshot({ path: `${out}/settings-${scheme}-${vw.width}.png`, fullPage: true });
	await ctx.close();
}
await browser.close();

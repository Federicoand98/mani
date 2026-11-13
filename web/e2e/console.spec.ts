import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from './fixtures.ts';

test('without the link, the console asks to sign in from the terminal', async ({ page, mani }) => {
	await page.goto(mani.url);
	await expect(page.getByRole('heading', { name: 'Sign in from the terminal' })).toBeVisible();
});

test('the link signs in once and leaves no token in the address bar', async ({ page, mani }) => {
	await page.goto(mani.link);
	await expect(page).toHaveURL(mani.url + '/');
	await expect(page.getByRole('heading', { name: 'Get started' })).toBeVisible();
	await expect(page.getByText(mani.home).first()).toBeVisible();

	const other = await page.context().browser()!.newContext();
	const p2 = await other.newPage();
	await p2.goto(mani.link);
	await expect(p2.getByText('This link has already been used')).toBeVisible();
	await other.close();
});

test('add Ollama, see its models, then remove it', async ({ page, mani }) => {
	await page.goto(mani.link);
	await page.getByRole('link', { name: 'Providers' }).click();
	await expect(page.getByRole('heading', { name: 'No provider connected' })).toBeVisible();

	await page.getByRole('button', { name: 'Add provider' }).click();
	await page.getByRole('button', { name: /Local - Ollama/ }).click();
	await page.getByLabel('Address').fill(mani.ollama);
	await page.getByRole('button', { name: 'Find Ollama' }).click();
	await expect(page.getByText('Found. 2 models already downloaded')).toBeVisible();
	await page.getByRole('button', { name: 'Add', exact: true }).click();

	await expect(page.getByText('Ollama added')).toBeVisible();
	const row = page.getByRole('row', { name: /Ollama/ });
	await expect(row.getByText('2 models')).toBeVisible();
	await expect(row.getByText('qwen3.5:9b')).toBeVisible();

	const cfg = JSON.parse(readFileSync(join(mani.home, 'config/mani/config.json'), 'utf8'));
	expect(cfg.providers.ollama).toMatchObject({ enabled: true, base_url: mani.ollama });

	await row.getByRole('button', { name: 'Test' }).click();
	await expect(row.getByText('Connected - 2 models')).toBeVisible();

	await row.getByRole('button', { name: 'Remove' }).click();
	await row.getByRole('button', { name: 'Confirm' }).click();
	await expect(page.getByRole('heading', { name: 'No provider connected' })).toBeVisible();
});

test('a refused key is reported and nothing is saved', async ({ page, mani }) => {
	await page.goto(mani.link);
	await page.goto(mani.url + '/providers');
	await page.getByRole('button', { name: 'Add provider' }).click();
	await page.getByRole('button', { name: /OpenAI/ }).click();

	await page.getByRole('button', { name: 'Test and add' }).click();
	await expect(page.getByText('Enter the API key.')).toBeVisible();

	await page.getByLabel('API key').fill('sk-wrong');
	await page.locator('summary', { hasText: 'Custom endpoint' }).click();
	await page.locator('#ap-endpoint').fill(mani.ollama); // answers 404 to /models
	await page.getByRole('button', { name: 'Test and add' }).click();
	await expect(page.getByRole('alert')).toContainText('OpenAI refused the connection');

	await page.getByRole('button', { name: 'Back' }).click();
	await page.keyboard.press('Escape');
	await expect(page.getByRole('heading', { name: 'No provider connected' })).toBeVisible();
	expect(existsSync(join(mani.home, 'data/mani/auth.json'))).toBe(false);
});

test('the theme choice survives a reload of a deep route', async ({ page, mani }) => {
	await page.goto(mani.link);
	await page.getByRole('link', { name: 'Settings' }).click();
	await page.getByRole('button', { name: 'Dark' }).click();
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');

	await page.reload();
	await expect(page.getByRole('heading', { name: 'Settings', level: 1 })).toBeVisible();
	await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark');
	await expect(page.getByRole('button', { name: 'Dark' })).toHaveAttribute('aria-pressed', 'true');
});

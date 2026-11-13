import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, signInWithOllama, test } from './fixtures.ts';

test('from a template to a working, saved agent, with a trial on the way', async ({ page, mani }) => {
	writeFileSync(join(mani.home, 'notes.txt'), 'buy more coffee\n');
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/agents');
	await page.getByRole('link', { name: 'New agent' }).first().click();

	await page.getByRole('button', { name: /Documents assistant/ }).click();
	await page.locator('#f-name').fill('handbook');
	await page.getByRole('button', { name: 'Continue' }).click(); // tools
	await page.getByRole('button', { name: 'Continue' }).click(); // permissions
	await page.getByRole('button', { name: 'Continue' }).click(); // limits
	await page.getByRole('button', { name: 'Continue' }).click(); // check
	await expect(page.getByText('The manifest is valid.')).toBeVisible();
	await expect(page.getByText('handbook.yaml', { exact: true })).toBeVisible();

	await page.getByRole('button', { name: 'Try it' }).click();
	await page.getByLabel('Task for the trial').fill('please read notes');
	await page.keyboard.press('Enter');
	await expect(page.getByText('The tool said: buy more coffee')).toBeVisible();
	await expect(page.getByText('Your agent works.')).toBeVisible();
	await page.getByRole('button', { name: 'Save', exact: true }).click();

	await expect(page.getByRole('heading', { name: 'Your agent works' })).toBeVisible();
	const file = readFileSync(join(mani.home, 'handbook.yaml'), 'utf8');
	expect(file).toContain('identity:                    # who thinks?');
	expect(file).toContain('  name: handbook');
	// The trial left nothing behind.
	expect(existsSync(join(mani.home, 'runs'))).toBe(false);

	await page.getByRole('link', { name: 'Open in chat' }).click();
	await expect(page.getByRole('heading', { name: 'Talk with handbook' })).toBeVisible();
});

test('the draft waits for you after a reload, until Start over', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/agents/new');
	await page.getByRole('button', { name: /Classifier/ }).click();
	await page.locator('#f-name').fill('mood');
	await page.getByRole('button', { name: 'Continue' }).click();

	await page.reload();
	await expect(page.getByText('Your draft is back where you left it.')).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Tools', level: 2 })).toBeVisible();
	await page.getByRole('button', { name: 'Back' }).click();
	await expect(page.locator('#f-name')).toHaveValue('mood');

	await page.getByRole('button', { name: 'Start over' }).first().click();
	await expect(page.getByRole('button', { name: /Documents assistant/ })).toBeVisible();
	await page.reload();
	await expect(page.getByText('Your draft is back where you left it.')).toHaveCount(0);
});

test('a name already taken is refused before going on', async ({ page, mani }) => {
	writeFileSync(join(mani.home, 'docs-assistant.yaml'), 'identity:\n  name: docs-assistant\n');
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/agents/new');
	await page.getByRole('button', { name: /Documents assistant/ }).click();
	await expect(page.getByText('docs-assistant.yaml already exists: pick another name.')).toBeVisible();
	await expect(page.getByRole('button', { name: 'Continue' })).toBeDisabled();
	await page.locator('#f-name').fill('docs-two');
	await expect(page.getByRole('button', { name: 'Continue' })).toBeEnabled();
});

test('import a YAML: saved as it is and opened in the editor', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/agents/new');
	await page.getByRole('button', { name: /Import YAML/ }).click();
	const yaml = '# kept as written\nidentity:\n  name: imported\n  provider: ollama\n  model: qwen3.5:9b\n';
	await page.getByLabel('Or paste it').fill(yaml);
	await page.getByLabel('Or paste it').blur();
	await expect(page.getByText('The manifest is valid.')).toBeVisible();
	await page.getByRole('button', { name: 'Save and open' }).click();
	await expect(page).toHaveURL(mani.url + '/agents/imported.yaml');
	await expect(page.getByRole('heading', { name: 'imported', level: 1 })).toBeVisible();
	expect(readFileSync(join(mani.home, 'imported.yaml'), 'utf8')).toBe(yaml);
});

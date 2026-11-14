import { existsSync, readFileSync, writeFileSync, mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { parse } from 'yaml';
import { expect, signInWithOllama, test } from './fixtures.ts';

test('from a template to a working, saved agent, with a trial on the way', async ({ page, mani }) => {
	writeFileSync(join(mani.home, 'notes.txt'), 'buy more coffee\n');
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/agents');
	await page.getByRole('link', { name: 'New agent' }).first().click();

	await page.getByRole('button', { name: /Documents assistant/ }).click();
	await page.locator('#f-name').fill('handbook');
	await expect(page.locator('select#f-model option[value="llama3.2:3b"]')).toHaveCount(1);
	await page.locator('#f-model').selectOption('llama3.2:3b');
	await page.getByRole('button', { name: 'Start trial' }).click();
	await page.getByLabel('Task for the trial').fill('please read notes');
	await page.keyboard.press('Enter');
	await expect(page.getByText('The tool said: buy more coffee')).toBeVisible();
	await expect(page.getByText('Your agent works.')).toBeVisible();
	await page.screenshot({ path: 'test-results/workbench-real-trial.png' });
	await page.getByRole('button', { name: 'Create agent', exact: true }).click();

	await expect(page.getByRole('heading', { name: 'Your agent works' })).toBeVisible();
	const file = readFileSync(join(mani.home, 'handbook.yaml'), 'utf8');
	expect(file).toContain('identity:                    # who thinks?');
	expect(file).toContain('  name: handbook');
	expect(parse(file).identity.model).toBe('llama3.2:3b');
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
	await page.getByRole('tab', { name: 'Tools', exact: true }).click();

	await page.reload();
	await expect(page.getByText('Your draft is back where you left it.')).toBeVisible();
	await expect(page.getByRole('tab', { name: 'Tools', exact: true })).toHaveAttribute('aria-selected', 'true');
	await page.getByRole('tab', { name: 'Agent', exact: true }).click();
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
	await expect(page.getByRole('button', { name: 'Create agent' })).toBeDisabled();
	await page.locator('#f-name').fill('docs-two');
	await expect(page.getByRole('button', { name: 'Create agent' })).toBeEnabled();
});

test('workbench uses Markdown, native Browse, segmented policy and hidden advanced fields without losing values', async ({ page, mani }) => {
	const outside = mkdtempSync(join(tmpdir(), 'mani-native-workbench-'));
	try {
		await page.setViewportSize({ width: 1440, height: 1000 });
		writeFileSync(join(outside, 'notes.txt'), 'outside workspace notes\n');
		await signInWithOllama(page, mani);
		await page.goto(mani.url + '/agents/new');
		await page.getByRole('button', { name: /Documents assistant/ }).click();
		await page.locator('#f-name').fill('workbench');
		await expect(page.getByRole('tab', { name: 'Output', exact: true })).toHaveCount(0);
		await expect(page.locator('details')).toHaveCount(0);
		await page.route('**/api/folders/pick', async (route) => {
			expect(route.request().method()).toBe('POST');
			await route.fulfill({ json: { path: outside, cancelled: false } });
		});
		await page.getByRole('button', { name: 'Browse', exact: true }).click();
		await expect(page.locator('#f-ws')).toHaveValue(outside);
		const prompt = '## Rules\nFirst line\nSecond line\n\n- Cite sources\n\n```text\n  indented\n```\n\n';
		await page.locator('#f-prompt').fill(prompt);
		await page.getByRole('button', { name: 'Preview', exact: true }).click();
		await expect(page.getByRole('region', { name: 'Instructions preview' }).getByRole('heading', { name: 'Rules' })).toBeVisible();
		await expect(page.locator('.preview p br')).toHaveCount(1);
		await page.getByRole('button', { name: 'Write', exact: true }).click();
		await expect(page.locator('#f-prompt')).toHaveValue(prompt);
		await expect(page.getByRole('button', { name: 'Create agent' })).toBeEnabled();
		await page.locator('#f-prompt').evaluate((el) => { el.scrollTop = 0; });
		await page.screenshot({ path: 'test-results/workbench-desktop.png' });
		const splitter = page.getByRole('separator', { name: 'Resize configuration and chat' });
		const before = await splitter.boundingBox();
		expect(before).not.toBeNull();
		await page.mouse.move(before!.x + 6, before!.y + 60);
		await page.mouse.down(); await page.mouse.move(before!.x + 76, before!.y + 60, { steps: 6 }); await page.mouse.up();
		const after = await splitter.boundingBox();
		expect(after!.x - before!.x).toBeGreaterThan(60);
		await splitter.focus(); await page.keyboard.press('ArrowLeft');
		expect((await splitter.boundingBox())!.x).toBeLessThan(after!.x - 20);
		await page.getByRole('tab', { name: 'Policy', exact: true }).click();
		await page.getByRole('group', { name: 'Access for write', exact: true }).getByRole('button', { name: 'Deny', exact: true }).click();
		await page.screenshot({ path: 'test-results/workbench-policy.png' });
		await page.getByRole('button', { name: 'Advanced options' }).click();
		await expect(page.getByRole('tab', { name: 'Output', exact: true })).toBeVisible();
		await page.getByLabel('Response format').selectOption('json');
		await page.getByLabel('Response schema (JSON)').fill('{"type":"object","properties":{"answer":{"type":"string"}},"required":["answer"]}');
		await page.getByRole('tab', { name: 'Limits', exact: true }).click();
		await page.locator('[data-path="limits.max_tokens"]').fill('12345');
		await page.getByRole('button', { name: 'Agent settings' }).click();
		await page.getByRole('tab', { name: 'Agent', exact: true }).click();
		await expect(page.locator('#f-prompt')).toHaveValue(prompt);
		await page.getByRole('button', { name: 'Create agent' }).click();
		await expect(page.getByRole('heading', { name: 'Your agent is saved' })).toBeVisible();
		const text = readFileSync(join(mani.home, 'workbench.yaml'), 'utf8');
		const spec = parse(text);
		expect(spec.identity.prompt).toBe(prompt);
		expect(spec.capabilities.workspace).toBe(outside);
		expect(spec.policy.tools.write).toBe('deny');
		expect(spec.output.schema.properties.answer.type).toBe('string');
		expect(spec.limits.max_tokens).toBe(12345);
		expect(text).toContain('identity:                    # who thinks?');
	} finally { rmSync(outside, { recursive: true, force: true }); }
});

test('a stale trial is replaced explicitly, not while tools or the model are running', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/agents/new');
	await page.getByRole('button', { name: /Documents assistant/ }).click();
	await page.getByRole('button', { name: 'Start trial' }).click();
	await page.getByLabel('Task for the trial').fill('slow reply');
	await page.keyboard.press('Enter');
	await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeVisible();
	await page.locator('#f-prompt').fill('New instructions with a different task.');
	await expect(page.getByRole('button', { name: 'Apply changes' })).toBeDisabled();
	await expect(page.getByRole('button', { name: 'Create agent' })).toBeDisabled();
	await page.getByRole('button', { name: 'Stop', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Apply changes' })).toBeEnabled();
	await page.getByRole('button', { name: 'Apply changes' }).click();
	await expect(page.getByText('slow reply', { exact: true })).toHaveCount(0);
	await page.getByLabel('Task for the trial').fill('hello');
	await page.keyboard.press('Enter');
	await expect(page.getByText('Hello there, friend')).toBeVisible();
});

test('native picker failures keep manual paths usable and the workbench fits a phone', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/agents/new');
	await page.getByRole('button', { name: /Documents assistant/ }).click();
	await page.route('**/api/folders/pick', (route) => route.fulfill({ status: 503, json: { error: 'Native folder picker unavailable' } }));
	await page.getByRole('button', { name: 'Browse', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('Enter a path manually');
	await page.locator('#f-ws').fill('.');
	await page.setViewportSize({ width: 390, height: 844 });
	await expect(page.getByRole('separator')).toBeHidden();
	await expect(page.getByRole('button', { name: 'Configure', exact: true })).toBeVisible();
	expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
	await page.locator('nav.side').evaluate(async (el) => { await Promise.all(el.getAnimations().map((animation) => animation.finished)); });
	await page.screenshot({ path: 'test-results/workbench-mobile.png' });
	await page.getByRole('button', { name: 'Try agent', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Start trial' })).toBeVisible();
	await page.getByRole('button', { name: 'Configure', exact: true }).click();
	await page.getByRole('tab', { name: 'Policy', exact: true }).click();
	await expect(page.getByRole('group', { name: 'Access for write' })).toBeVisible();
	expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});

test('trial tools preserve inline approvals with the segmented policy', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/agents/new');
	await page.getByRole('button', { name: /Documents assistant/ }).click();
	await page.getByRole('tab', { name: 'Tools', exact: true }).click();
	await page.locator('input[data-tool="bash"]').check();
	await page.getByRole('tab', { name: 'Policy', exact: true }).click();
	await page.getByRole('group', { name: 'Access for bash', exact: true }).getByRole('button', { name: 'Ask', exact: true }).click();
	await page.getByRole('button', { name: 'Start trial' }).click();
	await page.getByLabel('Task for the trial').fill('run it');
	await page.keyboard.press('Enter');
	await expect(page.getByRole('button', { name: 'Allow once', exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Create agent' })).toBeDisabled();
	await page.getByRole('button', { name: 'Allow once', exact: true }).click();
	await expect(page.getByText('The tool said: hello-from-bash')).toBeVisible();
	await expect(page.getByText('Your agent works.', { exact: true })).toBeVisible();
	expect(existsSync(join(mani.home, 'docs-assistant.yaml'))).toBe(false);
	expect(existsSync(join(mani.home, 'runs'))).toBe(false);
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

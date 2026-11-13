import { expect, signInWithOllama, test } from './fixtures.ts';
import { readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';

test('model limits are detected, manually saved per model, and survive reload', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/providers');
	await page.getByRole('button', { name: 'Model limits' }).click();
	await page.getByLabel('Model', { exact: true }).fill('llama3.2:3b');
	await page.getByRole('button', { name: 'Detect from endpoint' }).click();
	await expect(page.getByText('Endpoint limits detected.', { exact: false })).toBeVisible();
	await page.getByLabel('Context window', { exact: true }).fill('16384');
	await page.getByLabel('Maximum input tokens', { exact: true }).fill('10000');
	await page.getByLabel('Maximum output tokens', { exact: true }).fill('4096');
	await page.getByRole('button', { name: 'Save limits' }).click();
	await expect(page.getByRole('dialog')).toBeHidden();
	await page.reload();
	await page.getByRole('button', { name: 'Model limits' }).click();
	await page.getByLabel('Model', { exact: true }).fill('llama3.2:3b');
	await page.getByLabel('Model', { exact: true }).press('Tab');
	await expect(page.getByLabel('Context window', { exact: true })).toHaveValue('16384');
	await expect(page.getByLabel('Maximum input tokens', { exact: true })).toHaveValue('10000');
});

test('unknown limits block first send, configure, then send and restore the meter', async ({ page, mani }) => {
	await page.goto(mani.link);
	await page.evaluate(async (baseUrl) => {
		const r = await fetch('/api/providers/unknown', {
			method: 'PUT', headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ type: 'ollama', label: 'Unknown endpoint', baseUrl, models: ['unknown-model'] })
		});
		if (!r.ok) throw new Error(await r.text());
	}, mani.ollama);
	await page.goto(mani.url + '/chat');
	await expect(page.getByText('Context unknown', { exact: true })).toBeVisible();
	await page.getByLabel('Write a message…').fill('keep this draft');
	await page.keyboard.press('Enter');
	await expect(page.getByRole('dialog')).toBeVisible();
	expect(new URL(page.url()).searchParams.get('c')).toBeNull();
	await page.getByRole('button', { name: 'Detect from endpoint' }).click();
	await expect(page.getByText('This endpoint does not report limits', { exact: false })).toBeVisible();
	await page.getByLabel('Context window', { exact: true }).fill('1024');
	await page.getByLabel('Requested output tokens', { exact: true }).fill('128');
	await page.getByRole('button', { name: 'Save limits' }).click();
	await expect(page.getByRole('dialog')).toBeHidden();
	await expect(page.getByLabel('Write a message…')).toHaveValue('keep this draft');
	await expect(page.getByTestId('context-meter')).toContainText('Limits known');
	await page.getByLabel('Write a message…').press('Enter');
	await expect(page.getByText('Hello there, friend')).toBeVisible();
	await expect(page.getByTestId('context-meter').locator('summary')).toContainText(/~[\d,]+ \/ 896/);
	const meter = await page.getByTestId('context-meter').locator('summary').textContent();
	await page.reload();
	await expect(page.getByTestId('context-meter').locator('summary')).toHaveText(meter!);
	await expect(page.getByRole('log').getByText('keep this draft')).toBeVisible();
});

test('context meter persists independently of turn usage and excludes unsent drafts', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	await expect(page.getByTestId('context-meter')).toContainText('Limits known');
	await page.getByLabel('Write a message…').fill('hello');
	await page.keyboard.press('Enter');
	await expect(page.getByText('Hello there, friend')).toBeVisible();
	const meter = page.getByTestId('context-meter');
	await expect(meter.locator('summary')).toContainText(/~[\d,]+ \/ 4,096/);
	const before = await meter.locator('summary').textContent();
	await page.getByLabel('Write a message…').fill('an unsent draft '.repeat(300));
	await expect(meter.locator('summary')).toHaveText(before!);
	await meter.locator('summary').click();
	await expect(meter).toContainText('Reserved output4,096');
	await expect(meter).toContainText('Last reported input12');
	await expect(meter).toContainText('Endpoint');
	await page.screenshot({ path: 'test-results/context-meter.png' });
	await page.reload();
	await expect(meter.locator('summary')).toHaveText(before!);
	await expect(page.getByText('Earlier messages summarized. History preserved.')).toHaveCount(0);
});

test('configure reserves smaller requested output for small-window models', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	await expect(page.getByTestId('context-meter')).toContainText('Limits known');
	await page.getByTestId('context-meter').locator('summary').click();
	await page.getByTestId('context-meter').getByRole('button', { name: 'Configure limits' }).click();
	await page.getByLabel('Context window', { exact: true }).fill('1024');
	await page.getByLabel('Requested output tokens', { exact: true }).fill('128');
	await page.getByRole('button', { name: 'Save limits' }).click();
	await expect(page.getByRole('dialog')).toBeHidden();
	await page.getByLabel('Write a message…').fill('hello');
	await page.keyboard.press('Enter');
	await expect(page.getByText('Hello there, friend')).toBeVisible();
	await expect(page.getByTestId('context-meter').locator('summary')).toContainText(/~[\d,]+ \/ 896/);
	await page.goto(mani.url + '/settings');
	await expect(page.getByLabel('Requested output tokens', { exact: true })).toHaveValue('128');
	await page.getByLabel('Requested output tokens', { exact: true }).fill('256');
	await page.getByRole('button', { name: 'Save limits' }).click();
	await page.reload();
	await expect(page.getByLabel('Requested output tokens', { exact: true })).toHaveValue('256');
});

test('isolated context events show compaction once and restore context without replaying its note', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	const context = {
		estimated_input: 900, input_budget: 6000, output_limit: 2000, context_window: 8000,
		max_input_tokens: 7000, max_output_tokens: 3000, limit_source: 'mixed',
		last_input: 850, state: 'ready', compacted: true
	};
	let saved = false;
	await page.route(/\/api\/conversations\/[0-9a-f]+$/, async (route) => {
		const res = await route.fetch();
		await route.fulfill({ response: res, json: { ...await res.json(), ...(saved ? { context } : {}) } });
	});
	await page.routeWebSocket(/\/api\/conversations\/[^/]+\/ws$/, (ws) => {
		ws.onMessage((data) => {
			if (JSON.parse(String(data)).type !== 'input') return;
			ws.send(JSON.stringify({ type: 'context', payload: { ...context, state: 'compacting', compacted: false } }));
			setTimeout(() => {
				saved = true;
				for (let i = 0; i < 2; i++) ws.send(JSON.stringify({ type: 'context', payload: context }));
				ws.send(JSON.stringify({ type: 'usage', payload: { input: 99999, output: 400 } }));
				ws.send(JSON.stringify({ type: 'token', payload: { text: 'Summary complete' } }));
				ws.send(JSON.stringify({ type: 'done' }));
			}, 250);
		});
	});
	await page.goto(mani.url + '/chat');
	await expect(page.getByTestId('context-meter')).toContainText('Limits known');
	await page.getByLabel('Write a message…').fill('summarize');
	await page.keyboard.press('Enter');
	await expect(page.getByTestId('context-meter')).toContainText('Summarizing earlier messages…');
	await expect(page.getByText('Earlier messages summarized. History preserved.')).toHaveCount(1);
	await expect(page.getByTestId('context-meter').locator('summary')).toContainText('~900 / 6,000');
	await page.reload();
	await expect(page.getByTestId('context-meter').locator('summary')).toContainText('~900 / 6,000');
	await expect(page.getByText('Earlier messages summarized. History preserved.')).toHaveCount(0);
	await page.setViewportSize({ width: 375, height: 812 });
	await page.getByTestId('context-meter').locator('summary').click();
	expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
	await page.screenshot({ path: 'test-results/context-meter-phone.png', animations: 'disabled' });
});

test('real runtime compacts persisted history and restores the checkpoint without replaying its note', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	const id = await page.evaluate(async () => {
		for (const [url, method, input] of [
			['/api/providers/ollama', 'PUT', { modelLimits: { 'llama3.2:3b': { context_window: 1024 } } }],
			['/api/settings', 'PUT', { maxOutputTokens: 128 }]
		] as const) {
			const r = await fetch(url, { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(input) });
			if (!r.ok) throw new Error(await r.text());
		}
		const r = await fetch('/api/conversations', {
			method: 'POST', headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ provider: 'ollama', model: 'llama3.2:3b' })
		});
		if (!r.ok) throw new Error(await r.text());
		return (await r.json()).id as string;
	});
	const path = join(mani.home, '.mani/sessions', id + '.json');
	const seeded = JSON.parse(readFileSync(path, 'utf8'));
	const history = Array.from({ length: 8 }, (_, i) => [
		{ role: 'user', content: [{ type: 'text', text: `Earlier question ${i + 1}` }] },
		{ role: 'assistant', content: [{ type: 'text', text: i < 6 ? `Earlier answer ${i + 1}: ` + 'x'.repeat(600) : `Tiny answer ${i + 1}.` }] }
	]).flat();
	writeFileSync(path, JSON.stringify({ ...seeded, messages: history }));
	await page.goto(mani.url + '/chat?c=' + id);
	await expect(page.getByRole('log').getByText('Tiny answer 8.', { exact: true })).toBeVisible();
	await page.getByLabel('Write a message…').fill('A short next question');
	await page.keyboard.press('Enter');
	await expect(page.getByText('Hello there, friend', { exact: true })).toBeVisible();
	await expect(page.getByText('Earlier messages summarized. History preserved.', { exact: true })).toHaveCount(1);
	await expect(page.getByRole('button', { name: 'Stop', exact: true })).toBeHidden();

	const saved = JSON.parse(readFileSync(path, 'utf8'));
	expect(saved.compaction.summary).toBe('Hello there, friend');
	expect(saved.compaction.covered_messages).toBe(12);
	expect(saved.messages).toHaveLength(18);
	expect(saved.messages.slice(0, 16)).toEqual(history);
	expect(saved.context.input_budget).toBe(896);
	expect(saved.context.estimated_input).toBeLessThanOrEqual(Math.floor(896 * 0.8));
	const requests = await (await fetch(mani.ollama + '/__requests')).json();
	expect(requests.length).toBeGreaterThanOrEqual(2);
	expect(requests.slice(0, -1).every((r: { stream: boolean }) => r.stream === false)).toBe(true);
	expect(requests.at(-1).stream).toBe(true);
	const meter = page.getByTestId('context-meter');
	await expect(meter.locator('summary')).toContainText(`~${saved.context.estimated_input.toLocaleString('en')} / 896`);
	await page.screenshot({ path: 'test-results/context-real-compaction.png', animations: 'disabled' });
	await page.reload();
	await expect(page.getByText('Earlier messages summarized. History preserved.', { exact: true })).toHaveCount(0);
	await expect(meter.locator('summary')).toContainText(`~${saved.context.estimated_input.toLocaleString('en')} / 896`);
	await expect(page.getByRole('log').getByText('Tiny answer 8.', { exact: true })).toBeVisible();
	const reloaded = JSON.parse(readFileSync(path, 'utf8'));
	expect(reloaded.compaction).toEqual(saved.compaction);
	expect(reloaded.messages).toEqual(saved.messages);
});

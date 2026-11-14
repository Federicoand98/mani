import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, signInWithOllama, test } from './fixtures.ts';

const HELPER = `identity:
  name: helper
  provider: ollama
  model: qwen3.5:9b
  prompt: "help"
capabilities:
  tools: [read, bash]
context:
  inject: false
policy:
  tools:
    read: allow
    bash: ask
limits:
  max_tool_calls: 6
observability:
  journal:
    enabled: true
    path: runs/helper
`;

const QUIET = `identity:                    # who thinks?
  name: quiet
  provider: ollama
  model: qwen3.5:9b
  prompt: "hi"
`;

function seed(dir: string) {
	mkdirSync(join(dir, 'agents'), { recursive: true });
	writeFileSync(join(dir, 'agents/helper.yaml'), HELPER);
	writeFileSync(join(dir, 'notes.txt'), 'buy more coffee\n');
}

test('a conversation with an agent is one row of Runs, and each message opens as a run', async ({ page, mani }) => {
	seed(mani.home);
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat?agent=agents%2Fhelper.yaml');
	const send = async (text: string) => {
		await page.getByLabel('Write a message…').fill(text);
		await page.keyboard.press('Enter');
	};
	await send('run it');
	await page.getByRole('alert').filter({ hasText: 'asks to use' }).getByRole('button', { name: 'Allow once' }).click();
	await expect(page.getByText('The tool said: hello-from-bash')).toBeVisible();
	await send('please read notes');
	await expect(page.getByText('The tool said: buy more coffee')).toBeVisible();

	await page.getByRole('link', { name: 'Runs' }).click();
	const group = page.locator('tr.group');
	await expect(group).toHaveCount(1);
	await expect(group).toContainText('2 messages');
	await expect(group).toContainText('helper');
	await group.getByRole('button').click();
	await expect(page.getByText('Message 1')).toBeVisible();
	await page.screenshot({ path: 'test-results/runs-list.png', animations: 'disabled' });
	await page.locator('tr.child', { hasText: 'run it' }).click();

	await expect(page.getByRole('heading', { name: /helper/ })).toBeVisible();
	await expect(page.getByRole('paragraph').filter({ hasText: /^run it$/ })).toBeVisible(); // the task
	await expect(page.getByText('The tool said: hello-from-bash')).toBeVisible(); // the answer
	const step = page.locator('li.ev', { hasText: 'echo hello-from-bash' });
	await expect(step).toContainText('Allowed once in the chat');
	await step.getByText('Output').click();
	await expect(step.locator('pre')).toContainText('hello-from-bash');
	await expect(page.getByText('Message 1 of 2 in this conversation')).toBeVisible();
	await expect(page.getByText('1 / 6')).toBeVisible(); // tool calls against the limit it ran with
	await page.screenshot({ path: 'test-results/run-detail.png', fullPage: true, animations: 'disabled' });

	await page.getByRole('link', { name: 'Open conversation' }).click();
	await expect(page).toHaveURL(/\/chat\?c=/);
	await expect(page.getByText('The tool said: buy more coffee')).toBeVisible();

	await page.goto(mani.url + '/');
	await expect(page.getByRole('heading', { name: 'Recent runs' })).toBeVisible();
});

test('"Turn on" records the runs of an agent, changing only its journal block', async ({ page, mani }) => {
	mkdirSync(join(mani.home, 'agents'), { recursive: true });
	writeFileSync(join(mani.home, 'agents/quiet.yaml'), QUIET);
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/runs');
	await expect(page.getByRole('heading', { name: 'No agent records its runs yet' })).toBeVisible();
	await page.getByRole('button', { name: 'Turn on' }).click();
	await expect(page.getByText('path: runs/quiet')).toBeVisible();
	await page.getByRole('button', { name: 'Turn on' }).click();
	await expect(page.getByText('quiet now records its runs.')).toBeVisible();

	const text = readFileSync(join(mani.home, 'agents/quiet.yaml'), 'utf8');
	expect(text.startsWith(QUIET)).toBe(true);
	expect(text).toMatch(/observability:\n\s+journal:\n\s+enabled: true\n\s+path: runs\/quiet/);
	await expect(page.getByRole('heading', { name: 'No runs yet' })).toBeVisible();
});

test('the filters live in the URL', async ({ page, mani }) => {
	seed(mani.home);
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/runs?status=error');
	await expect(page.getByRole('button', { name: 'Error' })).toHaveAttribute('aria-pressed', 'true');
	await expect(page.getByRole('heading', { name: 'No runs match these filters' })).toBeVisible();
	await page.getByRole('button', { name: 'Ok', exact: true }).click();
	await expect(page).toHaveURL(/status=ok/);
	await page.getByLabel('Period').selectOption('30d');
	await expect(page).toHaveURL(/status=ok&period=30d/);
	await page.getByRole('button', { name: 'Clear filters' }).click();
	await expect(page).toHaveURL(/\/runs$/);
});

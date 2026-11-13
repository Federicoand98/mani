import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { expect, signInWithOllama, test } from './fixtures.ts';

const HELPER = `identity:
  name: helper
  description: "Reads notes and runs small commands"
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
`;

function seed(dir: string) {
	mkdirSync(join(dir, 'agents'), { recursive: true });
	writeFileSync(join(dir, 'agents/helper.yaml'), HELPER);
	writeFileSync(join(dir, 'notes.txt'), 'buy more coffee\n');
}

test('an agent reads a file of the folder, and the call stays after a reload', async ({ page, mani }) => {
	seed(mani.home);
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/agents/agents/helper.yaml');
	await page.getByRole('link', { name: 'Try in chat' }).click();
	await expect(page.getByRole('heading', { name: 'Talk with helper' })).toBeVisible();

	await page.getByLabel('Write a message…').fill('please read notes');
	await page.keyboard.press('Enter');
	await expect(page.getByText('The tool said: buy more coffee')).toBeVisible();
	const call = page.locator('details.tool', { hasText: 'notes.txt' });
	await expect(call).toBeVisible();
	await expect(call.getByText('Done')).toBeVisible();
	const meter = page.getByTestId('context-meter').locator('summary');
	await expect(meter).toContainText(/~[\d,]+ \/ 4,096/);
	const before = await meter.textContent();
	const id = new URL(page.url()).searchParams.get('c')!;
	const stored = JSON.parse(readFileSync(join(mani.home, '.mani/sessions', id + '.json'), 'utf8'));
	expect(stored.context.estimated_input).toBeGreaterThan(0);
	expect(stored.compaction).toBeUndefined();
	expect(readFileSync(join(mani.home, 'agents/helper.yaml'), 'utf8')).toBe(HELPER);

	await page.reload();
	await expect(page.locator('details.tool', { hasText: 'notes.txt' })).toBeVisible();
	await expect(page.getByText('The tool said: buy more coffee')).toBeVisible();
	await expect(meter).toHaveText(before!);
	await meter.click();
	await expect(page.getByTestId('context-meter')).toContainText('Endpoint');
	await page.screenshot({ path: 'test-results/context-agent-meter.png', animations: 'disabled' });
});

test('a tool on "Ask" waits in the conversation: once, for the conversation, deny', async ({ page, mani }) => {
	seed(mani.home);
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat?agent=agents%2Fhelper.yaml');

	const send = async (text: string) => {
		await page.getByLabel('Write a message…').fill(text);
		await page.keyboard.press('Enter');
	};

	await send('run it');
	const ask = page.getByRole('alert').filter({ hasText: 'asks to use' });
	await expect(ask).toContainText('echo hello-from-bash');
	await ask.getByRole('button', { name: 'Allow once' }).click();
	await expect(page.getByText('The tool said: hello-from-bash')).toBeVisible();

	await send('run it again');
	await page.getByRole('alert').filter({ hasText: 'asks to use' }).getByRole('button', { name: 'Allow for this conversation' }).click();
	await expect(page.getByText('The tool said: hello-from-bash').nth(1)).toBeVisible();

	await send('run it a third time');
	await expect(page.getByText('The tool said: hello-from-bash').nth(2)).toBeVisible();
	await expect(page.getByRole('alert').filter({ hasText: 'asks to use' })).toHaveCount(0);

	// A new conversation with the same agent asks again, and a denial stops the command.
	await page.getByRole('link', { name: 'New conversation' }).click();
	await expect(page.getByRole('button', { name: 'Agent', exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Agent', exact: true }).click();
	await page.getByRole('menuitem', { name: /helper/ }).click();
	await send('run it');
	await page.getByRole('alert').filter({ hasText: 'asks to use' }).getByRole('button', { name: 'Deny' }).click();
	await expect(page.getByText(/The tool said: .*blocked/)).toBeVisible();

	expect(readFileSync(join(mani.home, 'agents/helper.yaml'), 'utf8')).toBe(HELPER);
});

test('switch from the model to an agent and back, with a line that says so', async ({ page, mani }) => {
	seed(mani.home);
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	await page.getByLabel('Write a message…').fill('hello');
	await page.keyboard.press('Enter');
	await expect(page.getByText('Hello there, friend')).toBeVisible();

	await page.getByRole('button', { name: 'Agent', exact: true }).click();
	await page.getByRole('menuitem', { name: /helper/ }).click();
	await expect(page.getByText(/From here, the agent helper answers/)).toBeVisible();
	await expect(page.getByLabel('Model')).toBeHidden();

	await page.getByRole('button', { name: 'Remove the agent' }).click();
	await expect(page.getByText(/answers alone, without tools/)).toBeVisible();
	await expect(page.getByLabel('Model')).toBeVisible();
});

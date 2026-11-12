import { existsSync, readdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, signInWithOllama, test } from './fixtures.ts';

// The smallest valid PNG: 1×1, one pixel.
const PNG = Buffer.from(
	'89504e470d0a1a0a0000000d4948445200000001000000010806000000' + '1f15c4890000000d4944415478da63f80f00000101000518d84e0000000049454e44ae426082',
	'hex'
);

test('without a provider, the chat sends you to Providers', async ({ page, mani }) => {
	await page.goto(mani.link);
	await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Chat' }).click();
	await expect(page.getByRole('heading', { name: 'Connect a model to start' })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Go to Providers' })).toBeVisible();
});

test('a conversation: reply in streaming, saved in the folder, listed with its title', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	// Ollama lists its models in alphabetical order: the first is the default.
	await expect(page.getByRole('heading', { name: 'Talk with llama3.2:3b' })).toBeVisible();

	await page.getByLabel('Write a message…').fill('hello');
	await page.keyboard.press('Enter');
	await expect(page.getByText('Hello there, friend')).toBeVisible();
	await expect(page.getByText(/12 in · 4 out/)).toBeVisible();
	await expect(page).toHaveURL(/\/chat\?c=[0-9a-f]+/);

	const item = page.getByRole('complementary', { name: 'Conversations' }).getByRole('link', { name: /hello/ });
	await expect(item).toBeVisible();
	expect(readdirSync(join(mani.home, '.mani/sessions')).length).toBe(1);
	expect(existsSync(join(mani.home, '.mani/.gitignore'))).toBe(true);

	await page.reload();
	await expect(page.getByText('Hello there, friend')).toBeVisible();
});

test('Stop ends the reply and keeps the question', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	await page.getByLabel('Write a message…').fill('tell me something slow');
	await page.keyboard.press('Enter');
	await expect(page.getByText(/One two/)).toBeVisible();
	await page.getByRole('button', { name: 'Stop' }).click();
	await expect(page.getByText('Stopped. The reply so far is saved.')).toBeVisible();

	await page.reload();
	await expect(page.getByRole('log').getByText('tell me something slow')).toBeVisible();
});

test('reloading mid-reply does not lose it', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	await page.getByLabel('Write a message…').fill('slow please');
	await page.keyboard.press('Enter');
	await expect(page.getByText(/One two/)).toBeVisible();

	await page.reload();
	await expect(page.getByText('Still answering.')).toBeVisible();
	await expect(page.getByText('One two three four five six seven eight nine ten')).toBeVisible({ timeout: 10_000 });
	await expect(page.getByText('Still answering.')).toBeHidden();
});

test('rename and delete a conversation', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	await page.getByLabel('Write a message…').fill('hello');
	await page.keyboard.press('Enter');
	await expect(page.getByText('Hello there, friend')).toBeVisible();

	const list = page.getByRole('complementary', { name: 'Conversations' });
	await list.getByRole('button', { name: 'Rename hello' }).click();
	await list.getByLabel('Conversation title').fill('Greetings');
	await page.keyboard.press('Enter');
	await expect(list.getByRole('link', { name: /Greetings/ })).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Greetings', level: 1 })).toBeVisible();

	await list.getByRole('button', { name: 'Delete Greetings' }).click();
	await list.getByRole('button', { name: 'Delete', exact: true }).click();
	await expect(page.getByText('Conversation deleted')).toBeVisible();
	await expect(list.getByRole('link', { name: /Greetings/ })).toHaveCount(0);
	await expect(page).toHaveURL(mani.url + '/chat');
});

test('/model switches the model, /help lists the commands', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	await page.getByLabel('Write a message…').fill('hello');
	await page.keyboard.press('Enter');
	await expect(page.getByText('Hello there, friend')).toBeVisible();

	await page.getByLabel('Write a message…').fill('/model qwen3.5:9b');
	await page.keyboard.press('Enter');
	await expect(page.getByText('From here, qwen3.5:9b answers.')).toBeVisible();
	await expect(page.getByLabel('Model')).toHaveValue('ollama|qwen3.5:9b');

	await page.getByLabel('Write a message…').fill('/help');
	await page.keyboard.press('Enter');
	await expect(page.getByText(/\/clear\s+start a new conversation/)).toBeVisible();
});

test('an attached image reaches the model', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	await page.locator('input[type=file]').setInputFiles({ name: 'dot.png', mimeType: 'image/png', buffer: PNG });
	await expect(page.getByRole('img', { name: 'dot.png' })).toBeVisible();
	await page.getByLabel('Write a message…').fill('what is this?');
	await page.keyboard.press('Enter');
	await expect(page.getByText('Hello there, friend')).toBeVisible();

	const sent = await (await fetch(mani.ollama + '/__last')).text();
	expect(sent).toContain(PNG.toString('base64'));
});

test('code in a reply gets a Copy button', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	await page.getByLabel('Write a message…').fill('show me code');
	await page.keyboard.press('Enter');
	await expect(page.locator('pre code', { hasText: 'console.log(1)' })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Copy' })).toBeVisible();
});

// A note from a command belongs where it was written, not at the bottom forever.
test('a command note stays above the messages sent after it', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	await page.getByLabel('Write a message…').fill('/help');
	await page.keyboard.press('Enter');
	await page.getByLabel('Write a message…').fill('hello');
	await page.keyboard.press('Enter');
	await expect(page.getByText('Hello there, friend')).toBeVisible();

	const order = await page.getByRole('log').evaluate((log) => {
		const help = [...log.querySelectorAll('pre')].find((p) => p.textContent?.includes('/clear'))!;
		const reply = [...log.querySelectorAll('*')].find((e) => e.textContent === 'Hello there, friend')!;
		return help.compareDocumentPosition(reply) & Node.DOCUMENT_POSITION_FOLLOWING ? 'help first' : 'reply first';
	});
	expect(order).toBe('help first');
});

test('an image dropped anywhere on the conversation is attached', async ({ page, mani }) => {
	await signInWithOllama(page, mani);
	await page.goto(mani.url + '/chat');
	const data = await page.evaluateHandle((hex) => {
		const bytes = new Uint8Array(hex.match(/../g)!.map((h) => parseInt(h, 16)));
		const dt = new DataTransfer();
		dt.items.add(new File([bytes], 'dropped.png', { type: 'image/png' }));
		return dt;
	}, PNG.toString('hex'));
	const log = page.getByRole('log');
	await log.dispatchEvent('dragenter', { dataTransfer: data });
	await expect(page.getByText('Drop images to attach them to your message')).toBeVisible();
	await log.dispatchEvent('drop', { dataTransfer: data });
	await expect(page.getByRole('img', { name: 'dropped.png' })).toBeVisible();
	await expect(page.getByText('Drop images to attach them to your message')).toBeHidden();
});

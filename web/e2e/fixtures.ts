import { test as base } from '@playwright/test';
import { spawn } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { createServer, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';

const BIN = process.env.MANI_BIN ?? resolve(import.meta.dirname, '../../build/mani');

export type Mani = {
	url: string; // base URL, without the token
	link: string; // the one-time sign-in link mani printed
	ollama: string; // a fake Ollama with two models
	home: string; // where config.json and auth.json live for this test
};

// A fake Ollama: two models, and /api/chat streaming one word per chunk. A
// message containing "slow" streams a word every 400 ms; one containing "code"
// gets a fenced code block back. /__last returns the last chat request.
function fakeOllama(): Promise<Server> {
	let last = '';
	const s = createServer((req, res) => {
		if (req.url === '/api/tags') {
			res.setHeader('Content-Type', 'application/json');
			res.end(JSON.stringify({ models: [{ name: 'qwen3.5:9b' }, { name: 'llama3.2:3b' }] }));
			return;
		}
		if (req.url === '/__last') {
			res.end(last);
			return;
		}
		if (req.url === '/api/chat') {
			let body = '';
			req.on('data', (d) => (body += d));
			req.on('end', async () => {
				last = body;
				const msgs = JSON.parse(body).messages as { role: string; content: string }[];
				const said = msgs.filter((m) => m.role === 'user').at(-1)?.content ?? '';
				// A model that uses tools: "read notes" reads notes.txt, "run" runs a
				// command; after a tool result it reports it.
				const lastMsg = msgs.at(-1)!;
				const toolCall =
					lastMsg.role === 'user' && said.includes('read notes')
						? { name: 'read', arguments: { path: 'notes.txt' } }
						: lastMsg.role === 'user' && said.includes('run it')
							? { name: 'bash', arguments: { command: 'echo hello-from-bash' } }
							: null;
				if (toolCall || lastMsg.role === 'tool') {
					res.setHeader('Content-Type', 'application/x-ndjson');
					const message = toolCall
						? { role: 'assistant', content: '', tool_calls: [{ function: toolCall }] }
						: { role: 'assistant', content: 'The tool said: ' + lastMsg.content.trim() };
					res.write(JSON.stringify({ message, done: false }) + '\n');
					res.end(JSON.stringify({ message: { role: 'assistant', content: '' }, done: true, done_reason: 'stop' }) + '\n');
					return;
				}
				const reply = said.includes('code')
					? 'Here it is:\n\n```js\nconsole.log(1)\n```'
					: said.includes('slow')
						? 'One two three four five six seven eight nine ten'
						: 'Hello there, friend';
				const words = reply.split(/(?<= )/);
				res.setHeader('Content-Type', 'application/x-ndjson');
				for (const w of words) {
					if (res.destroyed) return;
					res.write(JSON.stringify({ message: { role: 'assistant', content: w }, done: false }) + '\n');
					await new Promise((r) => setTimeout(r, said.includes('slow') ? 400 : 10));
				}
				res.end(JSON.stringify({ message: { role: 'assistant', content: '' }, done: true, done_reason: 'stop', prompt_eval_count: 12, eval_count: 4 }) + '\n');
			});
			return;
		}
		res.statusCode = 404;
		res.end('not found');
	});
	return new Promise((ok) => s.listen(0, '127.0.0.1', () => ok(s)));
}

export const test = base.extend<{ mani: Mani }>({
	mani: async ({}, use) => {
		const home = mkdtempSync(join(tmpdir(), 'mani-e2e-'));
		const ollama = await fakeOllama();
		const port = 17000 + Math.floor(Math.random() * 2000);
		const proc = spawn(BIN, ['web', '--no-open', '--addr', `127.0.0.1:${port}`, '--dir', home], {
			env: { ...process.env, HOME: home, XDG_CONFIG_HOME: join(home, 'config'), XDG_DATA_HOME: join(home, 'data') }
		});
		const link = await new Promise<string>((ok, fail) => {
			let out = '';
			proc.stderr.on('data', (d) => {
				out += d;
				const m = out.match(/http:\/\/\S+\?t=[0-9a-f]+/);
				if (m) ok(m[0]);
			});
			proc.on('exit', (code) => fail(new Error(`mani web exited with ${code}:\n${out}`)));
		});
		await use({
			url: `http://127.0.0.1:${port}`,
			link,
			ollama: `http://127.0.0.1:${(ollama.address() as AddressInfo).port}`,
			home
		});
		proc.kill();
		ollama.close();
		rmSync(home, { recursive: true, force: true });
	}
});

export { expect } from '@playwright/test';

/** Signs in and adds the fake Ollama through the API, for tests about other pages. */
export async function signInWithOllama(page: import('@playwright/test').Page, mani: Mani) {
	await page.goto(mani.link);
	const status = await page.evaluate(
		(url) =>
			fetch('/api/providers/ollama', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ baseUrl: url }) }).then(
				(r) => r.status
			),
		mani.ollama
	);
	if (status !== 200) throw new Error(`adding the fake Ollama answered ${status}`);
}

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

function fakeOllama(): Promise<Server> {
	const s = createServer((req, res) => {
		if (req.url === '/api/tags') {
			res.setHeader('Content-Type', 'application/json');
			res.end(JSON.stringify({ models: [{ name: 'qwen3.5:9b' }, { name: 'llama3.2:3b' }] }));
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

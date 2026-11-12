import { afterEach, describe, expect, it, vi } from 'vitest';
import { api, ApiError, call } from './client.ts';

function respond(status: number, body: unknown) {
	return vi.fn().mockResolvedValue(new Response(body === undefined ? null : JSON.stringify(body), { status }));
}

afterEach(() => vi.unstubAllGlobals());

describe('call', () => {
	it('sends JSON and returns the decoded answer', async () => {
		const f = respond(200, { name: 'ollama', enabled: true });
		vi.stubGlobal('fetch', f);

		const p = await api.saveProvider('ollama', { baseUrl: 'http://box:11434' });

		expect(p.enabled).toBe(true);
		const [url, init] = f.mock.calls[0];
		expect(url).toBe('/api/providers/ollama');
		expect(init.method).toBe('PUT');
		expect(init.headers).toEqual({ 'Content-Type': 'application/json' });
		expect(JSON.parse(init.body)).toEqual({ baseUrl: 'http://box:11434' });
	});

	it('carries the server message on an error status', async () => {
		vi.stubGlobal('fetch', respond(400, { error: 'OpenAI needs an API key' }));
		await expect(api.saveProvider('openai', {})).rejects.toMatchObject({ status: 400, message: 'OpenAI needs an API key' });
	});

	it('falls back to the status text when the body is not JSON', async () => {
		vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('boom', { status: 502, statusText: 'Bad Gateway' })));
		await expect(call('GET', '/x')).rejects.toMatchObject({ status: 502, message: '502 Bad Gateway' });
	});

	it('reports status 0 when mani does not answer', async () => {
		vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')));
		const err = await call('GET', '/health').catch((e) => e);
		expect(err).toBeInstanceOf(ApiError);
		expect(err.status).toBe(0);
	});

	it('sends no body and no content type on a GET', async () => {
		const f = respond(200, { providers: [] });
		vi.stubGlobal('fetch', f);
		await api.providers();
		expect(f.mock.calls[0][1].body).toBeUndefined();
		expect(f.mock.calls[0][1].headers).toEqual({});
	});

	it('escapes the provider name in the path', async () => {
		const f = respond(200, { models: [] });
		vi.stubGlobal('fetch', f);
		await api.models('a/b');
		expect(f.mock.calls[0][0]).toBe('/api/providers/a%2Fb/models');
	});
});

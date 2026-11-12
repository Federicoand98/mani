import { describe, expect, it } from 'vitest';
import { idle, reduce, start, type Turn } from './turn.ts';

const run = (t: Turn, ...msgs: [string, unknown?, number?][]) =>
	msgs.reduce((acc, [type, payload, now]) => reduce(acc, { type, payload }, now ?? 0), t);

describe('reduce', () => {
	it('builds the reply from tokens and sums the usage', () => {
		const t = run(start(1000), ['token', { text: 'Hel' }, 1200], ['token', { text: 'lo' }, 1300], ['usage', { input: 10, output: 2 }], ['usage', { input: 5, output: 1 }], ['done']);
		expect(t.text).toBe('Hello');
		expect(t.usage).toEqual({ input: 15, output: 3 });
		expect(t.status).toBe('idle');
		expect(t.ended).toBe('done');
		expect(t.firstTokenAt).toBe(1200);
	});

	it('times the reasoning until the first token', () => {
		const t = run(start(1000), ['thinking', { text: 'hmm' }, 1100], ['thinking', { text: '…' }, 1500], ['token', { text: 'ok' }, 4000]);
		expect(t.thinking).toBe('hmm…');
		expect(t.thinkingMs).toBe(3000);
	});

	it('keeps the error message', () => {
		const t = run(start(0), ['error', { message: 'ollama: HTTP 404: model not found' }]);
		expect(t.error).toContain('model not found');
		expect(t.ended).toBe('error');
	});

	it('marks a reply already running in the background', () => {
		expect(run(idle(), ['busy']).status).toBe('busy');
	});

	it('ignores messages it does not know', () => {
		const t = start(0);
		expect(reduce(t, { type: 'tool_call' }, 0)).toBe(t);
	});
});

import { describe, expect, it } from 'vitest';
import { idle, openAsks, reduce, start, textOf, type Turn } from './turn.ts';

const run = (t: Turn, ...msgs: [string, unknown?, number?][]) =>
	msgs.reduce((acc, [type, payload, now]) => reduce(acc, { type, payload }, now ?? 0), t);

describe('reduce', () => {
	it('builds the reply from tokens and sums the usage', () => {
		const t = run(start(1000), ['token', { text: 'Hel' }, 1200], ['token', { text: 'lo' }, 1300], ['usage', { input: 10, output: 2 }], ['usage', { input: 5, output: 1 }], ['done']);
		expect(textOf(t)).toBe('Hello');
		expect(t.usage).toEqual({ input: 15, output: 3 });
		expect(t.status).toBe('idle');
		expect(t.ended).toBe('done');
		expect(t.firstTokenAt).toBe(1200);
	});

	it('keeps text, tool calls and results in the order they happened', () => {
		const t = run(
			start(0),
			['token', { text: 'Let me look. ' }],
			['tool_call', { name: 'read', input: { path: 'a.txt' } }],
			['tool_result', { name: 'read', result: 'hello', is_error: false }],
			['token', { text: 'It says hello.' }]
		);
		expect(t.segments.map((s) => s.kind)).toEqual(['text', 'tool', 'text']);
		expect(t.segments[1]).toMatchObject({ name: 'read', input: { path: 'a.txt' }, result: 'hello', isError: false });
		expect(textOf(t)).toBe('Let me look. It says hello.');
	});

	it('pairs a result with the latest open call of the same tool', () => {
		const t = run(start(0), ['tool_call', { name: 'bash' }], ['tool_result', { name: 'bash', result: '1' }], ['tool_call', { name: 'bash' }], ['tool_result', { name: 'bash', result: '2' }]);
		expect(t.segments.map((s) => (s.kind === 'tool' ? s.result : ''))).toEqual(['1', '2']);
	});

	it('tracks an approval from request to answer, once even if sent twice', () => {
		const req = { request_id: 'r1', tool_name: 'bash', risk_level: 'execute', preview: 'echo hi', input: { command: 'echo hi' } };
		let t = run(start(0), ['permission_request', req], ['permission_request', req]);
		expect(openAsks(t)).toHaveLength(1);
		t = run(t, ['permission_resolved', { request_id: 'r1', decision: 'allow_conversation' }]);
		expect(openAsks(t)).toHaveLength(0);
		expect(t.segments[0]).toMatchObject({ kind: 'ask', state: 'allow_conversation', tool: 'bash' });
	});

	it('times the reasoning until the first token', () => {
		const t = run(start(1000), ['thinking', { text: 'hmm' }, 1100], ['thinking', { text: '…' }, 1500], ['token', { text: 'ok' }, 4000]);
		expect(t.thinking).toBe('hmm…');
		expect(t.thinkingMs).toBe(3000);
	});

	it('keeps the error message and the structured answer', () => {
		expect(run(start(0), ['error', { message: 'ollama: HTTP 404: model not found' }]).error).toContain('model not found');
		expect(run(start(0), ['structured', { sentiment: 'negative' }]).structured).toEqual({ sentiment: 'negative' });
	});

	it('marks a reply already running in the background', () => {
		expect(run(idle(), ['busy']).status).toBe('busy');
	});
});

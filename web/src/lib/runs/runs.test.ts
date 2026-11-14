import { describe, expect, it } from 'vitest';
import type { Run, RunEvent } from '#lib/api/types.ts';
import { journalOn, apiQuery, decisionText, DEFAULT_FILTERS, duration, filtersFrom, filtersQuery, groupRuns, timeline } from './runs.ts';

const run = (id: string, o: Partial<Run> = {}): Run => ({
	id,
	agent: 'docs',
	agentPath: 'agents/docs.yaml',
	model: 'ollama / qwen3.5:9b',
	task: 'task ' + id,
	source: 'cli',
	sessionId: '',
	title: '',
	status: 'ok',
	startedAt: '2026-11-14T10:00:00Z',
	endedAt: '2026-11-14T10:00:05Z',
	summary: { llmCalls: 1, toolCalls: 0, inTokens: 100, outTokens: 10, blocked: 0, masked: 0, errors: 0 },
	...o
});

describe('groupRuns', () => {
	it('folds the messages of a chat conversation into one row, at its newest message', () => {
		const rows = groupRuns([
			run('c3', { source: 'web', sessionId: 's1', startedAt: '2026-11-14T10:30:00Z' }),
			run('x', { source: 'daily' }),
			run('c2', { source: 'web', sessionId: 's1', status: 'error', startedAt: '2026-11-14T10:20:00Z', summary: { ...run('').summary, blocked: 1 } }),
			run('c1', { source: 'web', sessionId: 's1', startedAt: '2026-11-14T10:10:00Z' })
		]);
		expect(rows.map((r) => (r.kind === 'run' ? r.run.id : 'conv'))).toEqual(['conv', 'x']);
		const g = rows[0];
		if (g.kind !== 'conversation') throw new Error('not a group');
		expect(g.runs.map((r) => r.id)).toEqual(['c3', 'c2', 'c1']);
		expect(g.status).toBe('error'); // the worst of its messages
		expect(g.startedAt).toBe('2026-11-14T10:10:00Z');
		expect(g.lastAt).toBe('2026-11-14T10:30:00Z');
		expect(g.tokens).toBe(330);
		expect(g.blocked).toBe(1);
	});

	it('leaves alone a one-message conversation and the runs of a trigger sharing a session', () => {
		const rows = groupRuns([
			run('a', { source: 'web', sessionId: 's1' }),
			run('d1', { source: 'daily', sessionId: 'mem' }),
			run('d2', { source: 'daily', sessionId: 'mem' })
		]);
		expect(rows.map((r) => r.kind)).toEqual(['run', 'run', 'run']);
	});

	it('puts a running message first in the status of its conversation', () => {
		const rows = groupRuns([run('b', { source: 'web', sessionId: 's', status: 'running' }), run('a', { source: 'web', sessionId: 's', status: 'error' })]);
		expect(rows[0].kind === 'conversation' && rows[0].status).toBe('running');
	});
});

describe('filters', () => {
	it('round-trip through the URL, leaving the defaults out', () => {
		expect(filtersQuery(DEFAULT_FILTERS)).toBe('');
		const f = { status: 'error', agent: 'docs', source: 'daily', period: '30d', blocked: true };
		expect(filtersFrom(new URLSearchParams(filtersQuery(f).slice(1)))).toEqual(f);
	});

	it('always send the period to the API, and the cursor when there is one', () => {
		expect(apiQuery(DEFAULT_FILTERS)).toBe('?period=7d');
		expect(apiQuery({ ...DEFAULT_FILTERS, status: 'ok' }, '2026-11-14T10:00:00Z', 200)).toBe(
			'?period=7d&status=ok&before=2026-11-14T10%3A00%3A00Z&limit=200'
		);
	});
});

describe('decisionText', () => {
	it('says who decided, from where the run started', () => {
		expect(decisionText('allowed', 'web').text).toBe('Allowed once in the chat');
		expect(decisionText('allowed', 'daily').text).toBe('Allowed by the trigger policy');
		expect(decisionText('denied', 'tui')).toMatchObject({ text: 'Denied in the terminal', blocked: true });
		expect(decisionText('expired', 'server')).toMatchObject({ text: 'Denied: nobody answered in time', blocked: true });
		expect(decisionText('unattended', 'cli').blocked).toBe(true);
		expect(decisionText('remembered', 'web').blocked).toBe(false);
	});
});

const ev = (seq: number, kind: string, data: Record<string, unknown>, depth = 0): RunEvent => ({ seq, at: '', depth, kind, data });

describe('timeline', () => {
	it('reads each tool call as one step with its answer and its result', () => {
		const lines = timeline(
			[
				ev(0, 'run_start', { source: 'web' }),
				ev(1, 'llm_call', { messages: 2, tools: 3 }),
				ev(2, 'llm_response', { in_tokens: 1200, out_tokens: 40, stop_reason: 'tool_use' }),
				ev(3, 'tool_call', { tool: 'write', input: { path: 'out.md', content: 'x' } }),
				ev(4, 'permission', { tool: 'write', decision: 'allowed' }),
				ev(5, 'tool_result', { tool: 'write', is_error: false, result_len: 12, preview: 'wrote 1 line' }),
				ev(6, 'tool_call', { tool: 'bash', input: { command: 'rm -rf old' } }),
				ev(7, 'guardrail', { tool: 'bash', action: 'deny', label: 'no deleting' }),
				ev(8, 'tool_call', { tool: 'read', input: { path: 'bank.txt' } }, 1),
				// The mask is recorded before the result: the journal's result hook runs last.
				ev(9, 'guardrail', { tool: 'read', action: 'mask' }, 1),
				ev(10, 'tool_result', { tool: 'read', is_error: false, result_len: 5000, preview: 'IBAN [iban]' }, 1),
				ev(11, 'run_end', { status: 'ok' })
			],
			'web'
		);
		expect(lines.map((l) => [l.tool ?? l.title, l.arg ?? '', l.detail])).toEqual([
			['Run started', '', 'Chat'],
			['Model answered', '', '1,200 in / 40 out, asked for tools'],
			['write', 'out.md', 'Allowed once in the chat - 12 B'],
			['bash', 'rm -rf old', 'Rule "no deleting"'],
			['read', 'bank.txt', 'masked by a redaction rule - 4.9 KB, preview below'],
			['Run ended', '', 'ok']
		]);
		expect(lines[3]).toMatchObject({ blocked: true, rules: true, title: 'Blocked' });
		expect(lines[4]).toMatchObject({ depth: 1, output: 'IBAN [iban]', cut: true });
	});

	it('shows a denied call as stopped, and a call still running as waiting', () => {
		const lines = timeline(
			[
				ev(0, 'tool_call', { tool: 'bash', input: { command: 'df -h' } }),
				ev(1, 'permission', { tool: 'bash', decision: 'expired' }),
				ev(2, 'tool_call', { tool: 'read', input: { path: 'a' } })
			],
			'cron'
		);
		expect(lines[0]).toMatchObject({ detail: 'Denied: nobody answered in time', blocked: true, icon: 'hand' });
		expect(lines[1].detail).toBe('waiting for the result');
	});
});

describe('duration', () => {
	it('reads well at every scale', () => {
		const at = (s: number) => new Date(Date.parse('2026-11-14T10:00:00Z') + s * 1000).toISOString();
		expect(duration(at(0), at(8.2))).toBe('8.2 s');
		expect(duration(at(0), at(41))).toBe('41 s');
		expect(duration(at(0), at(125))).toBe('2m 5s');
		expect(duration(at(0), '', Date.parse(at(3)))).toBe('3.0 s');
	});
});

describe('journalOn', () => {
	it('adds the journal block and keeps the rest of the file as it was', () => {
		const text = 'identity:                    # who thinks?\n  name: docs\n  model: m\n';
		const out = journalOn(text, 'docs');
		expect(out.startsWith(text)).toBe(true);
		expect(out).toContain('observability:');
		expect(out).toMatch(/journal:\n\s+enabled: true\n\s+path: runs\/docs/);
	});

	it('turns on a journal that was off, keeping its path', () => {
		const text = 'identity:\n  name: docs\nobservability:\n  journal:\n    enabled: false # off for now\n    path: logs\n';
		expect(journalOn(text, 'docs')).toBe(text.replace('enabled: false', 'enabled: true'));
	});
});

import type { ServerMsg } from '../api/wire.ts';

// The reply being written, folded from the WebSocket messages into the order
// things happened: text, tool calls with their results, approval requests.
// Pure, so the page and the tests share it.

export type Ask = {
	kind: 'ask';
	id: string;
	tool: string;
	risk: string;
	preview: string;
	input: Record<string, unknown>;
	state: 'open' | 'allow_once' | 'allow_conversation' | 'deny';
};

export type Segment =
	| { kind: 'text'; text: string }
	| { kind: 'tool'; name: string; input: Record<string, unknown>; result?: string; isError?: boolean }
	| Ask;

export type Turn = {
	status: 'idle' | 'answering' | 'busy';
	segments: Segment[];
	thinking: string;
	thinkingMs: number;
	startedAt: number;
	firstTokenAt: number;
	usage: { input: number; output: number } | null;
	structured: unknown;
	error: string;
	ended: '' | 'done' | 'cancelled' | 'error';
};

export const idle = (): Turn => ({
	status: 'idle',
	segments: [],
	thinking: '',
	thinkingMs: 0,
	startedAt: 0,
	firstTokenAt: 0,
	usage: null,
	structured: null,
	error: '',
	ended: ''
});

export function start(now: number): Turn {
	return { ...idle(), status: 'answering', startedAt: now };
}

/** All the text of the reply so far. */
export const textOf = (t: Turn) =>
	t.segments
		.filter((s): s is Extract<Segment, { kind: 'text' }> => s.kind === 'text')
		.map((s) => s.text)
		.join('');

/** Approval requests still waiting for an answer. */
export const openAsks = (t: Turn) => t.segments.filter((s): s is Ask => s.kind === 'ask' && s.state === 'open');

export function reduce(t: Turn, m: ServerMsg, now: number): Turn {
	const p = (m.payload ?? {}) as Record<string, unknown>;
	switch (m.type) {
		case 'busy':
			return { ...idle(), status: 'busy' };
		case 'thinking':
			return { ...t, thinking: t.thinking + String(p.text ?? '') };
		case 'token': {
			const text = String(p.text ?? '');
			const segs = [...t.segments];
			const last = segs.at(-1);
			if (last?.kind === 'text') segs[segs.length - 1] = { ...last, text: last.text + text };
			else segs.push({ kind: 'text', text });
			const first = t.firstTokenAt || now;
			const thinkingMs = t.thinking && !t.firstTokenAt ? now - t.startedAt : t.thinkingMs;
			return { ...t, segments: segs, firstTokenAt: first, thinkingMs };
		}
		case 'tool_call':
			return { ...t, segments: [...t.segments, { kind: 'tool', name: String(p.name ?? ''), input: (p.input ?? {}) as Record<string, unknown> }] };
		case 'tool_result': {
			const segs = [...t.segments];
			for (let i = segs.length - 1; i >= 0; i--) {
				const s = segs[i];
				if (s.kind === 'tool' && s.name === p.name && s.result === undefined) {
					segs[i] = { ...s, result: String(p.result ?? ''), isError: Boolean(p.is_error) };
					break;
				}
			}
			return { ...t, segments: segs };
		}
		case 'permission_request': {
			if (t.segments.some((s) => s.kind === 'ask' && s.id === p.request_id)) return t; // sent again to a reopened tab
			const ask: Ask = {
				kind: 'ask',
				id: String(p.request_id ?? ''),
				tool: String(p.tool_name ?? ''),
				risk: String(p.risk_level ?? ''),
				preview: String(p.preview ?? ''),
				input: (p.input ?? {}) as Record<string, unknown>,
				state: 'open'
			};
			return { ...t, segments: [...t.segments, ask] };
		}
		case 'permission_resolved':
			return {
				...t,
				segments: t.segments.map((s) => (s.kind === 'ask' && s.id === p.request_id ? { ...s, state: p.decision as Ask['state'] } : s))
			};
		case 'structured':
			return { ...t, structured: m.payload };
		case 'usage': {
			const prev = t.usage ?? { input: 0, output: 0 };
			return { ...t, usage: { input: prev.input + Number(p.input ?? 0), output: prev.output + Number(p.output ?? 0) } };
		}
		case 'done':
		case 'cancelled':
			return { ...t, status: 'idle', ended: m.type };
		case 'error':
			return { ...t, status: 'idle', ended: 'error', error: String(p.message ?? 'Something went wrong') };
		default:
			return t;
	}
}

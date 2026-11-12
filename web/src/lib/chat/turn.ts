import type { ServerMsg } from '../api/wire.ts';

// The reply being written, folded from the WebSocket messages. Pure, so the
// page and the tests share it.

export type Turn = {
	status: 'idle' | 'answering' | 'busy';
	text: string;
	thinking: string;
	thinkingMs: number;
	startedAt: number;
	firstTokenAt: number;
	usage: { input: number; output: number } | null;
	error: string;
	ended: '' | 'done' | 'cancelled' | 'error';
};

export const idle = (): Turn => ({
	status: 'idle',
	text: '',
	thinking: '',
	thinkingMs: 0,
	startedAt: 0,
	firstTokenAt: 0,
	usage: null,
	error: '',
	ended: ''
});

export function start(now: number): Turn {
	return { ...idle(), status: 'answering', startedAt: now };
}

export function reduce(t: Turn, m: ServerMsg, now: number): Turn {
	const p = (m.payload ?? {}) as Record<string, unknown>;
	switch (m.type) {
		case 'busy':
			return { ...idle(), status: 'busy' };
		case 'thinking':
			return { ...t, thinking: t.thinking + String(p.text ?? '') };
		case 'token': {
			const first = t.firstTokenAt || now;
			const thinkingMs = t.thinking && !t.firstTokenAt ? now - t.startedAt : t.thinkingMs;
			return { ...t, text: t.text + String(p.text ?? ''), firstTokenAt: first, thinkingMs };
		}
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

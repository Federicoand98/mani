import type { Run, RunEvent } from '#lib/api/types.ts';
import { Manifest } from '#lib/agents/doc.ts';
import type { IconName } from '#lib/components/icons.ts';

// --- filters <-> URL ---------------------------------------------------------------

export type Filters = { status: string; agent: string; source: string; period: string; blocked: boolean };

export const DEFAULT_FILTERS: Filters = { status: '', agent: '', source: '', period: '7d', blocked: false };

export function filtersFrom(params: { get(name: string): string | null }): Filters {
	return {
		status: params.get('status') ?? '',
		agent: params.get('agent') ?? '',
		source: params.get('source') ?? '',
		period: params.get('period') ?? '7d',
		blocked: params.get('blocked') === '1'
	};
}

/** The query string of f, without the defaults, so a link stays short. */
export function filtersQuery(f: Filters): string {
	const q = new URLSearchParams();
	if (f.status) q.set('status', f.status);
	if (f.agent) q.set('agent', f.agent);
	if (f.source) q.set('source', f.source);
	if (f.period !== '7d') q.set('period', f.period);
	if (f.blocked) q.set('blocked', '1');
	const s = q.toString();
	return s ? '?' + s : '';
}

/** The API query for f; the period is always sent, it has a default. */
export function apiQuery(f: Filters, before = '', limit = 0): string {
	const q = new URLSearchParams({ period: f.period });
	if (f.status) q.set('status', f.status);
	if (f.agent) q.set('agent', f.agent);
	if (f.source) q.set('source', f.source);
	if (f.blocked) q.set('blocked', '1');
	if (before) q.set('before', before);
	if (limit) q.set('limit', String(limit));
	return '?' + q.toString();
}

// --- words -----------------------------------------------------------------------------

export const SOURCES: Record<string, string> = {
	web: 'Chat',
	tui: 'Terminal chat',
	cli: 'mani run',
	daily: 'Daily trigger',
	cron: 'Cron trigger',
	webhook: 'Webhook',
	server: 'mani serve',
	mcp: 'MCP',
	flow: 'Flow',
	batch: 'mani batch'
};
export const sourceLabel = (s: string) => SOURCES[s] ?? (s || 'Unknown');

/** Sources where nobody can answer an "ask": a trigger has nobody in front of it. */
const UNATTENDED = new Set(['daily', 'cron', 'webhook']);

/** The run's tokens, in and out. */
export const tokens = (r: Run) => r.summary.inTokens + r.summary.outTokens;

export function duration(start: string, end: string, now = Date.now()): string {
	if (!start) return '';
	const ms = (end ? Date.parse(end) : now) - Date.parse(start);
	if (ms < 1000) return `${Math.max(0, ms)} ms`;
	const s = ms / 1000;
	if (s < 60) return `${s < 10 ? s.toFixed(1) : Math.round(s)} s`;
	const m = Math.floor(s / 60);
	if (m < 60) return `${m}m ${Math.round(s % 60)}s`;
	return `${Math.floor(m / 60)}h ${m % 60}m`;
}

/** "14:24", "yesterday 18:40" or "Nov 11, 16:05", in the viewer's time zone. */
export function when(iso: string, now = new Date()): string {
	if (!iso) return '';
	const d = new Date(iso);
	const time = d.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
	const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
	const diff = Math.round((day(now) - day(d)) / 86400000);
	if (diff === 0) return time;
	if (diff === 1) return `yesterday ${time}`;
	return `${d.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}, ${time}`;
}

// --- conversations -----------------------------------------------------------------------

/** Sources where a session is a conversation, one run per message. */
const INTERACTIVE = new Set(['web', 'tui', 'server']);

export type Group = {
	kind: 'conversation';
	key: string;
	runs: Run[]; // newest first, as the list
	status: string; // the worst of its runs
	agent: string;
	agentPath: string;
	model: string;
	source: string;
	startedAt: string; // of its first message
	lastAt: string; // of its last message
	tokens: number;
	blocked: number;
};
export type Row = { kind: 'run'; run: Run } | Group;

const RANK: Record<string, number> = { running: 0, error: 1, cancelled: 2, ok: 3 };

/**
 * groupRuns folds the runs of one conversation (decision M5-9) into one row at
 * the place of its newest message. A conversation with one message stays a
 * run; runs of triggers share a session across days and are never folded.
 */
export function groupRuns(runs: Run[]): Row[] {
	const bySession = new Map<string, Run[]>();
	for (const r of runs) {
		if (!r.sessionId || !INTERACTIVE.has(r.source)) continue;
		const key = r.source + ':' + r.sessionId;
		bySession.set(key, [...(bySession.get(key) ?? []), r]);
	}
	const rows: Row[] = [];
	const done = new Set<string>();
	for (const r of runs) {
		const key = r.source + ':' + r.sessionId;
		const g = bySession.get(key);
		if (!g || g.length < 2) {
			rows.push({ kind: 'run', run: r });
			continue;
		}
		if (done.has(key)) continue;
		done.add(key);
		rows.push({
			kind: 'conversation',
			key,
			runs: g,
			status: [...g].sort((a, b) => (RANK[a.status] ?? 9) - (RANK[b.status] ?? 9))[0].status,
			agent: r.agent,
			agentPath: r.agentPath,
			model: r.model,
			source: r.source,
			startedAt: g[g.length - 1].startedAt,
			lastAt: g[0].startedAt,
			tokens: g.reduce((n, x) => n + tokens(x), 0),
			blocked: g.reduce((n, x) => n + x.summary.blocked, 0)
		});
	}
	return rows;
}

// --- events ------------------------------------------------------------------------------

export type Tone = '' | 'ok' | 'warn' | 'error' | 'accent';
export type Kind = 'tool' | 'model' | 'rules' | 'other';
export type EventView = {
	icon: IconName;
	tone: Tone;
	kind: Kind;
	title: string;
	tool?: string;
	arg?: string;
	detail: string;
	output?: string;
	cut?: boolean;
	blocked?: boolean;
};

const str = (v: unknown) => (typeof v === 'string' ? v : '');
const num = (v: unknown) => (typeof v === 'number' ? v : 0);
const fmt = (n: number) => n.toLocaleString('en-US');
export const bytes = (n: number) => (n < 1024 ? `${n} B` : `${(n / 1024).toFixed(1)} KB`);

/** The argument of a tool call worth showing on one line. */
export function argOf(input: unknown): string {
	if (!input || typeof input !== 'object') return '';
	const o = input as Record<string, unknown>;
	for (const k of ['command', 'path', 'pattern', 'url', 'task', 'query', 'subagent']) {
		if (typeof o[k] === 'string' && o[k]) return o[k] as string;
	}
	const s = JSON.stringify(o);
	return s === '{}' ? '' : s;
}

/** What an answer to an "ask" means, said for the run it happened in. */
export function decisionText(decision: string, source: string): { text: string; tone: Tone; blocked: boolean } {
	const nobody = UNATTENDED.has(source);
	const where = source === 'web' ? ' in the chat' : source === 'tui' ? ' in the terminal' : '';
	switch (decision) {
		case 'allowed':
			return { text: nobody ? 'Allowed by the trigger policy' : `Allowed once${where}`, tone: 'ok', blocked: false };
		case 'allowed_always':
			return { text: `Allowed for the rest of the session${where}`, tone: 'ok', blocked: false };
		case 'remembered':
			return { text: 'Allowed without asking: allowed earlier in this conversation', tone: 'ok', blocked: false };
		case 'expired':
			return { text: 'Denied: nobody answered in time', tone: 'warn', blocked: true };
		case 'unattended':
			return { text: 'Denied: nobody to ask', tone: 'warn', blocked: true };
		default:
			return { text: `Denied${where}`, tone: 'error', blocked: true };
	}
}

/** view turns a journal event into a timeline line. */
export function view(ev: RunEvent, source: string, ask?: string): EventView {
	const d = ev.data ?? {};
	switch (ev.kind) {
		case 'run_start':
			return { icon: 'play', tone: 'accent', kind: 'other', title: 'Run started', detail: sourceLabel(str(d.source) || source) };
		case 'llm_call':
			return {
				icon: 'spark',
				tone: '',
				kind: 'model',
				title: d.purpose === 'compaction' ? 'Summarizing earlier messages' : 'Model call',
				detail: `${num(d.messages)} messages, ${num(d.tools)} tools`
			};
		case 'llm_response':
			return {
				icon: 'spark',
				tone: '',
				kind: 'model',
				title: 'Model answered',
				detail: `${fmt(num(d.in_tokens))} in / ${fmt(num(d.out_tokens))} out${d.stop_reason === 'tool_use' ? ', asked for tools' : ''}`
			};
		case 'tool_call':
			return { icon: 'tool', tone: '', kind: 'tool', title: 'Called', tool: str(d.tool), arg: argOf(d.input), detail: '' };
		case 'tool_result': {
			const err = d.is_error === true;
			const len = num(d.result_len);
			const preview = str(d.preview);
			const cut = preview.length > 0 && preview.length < len;
			return {
				icon: err ? 'x' : 'check',
				tone: err ? 'error' : '',
				kind: 'tool',
				title: err ? 'Failed' : 'Returned',
				tool: str(d.tool),
				detail: preview ? (cut ? `${bytes(len)}, preview below` : bytes(len)) : `${bytes(len)}, output not recorded`,
				output: preview || undefined,
				cut
			};
		}
		case 'permission': {
			const t = decisionText(str(d.decision), source);
			return { icon: 'hand', tone: t.tone, kind: 'rules', title: 'Asked', tool: str(d.tool), arg: ask, detail: t.text, blocked: t.blocked };
		}
		case 'guardrail':
			if (d.action === 'mask')
				return { icon: 'eye', tone: 'ok', kind: 'rules', title: 'Masked', tool: str(d.tool), detail: 'A redaction rule rewrote the output before the model saw it' };
			return {
				icon: 'lock',
				tone: 'error',
				kind: 'rules',
				title: 'Blocked',
				tool: str(d.tool),
				detail: d.label === 'permission' ? 'The policy denies this tool' : `Rule "${str(d.label)}"`,
				blocked: true
			};
		case 'run_end': {
			const st = str(d.status);
			return {
				icon: 'flag',
				tone: st === 'ok' ? 'ok' : st === 'error' ? 'error' : '',
				kind: 'other',
				title: st === 'ok' ? 'Run ended' : st === 'cancelled' ? 'Run cancelled' : 'Run ended with an error',
				detail: str(d.error) || st
			};
		}
		default:
			return { icon: 'circle', tone: '', kind: 'other', title: ev.kind, detail: JSON.stringify(d) };
	}
}

export type Line = EventView & { ev: RunEvent; depth: number; rules: boolean };

/**
 * timeline turns the events into lines, one per step. A tool call, the answer
 * to its "ask", a rule that blocked or masked it and its result are one step:
 * they share a line, so each call reads once and in order.
 */
export function timeline(events: RunEvent[], source: string): Line[] {
	const used = new Set<number>();
	const out: Line[] = [];
	events.forEach((ev, i) => {
		if (used.has(i) || ev.kind === 'llm_call') return; // a call's response says the same, with the tokens
		if (ev.kind !== 'tool_call') {
			const v = view(ev, source);
			out.push({ ...v, ev, depth: ev.depth, rules: v.kind === 'rules' });
			return;
		}
		const tool = str(ev.data?.tool);
		const parts: string[] = [];
		let line: Line = { ...view(ev, source), ev, depth: ev.depth, rules: false };
		for (let k = i + 1; k < events.length; k++) {
			const e = events[k];
			if (used.has(k) || e.depth !== ev.depth || str(e.data?.tool) !== tool) continue;
			if (e.kind === 'tool_call') break; // the next call of the same tool: ours had no more
			if (e.kind === 'permission') {
				used.add(k);
				const t = decisionText(str(e.data?.decision), source);
				parts.push(t.text);
				line.rules = true;
				if (t.blocked) {
					line = { ...line, icon: 'hand', tone: t.tone, blocked: true };
					break;
				}
			} else if (e.kind === 'guardrail') {
				used.add(k);
				line.rules = true;
				const g = view(e, source);
				if (e.data?.action === 'mask') {
					parts.push('masked by a redaction rule');
				} else {
					parts.push(g.detail);
					line = { ...line, icon: 'lock', tone: 'error', title: 'Blocked', blocked: true };
					break;
				}
			} else if (e.kind === 'tool_result') {
				used.add(k);
				const r = view(e, source);
				parts.push(r.detail);
				line = { ...line, icon: r.icon === 'x' ? 'x' : 'tool', tone: r.tone || line.tone, title: r.title, output: r.output, cut: r.cut };
				break;
			}
		}
		if (!parts.length) parts.push('waiting for the result');
		out.push({ ...line, detail: parts.join(' - ') });
	});
	return out;
}

// --- turning the journal on ------------------------------------------------------------

/**
 * journalOn turns on the journal in the text of a manifest (decision M5-4),
 * changing only those values: comments and the rest of the file stay as they
 * are. A path already written is kept; otherwise each agent gets its own.
 */
export function journalOn(text: string, name: string): string {
	const doc = new Manifest(text);
	if (!doc.ok) throw new Error('The file has YAML errors: fix them in the editor first.');
	doc.set(['observability', 'journal', 'enabled'], true);
	if (!doc.get(['observability', 'journal', 'path'])) doc.set(['observability', 'journal', 'path'], `runs/${name}`);
	return doc.toString();
}

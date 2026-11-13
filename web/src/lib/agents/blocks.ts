import type { Problem } from '../api/types.ts';

// The eight blocks of a manifest, in file order, as the editor shows them.
export const BLOCKS = [
	{ key: 'identity', title: 'Name and model', hint: 'Who it is and what thinks' },
	{ key: 'capabilities', title: 'Tools', hint: 'What it can use' },
	{ key: 'context', title: 'Memory', hint: 'What it sees and keeps' },
	{ key: 'output', title: 'Answer format', hint: 'Text or checked JSON' },
	{ key: 'policy', title: 'Permissions', hint: 'What it may do alone' },
	{ key: 'limits', title: 'Limits', hint: 'How much it may spend' },
	{ key: 'run', title: 'Start', hint: 'When it runs by itself' },
	{ key: 'observability', title: 'Record', hint: 'What it leaves behind' }
] as const;

export type BlockKey = (typeof BLOCKS)[number]['key'];

/** The block a problem belongs to; null for problems with the whole file. */
export function blockOf(p: Problem): BlockKey | null {
	const head = p.path.split(/[.[]/)[0];
	return BLOCKS.some((b) => b.key === head) ? (head as BlockKey) : null;
}

export function problemsIn(ps: Problem[], key: BlockKey): Problem[] {
	return ps.filter((p) => blockOf(p) === key);
}

/** A field path for data-path attributes: ["policy", "rules", 1, "pattern"] → "policy.rules[1].pattern". */
export function pathKey(path: (string | number)[]): string {
	return path.reduce<string>((acc, seg) => (typeof seg === 'number' ? `${acc}[${seg}]` : acc ? `${acc}.${seg}` : seg), '');
}

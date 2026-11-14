import {
	isMap,
	isScalar,
	isSeq,
	parseDocument,
	stringify,
	type Document,
	type Node,
	type Pair,
	type Scalar,
	type ScalarTag,
	type YAMLMap,
	type YAMLSeq
} from 'yaml';

// A manifest edited as text. The yaml library only reads: it says where every
// node sits in the file. Each edit then replaces exactly those characters, so
// comments, their alignment, key order, quoting and every untouched line stay
// byte for byte as they were (decision M3-1). Regenerating the text from the
// parsed document would not do: it moves and re-aligns comments.

export type Path = (string | number)[];

/** !include stays !include: it reads as a marker and is never rewritten. */
export class Include {
	constructor(readonly file: string) {}
}

const includeTag: ScalarTag = {
	tag: '!include',
	identify: (v) => v instanceof Include,
	resolve: (str) => new Include(str),
	stringify: (item) => String((item.value as Include).file)
};

export const BUILTIN_TOOLS = ['read', 'glob', 'grep', 'planning', 'write', 'edit', 'delete', 'fetch', 'bash', 'delegate'];

type Range = [number, number, number];
const rangeOf = (n: Node) => n.range as Range;

const isEmpty = (v: unknown) =>
	v === '' || v === null || v === undefined || (typeof v === 'number' && Number.isNaN(v)) || (Array.isArray(v) && v.length === 0);

export class Manifest {
	text: string;
	doc!: Document.Parsed;

	constructor(text: string) {
		this.text = text;
		this.reparse();
	}

	private reparse() {
		this.doc = parseDocument(this.text, { customTags: [includeTag], uniqueKeys: false });
	}

	private splice(start: number, end: number, insert: string) {
		this.text = this.text.slice(0, start) + insert + this.text.slice(end);
		this.reparse();
	}

	/** Syntax errors: the fields cannot be trusted until the text parses. */
	get errors(): string[] {
		return this.doc.errors.map((e) => e.message);
	}

	get ok(): boolean {
		return this.doc.errors.length === 0;
	}

	toString(): string {
		return this.text;
	}

	// --- reading ---------------------------------------------------------------

	get(path: Path): unknown {
		const v = this.doc.getIn(path);
		return isScalar(v) ? v.value : v;
	}

	str(path: Path): string {
		const v = this.get(path);
		if (v instanceof Include || v === undefined || v === null || typeof v === 'object') return '';
		return String(v);
	}

	num(path: Path): number | '' {
		const v = this.get(path);
		return typeof v === 'number' ? v : '';
	}

	bool(path: Path, fallback = false): boolean {
		const v = this.get(path);
		return typeof v === 'boolean' ? v : fallback;
	}

	has(path: Path): boolean {
		return this.doc.hasIn(path);
	}

	includeOf(path: Path): string | null {
		const v = this.get(path);
		return v instanceof Include ? v.file : null;
	}

	list(path: Path): unknown[] {
		const v = this.doc.getIn(path);
		return isSeq(v) ? (v.toJSON() as unknown[]) : [];
	}

	strings(path: Path): string[] {
		return this.list(path).map((x) => (x === null || x === undefined ? '' : String(x)));
	}

	map(path: Path): Record<string, unknown> {
		const v = this.doc.getIn(path);
		return isMap(v) ? (v.toJSON() as Record<string, unknown>) : {};
	}

	// --- text geometry ---------------------------------------------------------

	private lineStart(off: number) {
		return this.text.lastIndexOf('\n', off - 1) + 1;
	}

	/** The offset just past the newline of the line holding off. */
	private lineEnd(off: number) {
		const i = this.text.indexOf('\n', off);
		return i < 0 ? this.text.length : i + 1;
	}

	private col(off: number) {
		return off - this.lineStart(off);
	}

	/** The last offset that belongs to a node's own text, comments excluded. */
	private lastChar(n: Node) {
		const [, valueEnd] = rangeOf(n);
		let i = valueEnd - 1;
		while (i > 0 && (this.text[i] === '\n' || this.text[i] === ' ')) i--;
		return i;
	}

	/** Lines covered by a map entry or a list item, ready to cut. */
	private linesOf(first: Node, last: Node | null) {
		const start = this.lineStart(rangeOf(first)[0]);
		const end = this.lineEnd(this.lastChar(last ?? first));
		return [start, end] as const;
	}

	// --- rendering -------------------------------------------------------------

	private scalarText(v: unknown, like?: Scalar, keyCol = 0, flow = false): string {
		if (typeof v !== 'string') return String(v);
		if (like?.type === 'QUOTE_DOUBLE') return JSON.stringify(v);
		if (like?.type === 'QUOTE_SINGLE') return `'${v.replace(/'/g, "''")}'`;
		if (flow) return this.flowText(v);
		if (v.includes('\n') || like?.type === 'BLOCK_LITERAL' || like?.type === 'BLOCK_FOLDED') {
			const pad = ' '.repeat(keyCol + 2);
			const body = v.replace(/\n$/, '').split('\n').map((l) => (l ? pad + l : ''));
			return `${v.endsWith('\n\n') ? '|+' : v.endsWith('\n') ? '|' : '|-'}\n${body.join('\n')}`;
		}
		return stringify(v, { lineWidth: 0 }).trimEnd();
	}

	private flowText(v: unknown): string {
		if (Array.isArray(v)) return `[${v.map((x) => this.flowText(x)).join(', ')}]`;
		if (v && typeof v === 'object') {
			return `{ ${Object.entries(v)
				.map(([k, x]) => `${this.flowText(k)}: ${this.flowText(x)}`)
				.join(', ')} }`;
		}
		return typeof v === 'string'
			? stringify([v], { collectionStyle: 'flow', flowCollectionPadding: false, lineWidth: 0 }).trimEnd().slice(1, -1)
			: String(v);
	}

	/** Block YAML for a value, every line indented by col. */
	private blockText(v: unknown, col: number): string {
		const pad = ' '.repeat(col);
		return stringify(v, { lineWidth: 0, indent: 2 })
			.replace(/\n$/, '')
			.split('\n')
			.map((l) => pad + l)
			.join('\n');
	}

	// --- writing ---------------------------------------------------------------

	/** Sets a value. An empty string, null, NaN or an empty list removes the key. */
	set(path: Path, value: unknown): void {
		if (isEmpty(value)) return this.delete(path);
		const node = this.doc.getIn(path, true) as Node | undefined;
		if (node === undefined || node === null) return this.insert(path, value);

		if (isScalar(node)) {
			if (value && typeof value === 'object') {
				this.delete(path);
				return this.insert(path, value);
			}
			if (node.value === value) return;
			const [start, valueEnd] = rangeOf(node);
			const keyCol = this.col(this.lineStart(start) + (this.text.slice(this.lineStart(start)).match(/^\s*/)?.[0].length ?? 0));
			let end = valueEnd;
			if (node.type === 'BLOCK_LITERAL' || node.type === 'BLOCK_FOLDED') end = this.lineEnd(this.lastChar(node)) - 1;
			const parent = (path.length > 1 ? this.doc.getIn(path.slice(0, -1), true) : this.doc.contents) as Node | null;
			const flow = (isMap(parent) || isSeq(parent)) && parent.flow === true;
			let replacement = this.scalarText(value, node, keyCol, flow);
			if (replacement.startsWith('|+\n')) {
				end += this.text.slice(end).match(/^(?:[ \t]*\r?\n)+/)?.[0].length ?? 0;
				replacement += '\n';
			}
			return this.splice(start, end, replacement);
		}

		const coll = node as YAMLMap | YAMLSeq;
		if (!value || typeof value !== 'object') {
			this.delete(path);
			return this.insert(path, value);
		}
		const [start] = rangeOf(coll);
		if (coll.flow) {
			return this.splice(start, rangeOf(coll)[1], this.flowText(value));
		}
		// A block collection is rewritten in place, at its own indentation.
		const col = this.col(isSeq(coll) ? this.text.lastIndexOf('-', start) : start);
		const end = this.lineEnd(this.lastChar(coll));
		const block = this.blockText(value, col);
		return this.splice(this.lineStart(start), end, block + '\n');
	}

	/** Adds a key that does not exist, under the deepest map that does. */
	private insert(path: Path, value: unknown): void {
		let k = path.length - 1;
		while (k > 0 && !this.doc.hasIn(path.slice(0, k))) k--;
		const parentPath = path.slice(0, k);
		const parent = (k === 0 ? this.doc.contents : this.doc.getIn(parentPath, true)) as Node | null;

		let nested: unknown = value;
		for (let i = path.length - 1; i >= k; i--) nested = { [path[i]]: nested };

		if (parent === null || (isScalar(parent) && (parent.value === null || parent.value === undefined))) {
			if (k === 0) {
				const sep = this.text && !this.text.endsWith('\n') ? '\n' : '';
				return this.splice(this.text.length, this.text.length, sep + this.blockText(nested, 0) + '\n');
			}
			// "policy:" with nothing under it: replace the empty value.
			const holder = this.doc.getIn(parentPath.slice(0, -1), true) as YAMLMap;
			const pair = holder.items.find((p) => (isScalar(p.key) ? p.key.value : p.key) === parentPath.at(-1)) as Pair<Scalar, Node>;
			const keyCol = this.col(rangeOf(pair.key)[0]);
			const lineEnd = this.lineEnd(rangeOf(pair.key)[1]);
			const inner = (nested as Record<string, unknown>)[String(path[k])];
			const block = this.blockText({ [path[k]]: inner }, keyCol + 2);
			const keyLine = this.text.slice(this.lineStart(rangeOf(pair.key)[0]), lineEnd).replace(/\s*\n?$/, '');
			return this.splice(this.lineStart(rangeOf(pair.key)[0]), lineEnd, `${keyLine}\n${block}\n`);
		}

		if (isMap(parent)) {
			const map = parent as YAMLMap;
			if (map.flow) {
				const [k0, v0] = Object.entries(nested as object)[0];
				return this.flowInsert(map, `${k0}: ${this.flowText(v0)}`);
			}
			const last = map.items.at(-1) as Pair<Node, Node | null> | undefined;
			if (!last) {
				return this.splice(this.text.length, this.text.length, this.blockText(nested, 0) + '\n');
			}
			const col = this.col(rangeOf(map.items[0].key as Node)[0]);
			const at = this.lineEnd(this.lastChar((last.value as Node) ?? (last.key as Node)));
			const sep = at === this.text.length && !this.text.endsWith('\n') ? '\n' : '';
			return this.splice(at, at, sep + this.blockText(nested, col) + '\n');
		}
		throw new Error(`cannot add ${path.join('.')}: ${parentPath.join('.') || 'the file'} is not a map`);
	}

	/** Deletes a key or a list item, and the maps it leaves empty, so no `policy:` stays behind. */
	delete(path: Path): void {
		if (!this.doc.hasIn(path)) return;
		const parentPath = path.slice(0, -1);
		const parent = (parentPath.length ? this.doc.getIn(parentPath, true) : this.doc.contents) as YAMLMap | YAMLSeq;
		const key = path.at(-1)!;

		if (parent.flow) {
			const items = parent.items as (Pair<Node, Node | null> | Node)[];
			const i = isSeq(parent) ? Number(key) : items.findIndex((p) => { const k = (p as Pair<Node, Node>).key; return (isScalar(k) ? k.value : k) === key; });
			if (items.length === 1) {
				if (parentPath.length) return this.delete(parentPath);
				return this.splice(rangeOf(parent)[0], rangeOf(parent)[1], isSeq(parent) ? '[]' : '{}');
			}
			// Cut the item and one separator, leaving every other item as written.
			const startOf = (it: Pair<Node, Node | null> | Node) => rangeOf(isSeq(parent) ? (it as Node) : (it as Pair<Node, Node>).key)[0];
			const endOf = (it: Pair<Node, Node | null> | Node) => {
				const n = isSeq(parent) ? (it as Node) : ((it as Pair<Node, Node | null>).value ?? (it as Pair<Node, Node>).key);
				return this.lastChar(n) + 1;
			};
			const start = i === items.length - 1 ? endOf(items[i - 1]) : startOf(items[i]);
			const end = i === items.length - 1 ? endOf(items[i]) : startOf(items[i + 1]);
			return this.splice(start, end, '');
		}

		if (isSeq(parent)) {
			const item = parent.items[Number(key)] as Node;
			if (parent.items.length === 1 && parentPath.length) return this.delete(parentPath);
			const [start, end] = this.linesOf(item, null);
			return this.splice(start, end, '');
		}

		const pair = (parent as YAMLMap).items.find((p) => (isScalar(p.key) ? p.key.value : p.key) === key) as Pair<Node, Node | null>;
		if (parent.items.length === 1 && parentPath.length) return this.delete(parentPath);
		const [start, end] = this.linesOf(pair.key, pair.value);
		this.splice(start, end, '');
	}

	/** Adds an entry before the closing bracket of a flow collection. */
	private flowInsert(coll: YAMLMap | YAMLSeq, entry: string) {
		const close = this.text.lastIndexOf(isSeq(coll) ? ']' : '}', rangeOf(coll)[1] - 1);
		if (coll.items.length === 0) return this.splice(close, close, isSeq(coll) ? entry : ` ${entry} `);
		const last = coll.items.at(-1) as Pair<Node, Node | null> | Node;
		const lastNode = isSeq(coll) ? (last as Node) : ((last as Pair<Node, Node | null>).value ?? (last as Pair<Node, Node>).key);
		const at = this.lastChar(lastNode) + 1;
		this.splice(at, at, `, ${entry}`);
	}

	/** Appends an item to a list, creating the list when missing. */
	push(path: Path, item: unknown) {
		const seq = this.doc.getIn(path, true) as Node | undefined;
		if (!isSeq(seq)) return this.set(path, [item]);
		if (seq.flow) return this.flowInsert(seq, this.flowText(item));
		const first = seq.items[0] as Node;
		const dashCol = this.col(this.text.lastIndexOf('-', rangeOf(first)[0]));
		const lines = stringify(item, { lineWidth: 0, indent: 2 }).replace(/\n$/, '').split('\n');
		const pad = ' '.repeat(dashCol);
		const block = lines.map((l, i) => (i === 0 ? `${pad}- ${l}` : `${pad}  ${l}`)).join('\n');
		const at = this.lineEnd(this.lastChar(seq.items.at(-1) as Node));
		const sep = at === this.text.length && !this.text.endsWith('\n') ? '\n' : '';
		this.splice(at, at, sep + block + '\n');
	}

	removeAt(path: Path, index: number) {
		this.delete([...path, index]);
	}

	// --- tools -----------------------------------------------------------------

	/** The tool names in capabilities.tools, whether written as `- read` or `- name: read`. */
	toolNames(): string[] {
		return this.list(['capabilities', 'tools']).map((t) =>
			typeof t === 'string' ? t : t && typeof t === 'object' && 'name' in t ? String((t as { name: unknown }).name) : ''
		);
	}

	/** Adds or removes a built-in tool, in the style of the list already there. */
	setTool(name: string, on: boolean) {
		const names = this.toolNames();
		if (on) {
			if (names.includes(name)) return;
			const seq = this.doc.getIn(['capabilities', 'tools'], true);
			const scalarStyle = isSeq(seq) && seq.items.some((i) => isScalar(i));
			this.push(['capabilities', 'tools'], scalarStyle ? name : { name });
		} else {
			const i = names.indexOf(name);
			if (i >= 0) this.removeAt(['capabilities', 'tools'], i);
		}
	}

	/** Entries a form does not edit: external tools, which carry a command. */
	customTools(): { name: string; command: string }[] {
		return this.list(['capabilities', 'tools']).flatMap((t) => {
			const o = t as Record<string, unknown> | null;
			return o && typeof o === 'object' && 'command' in o ? [{ name: String(o.name ?? ''), command: String(o.command) }] : [];
		});
	}
}

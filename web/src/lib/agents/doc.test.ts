import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { Manifest } from './doc.ts';

const handWritten = `# The morning summary.
identity:
  name: morning   # short on purpose
  provider: ollama
  model: "qwen3.5:9b"
  prompt: !include prompt.md

capabilities:
  tools:
    - name: read
    - name: grep
policy:
  tools:
    read: allow   # reading is harmless
  rules:
    - { tool: bash, pattern: 'rm\\s+-rf', action: deny }
`;

describe('Manifest', () => {
	it('round-trips a hand-written file unchanged', () => {
		expect(new Manifest(handWritten).toString()).toBe(handWritten);
	});

	it('round-trips the init template and the examples', () => {
		for (const f of ['../cmd/mani/templates/agent.yaml', '../_examples/flow/classify.yaml']) {
			const text = readFileSync(f, 'utf8');
			expect(new Manifest(text).toString(), f).toBe(text);
		}
	});

	it('changes one value and nothing else', () => {
		const m = new Manifest(handWritten);
		m.set(['identity', 'name'], 'evening');
		m.set(['policy', 'tools', 'grep'], 'ask');
		const out = m.toString();
		expect(out).toContain('name: evening   # short on purpose');
		expect(out).toContain('grep: ask');
		expect(out).toContain('read: allow   # reading is harmless');
		expect(out).toContain('# The morning summary.');
		expect(out).toContain('prompt: !include prompt.md');
		expect(out).toContain(`pattern: 'rm\\s+-rf'`);
	});

	it('keeps the quoting of a scalar it edits', () => {
		const m = new Manifest(handWritten);
		m.set(['identity', 'model'], 'llama3.2:3b');
		expect(m.toString()).toContain('model: "llama3.2:3b"');
	});

	it('removes empty values and the maps they leave behind', () => {
		const m = new Manifest('identity:\n  name: x\nlimits:\n  max_tokens: 10\n');
		m.set(['limits', 'max_tokens'], '');
		expect(m.toString()).toBe('identity:\n  name: x\n');
	});

	it('reads an !include without losing it', () => {
		const m = new Manifest(handWritten);
		expect(m.includeOf(['identity', 'prompt'])).toBe('prompt.md');
		expect(m.str(['identity', 'prompt'])).toBe('');
	});

	it('adds and removes tools in the style of the list', () => {
		const block = new Manifest(handWritten);
		block.setTool('bash', true);
		expect(block.toString()).toContain('    - name: bash');
		block.setTool('read', false);
		expect(block.toolNames()).toEqual(['grep', 'bash']);

		const flow = new Manifest('capabilities:\n  tools: [read]\n');
		flow.setTool('grep', true);
		expect(flow.toolNames()).toEqual(['read', 'grep']);
		expect(flow.toString()).toBe('capabilities:\n  tools: [read, grep]\n');
		flow.setTool('read', false);
		flow.setTool('grep', false);
		expect(flow.toString()).toBe('');
	});

	it('creates a block that does not exist yet', () => {
		const m = new Manifest('identity:\n  name: x\n');
		m.set(['limits', 'max_tool_calls'], 6);
		expect(m.toString()).toBe('identity:\n  name: x\nlimits:\n  max_tool_calls: 6\n');
	});

	it('keeps the alignment of trailing comments around an edit', () => {
		const t = readFileSync('../cmd/mani/templates/agent.yaml', 'utf8');
		const m = new Manifest(t);
		m.set(['identity', 'provider'], 'openai');
		expect(m.toString()).toBe(t.replace('  provider: ollama           #', '  provider: openai           #'));
	});

	it('edits a block scalar prompt', () => {
		const m = new Manifest('identity:\n  prompt: |\n    old line\n  model: m\n');
		m.set(['identity', 'prompt'], 'first\nsecond\n');
		expect(m.toString()).toBe('identity:\n  prompt: |\n    first\n    second\n  model: m\n');
	});

	it('adds a key under an empty block', () => {
		const m = new Manifest('identity:\n  name: x\npolicy:\nlimits:\n  max_tokens: 5\n');
		m.set(['policy', 'tools', 'bash'], 'deny');
		expect(m.toString()).toBe('identity:\n  name: x\npolicy:\n  tools:\n    bash: deny\nlimits:\n  max_tokens: 5\n');
	});

	it('appends to a flow map and to block lists', () => {
		const m = new Manifest('run:\n  triggers:\n    - { type: daily, at: "07:00" }\n');
		m.set(['run', 'triggers', 0, 'prompt'], 'go');
		expect(m.toString()).toBe('run:\n  triggers:\n    - { type: daily, at: "07:00", prompt: go }\n');
		m.push(['run', 'triggers'], { type: 'every', every: '30m' });
		expect(m.toString()).toBe('run:\n  triggers:\n    - { type: daily, at: "07:00", prompt: go }\n    - type: every\n      every: 30m\n');
		m.removeAt(['run', 'triggers'], 0);
		expect(m.toString()).toBe('run:\n  triggers:\n    - type: every\n      every: 30m\n');
	});

	it('replaces a list in place, keeping its position', () => {
		const m = new Manifest('policy:\n  network:\n    allow:\n      - a.com\n    deny: [x]\nlimits:\n  max_tokens: 1\n');
		m.set(['policy', 'network', 'allow'], ['a.com', 'b.com']);
		m.set(['policy', 'network', 'deny'], ['x', 'y']);
		expect(m.toString()).toBe('policy:\n  network:\n    allow:\n      - a.com\n      - b.com\n    deny: [x, y]\nlimits:\n  max_tokens: 1\n');
	});

	it('reports a syntax error instead of guessing', () => {
		const m = new Manifest('identity:\n  name: [oops\n');
		expect(m.ok).toBe(false);
		expect(m.errors.length).toBeGreaterThan(0);
	});

	it('lists external tools apart from the built-in ones', () => {
		const m = new Manifest('capabilities:\n  tools:\n    - read\n    - name: weather\n      command: python3\n');
		expect(m.customTools()).toEqual([{ name: 'weather', command: 'python3' }]);
	});
});

describe('Manifest, flow collections', () => {
	it('deletes one entry of a flow map and keeps the others as written', () => {
		const m = new Manifest('run:\n  triggers:\n    - { type: daily, at: "07:00", prompt: go }\n');
		m.delete(['run', 'triggers', 0, 'prompt']);
		expect(m.toString()).toBe('run:\n  triggers:\n    - { type: daily, at: "07:00" }\n');
		m.delete(['run', 'triggers', 0, 'type']);
		expect(m.toString()).toBe('run:\n  triggers:\n    - { at: "07:00" }\n');
	});

	it('pushes to and deletes from a flow list', () => {
		const m = new Manifest('capabilities:\n  tools: [read, "grep"]\n');
		m.push(['capabilities', 'tools'], 'bash');
		expect(m.toString()).toBe('capabilities:\n  tools: [read, "grep", bash]\n');
		m.removeAt(['capabilities', 'tools'], 1);
		expect(m.toString()).toBe('capabilities:\n  tools: [read, bash]\n');
	});
});

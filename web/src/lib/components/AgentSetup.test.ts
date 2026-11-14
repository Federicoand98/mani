import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { flushSync } from 'svelte';
import { createClassComponent } from 'svelte/legacy';
import AgentSetup from './AgentSetup.svelte';
import { ManifestState } from '../agents/editor.svelte.ts';
import { api } from '../api/client.ts';
import { app } from '../state.svelte.ts';

class Draft extends ManifestState {
	override scheduleCheck() {}
}

let component: ReturnType<typeof createClassComponent> | undefined;
const onerrors = vi.fn<(errors: string[]) => void>();
const template = `# Keep this heading.
identity:
  name: helper  # Keep the name comment.
  provider: local
  model: small
  prompt: !include instructions.md
capabilities:
  tools:
    - read  # Reading stays included.
    - bash
    - name: external
      command: node
      args: ["--safe", ""]
      env: { KEY: "\${TOKEN}" }
      schema:
        type: object
        properties: { query: { type: string } }
policy:
  tools: { default: ask, read: allow }
# Keep this final note.
`;

function setup(section: 'agent' | 'tools' | 'policy' | 'context' | 'output' | 'limits' | 'run' | 'records', text = template) {
	const ed = new Draft('helper.yaml', text);
	component = createClassComponent({ component: AgentSetup, target: document.body, props: { ed, section, onerrors } });
	flushSync();
	return ed;
}

function control(path: string) {
	const found = [...document.querySelectorAll<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>('input[data-path], textarea[data-path], select[data-path]')]
		.find((element) => element.dataset.path === path);
	if (!found) throw new Error(`Missing control ${path}`);
	return found;
}

function input(path: string, value: string) {
	const element = control(path);
	element.value = value;
	element.dispatchEvent(new Event(element.tagName === 'SELECT' ? 'change' : 'input', { bubbles: true }));
	flushSync();
}

function click(text: string, within: ParentNode = document) {
	const button = [...within.querySelectorAll<HTMLButtonElement>('button')].find((item) => item.textContent?.trim() === text);
	if (!button) throw new Error(`Missing button ${text}`);
	button.click();
	flushSync();
}

function verdict(name: string, text: string) {
	const group = [...document.querySelectorAll<HTMLElement>('.seg[data-path]')].find((item) => item.dataset.path === `policy.tools.${name}`);
	if (!group) throw new Error(`Missing policy ${name}`);
	click(text, group);
}

beforeEach(() => {
	app.providersLoaded = true;
	app.providers = [{ name: 'local', label: 'Local', enabled: true, local: true, auth: 'none', baseUrl: '', custom: false }];
	vi.spyOn(api, 'models').mockResolvedValue({ models: ['small', 'large'] });
	onerrors.mockClear();
});

afterEach(() => {
	component?.$destroy();
	component = undefined;
	document.body.innerHTML = '';
	vi.restoreAllMocks();
});

it('mounts every section without changing a commented template or its include', () => {
	const ed = setup('agent');
	expect(ed.text).toBe(template);
	expect(document.querySelectorAll('[data-section]')).toHaveLength(8);
	expect(document.querySelectorAll('[data-section]:not([hidden])')).toHaveLength(1);
	expect(document.getElementById('f-prompt')).toBeNull();
	expect(document.querySelector('[data-path="identity.prompt"]')?.textContent).toContain('instructions.md');
	input('identity.name', 'reader');
	expect(ed.text).toContain('name: reader  # Keep the name comment.');
	expect(ed.text).toContain('prompt: !include instructions.md');
	expect(ed.text).toContain('# Keep this final note.');
	expect(document.querySelector('details')).toBeNull();
});

it('separates built-in inclusion from inherited and explicit policy decisions', () => {
	const ed = setup('policy');
	expect(document.querySelector('#setup-policy-access')?.textContent).not.toContain('Off');
	verdict('bash', 'Deny');
	expect(ed.str(['policy', 'tools', 'bash'])).toBe('deny');
	expect(ed.toolNames()).toContain('bash');
	verdict('default', 'Allow');
	expect(ed.str(['policy', 'tools', 'default'])).toBe('allow');
	expect(ed.str(['policy', 'tools', 'read'])).toBe('allow');
	(document.querySelector('[aria-label="Use default for bash"]') as HTMLButtonElement).click();
	flushSync();
	expect(ed.has(['policy', 'tools', 'bash'])).toBe(false);
	const builtin = document.querySelector<HTMLInputElement>('[data-tool="bash"]')!;
	builtin.checked = false;
	builtin.dispatchEvent(new Event('change', { bubbles: true }));
	flushSync();
	expect(ed.toolNames()).not.toContain('bash');
	expect(ed.str(['policy', 'tools', 'default'])).toBe('allow');
	expect(ed.text).toContain('- read  # Reading stays included.');
});

it('adds arbitrary MCP policy overrides and blocks duplicate names', () => {
	const ed = setup('policy');
	const name = document.querySelector<HTMLInputElement>('.override-add input')!;
	name.value = 'docs.search';
	name.dispatchEvent(new Event('input', { bubbles: true }));
	click('+ Tool override');
	expect(ed.str(['policy', 'tools', 'docs.search'])).toBe('ask');
	verdict('docs.search', 'Deny');
	expect(ed.str(['policy', 'tools', 'docs.search'])).toBe('deny');
	name.value = 'read';
	name.dispatchEvent(new Event('input', { bubbles: true }));
	click('+ Tool override');
	expect(onerrors.mock.lastCall?.[0]).toContain('Choose a unique tool name.');
	expect(ed.str(['policy', 'tools', 'read'])).toBe('allow');
	(document.querySelector('[aria-label="Remove override docs.search"]') as HTMLButtonElement).click();
	flushSync();
	expect(ed.has(['policy', 'tools', 'docs.search'])).toBe(false);
});

it('keeps invalid JSON drafts and errors across section changes and leaves valid YAML untouched', () => {
	const ed = setup('output');
	const before = ed.text;
	input('output.schema', '{"type":');
	expect(ed.text).toBe(before);
	expect(control('output.schema').getAttribute('aria-invalid')).toBe('true');
	expect(onerrors.mock.lastCall?.[0].some((error) => error.startsWith('output.schema:'))).toBe(true);
	component!.$set({ section: 'agent' });
	flushSync();
	expect((document.querySelector('[data-section="output"]') as HTMLElement).hidden).toBe(true);
	component!.$set({ section: 'output' });
	flushSync();
	expect(control('output.schema').value).toBe('{"type":');
	input('output.schema', '{"type":"array"}');
	expect(ed.has(['output', 'schema'])).toBe(false);
	expect(onerrors.mock.lastCall?.[0].join()).toContain('Schema type must be "object".');
	input('output.schema', '{"type":"object","properties":{"label":{"type":"string","enum":["yes","no"]}},"required":["label"]}');
	expect(ed.str(['output', 'schema', 'type'])).toBe('object');
	expect(ed.strings(['output', 'schema', 'properties', 'label', 'enum'])).toEqual(['yes', 'no']);
	expect(onerrors.mock.lastCall?.[0]).toEqual([]);
	expect(ed.text).toContain('prompt: !include instructions.md');
	input('output.schema', '');
	expect(ed.has(['output'])).toBe(false);
});

it('edits every custom tool field, validates environment and arguments, and preserves unrelated comments', () => {
	const ed = setup('tools');
	click('Custom');
	input('capabilities.tools[2].name', 'script');
	input('capabilities.tools[2].description', 'Search docs');
	input('capabilities.tools[2].command', 'python');
	input('capabilities.tools[2].risk', 'execute');
	expect(ed.str(['capabilities', 'tools', 2, 'risk']), ed.text).toBe('execute');
	input('capabilities.tools[2].args', '["-c", "", "two words"]');
	input('capabilities.tools[2].env', '{"KEY":42}');
	expect(ed.map(['capabilities', 'tools', 2, 'env'])).toEqual({ KEY: '${TOKEN}' });
	expect(onerrors.mock.lastCall?.[0].join()).toContain('Environment values must be strings.');
	input('capabilities.tools[2].env', '{"KEY":"${NEW_TOKEN}"}');
	input('capabilities.tools[2].schema', '{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}');
	expect(ed.str(['capabilities', 'tools', 2, 'command'])).toBe('python');
	expect(ed.strings(['capabilities', 'tools', 2, 'args'])).toEqual(['-c', '', 'two words']);
	expect(ed.str(['capabilities', 'tools', 2, 'risk']), ed.text).toBe('execute');
	expect(ed.strings(['capabilities', 'tools', 2, 'schema', 'required'])).toEqual(['q']);
	expect(ed.map(['capabilities', 'tools', 2, 'env'])).toEqual({ KEY: '${NEW_TOKEN}' });
	expect(onerrors.mock.lastCall?.[0]).toEqual([]);
	expect(ed.text).toContain('- read  # Reading stays included.');
	expect(ed.text).toContain('# Keep this heading.');
	expect(ed.text).toContain('prompt: !include instructions.md');
});

it('edits MCP and all subagent fields with exact manifest paths', () => {
	const ed = setup('tools');
	click('MCP');
	click('+ Add MCP server');
	input('capabilities.mcp[0].name', 'docs');
	input('capabilities.mcp[0].command', 'node');
	input('capabilities.mcp[0].url', 'https://example.test/mcp');
	input('capabilities.mcp[0].args', '["server.js","--stdio"]');
	click('Subagents');
	click('+ Add subagent');
	input('capabilities.subagents[0].name', 'research');
	input('capabilities.subagents[0].description', 'Find sources');
	input('capabilities.subagents[0].model', 'large');
	input('capabilities.subagents[0].tools', '["read","grep"]');
	input('capabilities.subagents[0].prompt', 'Search first.');
	expect(ed.map(['capabilities', 'mcp', 0])).toEqual({ name: 'docs', command: 'node', url: 'https://example.test/mcp', args: ['server.js', '--stdio'] });
	expect(ed.map(['capabilities', 'subagents', 0])).toEqual({ name: 'research', description: 'Find sources', model: 'large', tools: ['read', 'grep'], prompt: 'Search first.' });
	click('Remove subagent');
	click('MCP');
	click('Remove server');
	expect(ed.list(['capabilities', 'subagents'])).toEqual([]);
	expect(ed.list(['capabilities', 'mcp'])).toEqual([]);
});

it('adds deny-only block rules, redaction and host lists', () => {
	const ed = setup('policy');
	click('Blocked calls');
	click('+ Block a call');
	input('policy.rules[0].tool', 'bash');
	input('policy.rules[0].label', 'No sudo');
	input('policy.rules[0].pattern', '^sudo');
	expect(ed.str(['policy', 'rules', 0, 'action'])).toBe('deny');
	expect(document.querySelector('[data-path="policy.rules[0].action"]')).toBeNull();
	click('Masked output');
	click('+ Mask a pattern');
	input('policy.redact[0].pattern', 'token=\\S+');
	input('policy.redact[0].with', 'token=[hidden]');
	click('Network');
	input('policy.network.allow', 'docs.example.test\napi.example.test\n');
	input('policy.network.deny', '*.internal');
	expect(ed.strings(['policy', 'network', 'allow'])).toEqual(['docs.example.test', 'api.example.test']);
	expect(ed.str(['policy', 'redact', 0, 'with'])).toBe('token=[hidden]');
	expect(ed.text).toContain('# Keep this final note.');
});

it('edits context strategies, all seven limits, every trigger type and scheduler and journal fields', () => {
	const ed = setup('context');
	input('context.compaction.mode', 'summary');
	input('context.compaction.keep_turns', '3');
	input('context.compaction.mode', 'trim');
	input('context.compaction.keep', '12');
	input('context.window', '8192');
	expect(ed.num(['context', 'compaction', 'keep_turns'])).toBe(3);
	for (const [name, value] of Object.entries({ max_tokens: '8000', max_output_tokens: '1000', max_tool_calls: '20', max_iterations: '10', max_duration: '5m', tool_timeout: '30s', subagent_depth: '2' })) input(`limits.${name}`, value);
	expect(Object.keys(ed.map(['limits']))).toHaveLength(7);
	click('+ Add trigger');
	input('run.triggers[0].name', 'scheduled');
	input('run.triggers[0].every', '15m');
	input('run.triggers[0].type', 'daily');
	input('run.triggers[0].at', '08:15');
	input('run.triggers[0].type', 'webhook');
	input('run.triggers[0].addr', ':9090');
	input('run.triggers[0].path', '/jobs');
	input('run.triggers[0].token', '${HOOK_TOKEN}');
	input('run.triggers[0].memory', 'persistent');
	input('run.triggers[0].prompt', 'Report changes.');
	for (const [path, value] of [
		['run.scheduler.path', './queue'], ['run.scheduler.concurrency', '2'], ['run.scheduler.max_pending', '16'],
		['run.scheduler.retry.max_attempts', '4'], ['run.scheduler.retry.backoff', '45s'],
		['observability.journal.backend', 'sqlite'], ['observability.journal.path', './runs.db'], ['observability.journal.retention', '100']
	]) input(path, value);
	expect(ed.map(['run', 'triggers', 0])).toMatchObject({ type: 'webhook', name: 'scheduled', every: '15m', at: '08:15', addr: ':9090', path: '/jobs', token: '${HOOK_TOKEN}', memory: 'persistent', prompt: 'Report changes.' });
	expect(ed.map(['run', 'scheduler'])).toEqual({ path: './queue', concurrency: 2, max_pending: 16, retry: { max_attempts: 4, backoff: '45s' } });
	expect(ed.map(['observability', 'journal'])).toEqual({ backend: 'sqlite', path: './runs.db', retention: 100 });
	expect(ed.text).toContain('prompt: !include instructions.md');
});

it('retains the right invalid draft when removing an earlier custom entry', () => {
	const ed = setup('tools');
	click('Custom');
	click('+ Add custom tool');
	input('capabilities.tools[3].env', '{bad');
	input('capabilities.tools[2].env', '{also bad');
	click('Remove tool');
	expect(ed.list(['capabilities', 'tools'])).toHaveLength(3);
	expect(control('capabilities.tools[2].env').value).toBe('{bad');
	expect(onerrors.mock.lastCall?.[0]).toHaveLength(1);
	click('Remove tool');
	expect(onerrors.mock.lastCall?.[0]).toEqual([]);
});

it('does not replace a schema containing !include with serialized marker objects', () => {
	const ed = setup('output', template + 'output:\n  schema:\n    type: object\n    properties:\n      label:\n        type: string\n        enum: !include labels.txt\n');
	expect((control('output.schema') as HTMLTextAreaElement).readOnly).toBe(true);
	expect(document.querySelector('[data-section="output"]')?.textContent).toContain('labels.txt');
	input('identity.name', 'included-schema');
	expect(ed.text).toContain('enum: !include labels.txt');
});

it('patches nested JSON fields without losing schema comments or untouched fields', () => {
	const ed = setup('output', template + `output:
  schema:
    type: object
    properties:
      answer:
        type: string  # Keep the answer type.
        description: Old description
      score:
        type: number  # Keep this sibling.
    required: [answer]  # Keep the requirement.
`);
	input('output.schema', JSON.stringify({
		type: 'object',
		properties: { answer: { type: 'string', description: 'New description' }, score: { type: 'number' } },
		required: ['answer']
	}));
	expect(ed.str(['output', 'schema', 'properties', 'answer', 'description'])).toBe('New description');
	expect(ed.text).toContain('type: string  # Keep the answer type.');
	expect(ed.text).toContain('type: number  # Keep this sibling.');
	expect(ed.text).toContain('required: [answer]  # Keep the requirement.');
});

it('keeps empty JSON string values and new interpolation values intact', () => {
	const ed = setup('tools');
	input('capabilities.tools[2].env', '{"KEY":"","NEW":"${NEW_TOKEN}"}');
	input('capabilities.tools[2].args', '["--flag","","a,b","${ARG}"]');
	expect(ed.parses).toBe(true);
	expect(ed.map(['capabilities', 'tools', 2, 'env'])).toEqual({ KEY: '', NEW: '${NEW_TOKEN}' });
	expect(ed.strings(['capabilities', 'tools', 2, 'args'])).toEqual(['--flag', '', 'a,b', '${ARG}']);
	expect(onerrors.mock.lastCall?.[0]).toEqual([]);
});

it('updates form drafts after external YAML edits without discarding unrelated invalid drafts', () => {
	const ed = setup('output');
	input('output.schema', '{"type":');
	input('capabilities.tools[2].env', '{bad env');
	ed.set(['output', 'schema'], { type: 'object', properties: { answer: { type: 'string' } } });
	flushSync();
	expect(JSON.parse(control('output.schema').value)).toEqual({ type: 'object', properties: { answer: { type: 'string' } } });
	expect(control('capabilities.tools[2].env').value).toBe('{bad env');
	expect(onerrors.mock.lastCall?.[0]).toHaveLength(1);
	expect(onerrors.mock.lastCall?.[0][0]).toContain('capabilities.tools[2].env');
});

it('switches response formats only on demand and clears discarded JSON errors', () => {
	const ed = setup('output');
	const format = document.getElementById('f-output-format') as HTMLSelectElement;
	expect(format.value).toBe('text');
	expect(ed.has(['output'])).toBe(false);
	format.value = 'json';
	format.dispatchEvent(new Event('change', { bubbles: true }));
	flushSync();
	expect(ed.map(['output', 'schema'])).toEqual({ type: 'object', properties: {} });
	input('output.schema', '{invalid');
	format.value = 'text';
	format.dispatchEvent(new Event('change', { bubbles: true }));
	flushSync();
	expect(ed.has(['output'])).toBe(false);
	expect(onerrors.mock.lastCall?.[0]).toEqual([]);
});

it('reports model discovery failures explicitly and offers retry', async () => {
	vi.mocked(api.models).mockRejectedValueOnce(new Error('Model service offline'));
	setup('agent');
	await vi.waitFor(() => {
		flushSync();
		expect(document.body.textContent).toContain('Models: Model service offline');
	});
	expect(onerrors.mock.lastCall?.[0]).toContain('Models: Model service offline');
	click('Retry');
	await vi.waitFor(() => {
		flushSync();
		expect(document.body.textContent).not.toContain('Model service offline');
		expect(document.querySelectorAll('#setup-models option')).toHaveLength(2);
	});
});

it('reports provider discovery failures instead of silently showing an empty list', async () => {
	app.providersLoaded = false;
	app.providers = [];
	vi.spyOn(api, 'providers').mockRejectedValueOnce(new Error('Provider service offline'));
	setup('agent');
	await vi.waitFor(() => {
		flushSync();
		expect(document.body.textContent).toContain('Providers: Provider service offline');
	});
	expect(onerrors.mock.lastCall?.[0]).toContain('Providers: Provider service offline');
});

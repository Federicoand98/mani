<script lang="ts">
	import { untrack } from 'svelte';
	import { isMap, isNode, isSeq, visit } from 'yaml';
	import { api } from '../api/client.ts';
	import { BUILTIN_TOOLS, Include, Manifest, type Path } from '../agents/doc.ts';
	import type { ManifestState } from '../agents/editor.svelte.ts';
	import { TOOL_INFO } from '../agents/tools.ts';
	import { app } from '../state.svelte.ts';
	import MarkdownEditor from './MarkdownEditor.svelte';
	import NativeFolderInput from './NativeFolderInput.svelte';

	type Section = 'agent' | 'tools' | 'policy' | 'context' | 'output' | 'limits' | 'run' | 'records';
	type Kind = 'text' | 'number' | 'textarea' | 'lines' | 'schema' | 'env' | 'array' | 'password';
	let { ed, section, onerrors }: {
		ed: ManifestState;
		section: Section;
		onerrors?: (errors: string[]) => void;
	} = $props();

	let toolsTab = $state('builtin');
	let policyTab = $state('access');
	let drafts = $state<Record<string, string>>({});
	let sources = $state<Record<string, string>>({});
	let errors = $state<Record<string, string>>({});
	let overrideName = $state('');
	let overrideError = $state('');
	let providerError = $state('');
	let modelError = $state('');
	let models = $state<string[]>([]);
	let providerRetry = $state(0);
	let modelRetry = $state(0);
	const provider = $derived(ed.str(['identity', 'provider']));
	const verdicts = $derived(ed.map(['policy', 'tools']));
	const fallback = $derived(String(verdicts.default ?? 'allow'));
	const structuredOutput = $derived(ed.has(['output', 'schema']) || !!drafts[key(['output', 'schema'])]?.trim());
	const knownTools = $derived([...new Set(ed.toolNames().filter((name) => name && name !== 'default'))]);
	const overrides = $derived(Object.keys(verdicts).filter((name) => name !== 'default' && !knownTools.includes(name)));
	const customTools = $derived(ed.list(['capabilities', 'tools']).flatMap((value, index) => {
		const name = typeof value === 'string' ? value : String((value as Record<string, unknown> | null)?.name ?? '');
		return !BUILTIN_TOOLS.includes(name) || (value && typeof value === 'object' && Object.keys(value).length > 1)
			? [{ index, name }] : [];
	}));
	const toolTabs = [['builtin', 'Built-in'], ['custom', 'Custom'], ['mcp', 'MCP'], ['subagents', 'Subagents']];
	const policyTabs = [['access', 'Tool access'], ['rules', 'Blocked calls'], ['redact', 'Masked output'], ['network', 'Network']];
	const choices = [['allow', 'Allow'], ['ask', 'Ask'], ['deny', 'Deny']];
	function key(path: Path) { return JSON.stringify(path); }
	const pathText = (path: Path) => path.map((part, i) => typeof part === 'number' ? `[${part}]` : `${i ? '.' : ''}${part}`).join('');
	const fieldId = (path: Path) => `setup-${path.map((part) => encodeURIComponent(String(part))).join('-')}`;
	const jsonKind = (kind: Kind) => kind === 'schema' || kind === 'env' || kind === 'array';
	const canonical = (path: Path) => {
		const node = ed.m.doc.getIn(path, true);
		return ed.has(path) ? JSON.stringify(isNode(node) ? node.toJSON() : node, (_name, value) => value instanceof Include ? `!include ${value.file}` : value, 2) ?? '' : '';
	};

	$effect(() => {
		void ed;
		untrack(() => { drafts = {}; sources = {}; errors = {}; overrideName = ''; overrideError = ''; });
	});

	$effect(() => {
		void ed.rev;
		for (const id of Object.keys(drafts)) {
			if (sources[id] !== canonical(JSON.parse(id))) {
				delete drafts[id];
				delete sources[id];
				delete errors[id];
			}
		}
	});

	$effect(() => {
		const notify = onerrors;
		const all = [...Object.values(errors), overrideError, providerError, modelError].filter(Boolean);
		untrack(() => notify?.(all));
	});

	$effect(() => {
		void providerRetry;
		if (app.providersLoaded) return;
		let active = true;
		providerError = '';
		api.providers().then((result) => {
			if (!active) return;
			app.providers = result.providers;
			app.providersLoaded = true;
		}, (error: unknown) => {
			if (active) providerError = `Providers: ${error instanceof Error ? error.message : String(error)}`;
		});
		return () => { active = false; };
	});

	$effect(() => {
		const name = provider;
		void modelRetry;
		const connected = app.providers.some((p) => p.name === name && p.enabled);
		let active = true;
		models = [];
		modelError = '';
		if (connected) api.models(name).then((result) => {
			if (active) models = result.models;
		}, (error: unknown) => {
			if (active) modelError = `Models: ${error instanceof Error ? error.message : String(error)}`;
		});
		return () => { active = false; };
	});

	function includes(path: Path): string[] {
		void ed.rev;
		const node = ed.m.doc.getIn(path, true);
		const files: string[] = [];
		if (isNode(node)) visit(node, { Scalar(_key, scalar) {
			if (scalar.value instanceof Include) files.push(scalar.value.file);
		} });
		return files;
	}

	function patch(m: Manifest, path: Path, value: unknown) {
		const node = m.doc.getIn(path, true);
		if (value && typeof value === 'object' && !Array.isArray(value) && isMap(node) && !Object.values(value).includes('')) {
			const next = value as Record<string, unknown>;
			for (const name of Object.keys(m.map(path))) if (!Object.hasOwn(next, name)) m.delete([...path, name]);
			for (const [name, child] of Object.entries(next)) patch(m, [...path, name], child);
		} else if (Array.isArray(value) && isSeq(node) && value.length && !value.includes('')) {
			for (let i = m.list(path).length - 1; i >= value.length; i--) m.removeAt(path, i);
			for (let i = 0; i < value.length; i++) {
				if (i >= m.list(path).length) m.push(path, value[i]);
				else patch(m, [...path, i], value[i]);
			}
		} else if (canonicalFor(m, path) !== JSON.stringify(value)) m.set(path, value);
	}

	function canonicalFor(m: Manifest, path: Path) {
		const node = m.doc.getIn(path, true);
		return JSON.stringify(isNode(node) ? node.toJSON() : node);
	}

	function set(path: Path, value: unknown) {
		try {
			const apply = (m: Manifest) => {
				// A scalar tool reference becomes a mapping only when its fields are edited.
				if (path[0] === 'capabilities' && path[1] === 'tools' && typeof path[2] === 'number' && path.length > 3) {
					const parent = path.slice(0, 3);
					if (typeof m.get(parent) === 'string') m.set(parent, { name: m.str(parent) });
				}
				patch(m, path, value);
			};
			const candidate = new Manifest(ed.text);
			apply(candidate);
			if (!candidate.ok) throw new Error(candidate.errors[0]);
			ed.edit(apply);
			delete errors[key(path)];
		} catch (error) {
			errors[key(path)] = `${pathText(path)}: ${error instanceof Error ? error.message : String(error)}`;
		}
	}

	function input(path: Path, value: string, kind: Kind, invalid = false) {
		const id = key(path);
		if (jsonKind(kind) || kind === 'lines' || kind === 'number') {
			drafts[id] = value;
			sources[id] = canonical(path);
		}
		try {
			if (invalid) throw new Error('Enter a non-negative whole number.');
			if (jsonKind(kind)) {
				if (!value.trim()) {
					set(path, null);
					sources[id] = canonical(path);
					return;
				}
				const parsed: unknown = JSON.parse(value);
				if (kind === 'array') {
					if (!Array.isArray(parsed) || parsed.some((v) => typeof v !== 'string')) throw new Error('Use a JSON array of strings.');
				} else {
					if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) throw new Error('Use a JSON object.');
					if (kind === 'env' && Object.values(parsed).some((v) => typeof v !== 'string')) throw new Error('Environment values must be strings.');
					if (kind === 'schema' && (parsed as Record<string, unknown>).type !== 'object') throw new Error('Schema type must be "object".');
				}
				set(path, parsed);
			} else if (kind === 'number') set(path, value === '' ? null : Number(value));
			else if (kind === 'lines') set(path, value.split('\n').map((v) => v.trim()).filter(Boolean));
			else set(path, value);
			if (Object.hasOwn(drafts, id)) sources[id] = canonical(path);
		} catch (error) {
			errors[id] = `${pathText(path)}: ${error instanceof Error ? error.message : String(error)}`;
		}
	}

	function display(path: Path, kind: Kind): string | number {
		void ed.rev;
		if (drafts[key(path)] !== undefined) return drafts[key(path)];
		if (jsonKind(kind)) return canonical(path);
		if (kind === 'number') return ed.num(path);
		if (kind === 'lines') return ed.strings(path).join('\n');
		if (path[0] === 'capabilities' && path[1] === 'tools' && path[3] === 'name' && typeof ed.m.get(path.slice(0, 3)) === 'string') return ed.str(path.slice(0, 3));
		return ed.str(path);
	}

	function add(path: Path, value: unknown) {
		ed.edit((m) => m.push(path, value));
	}

	function remove(path: Path, index: number) {
		ed.edit((m) => m.removeAt(path, index));
		// Indexed drafts follow their entry when an earlier row is removed.
		const shift = <T,>(values: Record<string, T>) => Object.fromEntries(Object.entries(values).flatMap(([id, value]) => {
			const p: Path = JSON.parse(id);
			if (!path.every((part, i) => p[i] === part) || typeof p[path.length] !== 'number') return [[id, value]];
			const n = p[path.length] as number;
			if (n === index) return [];
			if (n > index) p[path.length] = n - 1;
			return [[key(p), value]];
		}));
		drafts = shift(drafts);
		sources = shift(sources);
		errors = shift(errors);
	}

	function setBuiltin(name: string, included: boolean) {
		if (included) ed.edit((m) => m.setTool(name, true));
		else remove(['capabilities', 'tools'], ed.toolNames().indexOf(name));
	}

	function outputFormat(structured: boolean) {
		const path = ['output', 'schema'];
		if (structured) set(path, { type: 'object', properties: {} });
		else {
			set(path, null);
			delete drafts[key(path)];
			delete sources[key(path)];
			delete errors[key(path)];
		}
	}

	function addOverride() {
		const name = overrideName.trim();
		if (!name || name === 'default' || Object.hasOwn(verdicts, name) || knownTools.includes(name)) {
			overrideError = 'Choose a unique tool name.';
			return;
		}
		set(['policy', 'tools', name], fallback);
		overrideName = '';
		overrideError = '';
	}

	function tabKey(event: KeyboardEvent, group: 'tools' | 'policy') {
		if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
		event.preventDefault();
		const tabs = group === 'tools' ? toolTabs : policyTabs;
		const current = group === 'tools' ? toolsTab : policyTab;
		const index = tabs.findIndex(([id]) => id === current);
		const next = event.key === 'Home' ? 0 : event.key === 'End' ? tabs.length - 1 : (index + (event.key === 'ArrowRight' ? 1 : -1) + tabs.length) % tabs.length;
		if (group === 'tools') toolsTab = tabs[next][0]; else policyTab = tabs[next][0];
		document.getElementById(`setup-tab-${group}-${tabs[next][0]}`)?.focus();
	}
</script>

{#snippet field(path: Path, label: string, kind: Kind = 'text', placeholder = '', id = '')}
	{@const fid = id || fieldId(path)}
	{@const error = errors[key(path)]}
	{@const files = includes(path)}
	<div class="field" class:full={kind === 'textarea' || jsonKind(kind) || kind === 'lines'}>
		<label for={fid}>{label}</label>
		{#if kind === 'textarea' || jsonKind(kind) || kind === 'lines'}
			<textarea id={fid} class="input" class:mono={kind !== 'textarea'} rows={jsonKind(kind) ? 5 : 3}
				data-path={pathText(path)} value={display(path, kind)} {placeholder} readonly={files.length > 0}
				aria-invalid={!!error} aria-describedby={error ? `${fid}-error` : undefined} spellcheck={false}
				oninput={(e) => input(path, e.currentTarget.value, kind)}></textarea>
		{:else}
			<input id={fid} class="input" type={kind === 'number' ? 'number' : kind === 'password' ? 'password' : 'text'}
				min={kind === 'number' ? 0 : undefined} step={kind === 'number' ? 1 : undefined}
				data-path={pathText(path)} value={display(path, kind)} {placeholder} readonly={files.length > 0}
				aria-invalid={!!error} aria-describedby={error ? `${fid}-error` : undefined}
				oninput={(e) => input(path, e.currentTarget.value, kind, kind === 'number' && !e.currentTarget.validity.valid)} />
		{/if}
		{#if files.length}<span class="help">Included: {files.join(', ')} (edit in File).</span>{/if}
		{#if error}<span id={`${fid}-error`} class="field-error" role="alert">{error}</span>{/if}
	</div>
{/snippet}

{#snippet select(path: Path, label: string, options: string[][], fallbackValue = '', id = '')}
	{@const fid = id || fieldId(path)}
	{@const current = ed.str(path) || fallbackValue}
	<div class="field">
		<label for={fid}>{label}</label>
		<select id={fid} class="input" data-path={pathText(path)} value={current} onchange={(e) => set(path, e.currentTarget.value)}>
			{#if !options.some(([value]) => value === current)}<option value={current}>{current || 'Runtime default'}</option>{/if}
			{#each options as [value, text] (value)}<option {value}>{text}</option>{/each}
		</select>
		{#if errors[key(path)]}<span class="field-error" role="alert">{errors[key(path)]}</span>{/if}
	</div>
{/snippet}

{#snippet toggle(path: Path, label: string, fallbackValue = false)}
	<label class="toggle">
		<input type="checkbox" data-path={pathText(path)} checked={ed.bool(path, fallbackValue)} onchange={(e) => set(path, e.currentTarget.checked)} />
		{label}
	</label>
{/snippet}

{#snippet access(name: string, label: string)}
	<div class="seg small" role="group" aria-label={label} data-path={pathText(['policy', 'tools', name])}>
		{#each choices as [value, text] (value)}
			<button type="button" class={value} aria-pressed={(verdicts[name] ?? fallback) === value} onclick={() => set(['policy', 'tools', name], value)}>{text}</button>
		{/each}
	</div>
{/snippet}

<div class="agent-setup">
	<section class="pane" data-section="agent" aria-label="Agent" hidden={section !== 'agent'}>
		<div class="fields">
			<div class="identity-wide">{@render field(['identity', 'name'], 'Name', 'text', '', 'f-name')}</div>
			<div class="identity-wide">{@render field(['identity', 'description'], 'Description', 'text', '', 'f-desc')}</div>
			{@render select(['identity', 'provider'], 'Provider', app.providers.map((p) => [p.name, `${p.label}${p.enabled ? '' : ' (not connected)'}`]), '', 'f-provider')}
			<div class="field">
				<label for="f-model">Model</label>
				<input id="f-model" class="input" list="setup-models" data-path="identity.model" value={ed.str(['identity', 'model'])} oninput={(e) => set(['identity', 'model'], e.currentTarget.value)} />
				<datalist id="setup-models">{#each models as model (model)}<option value={model}></option>{/each}</datalist>
			</div>
		</div>
		{#if providerError}<div class="field-error" role="alert">{providerError} <button type="button" class="btn ghost small" onclick={() => providerRetry++}>Retry</button></div>{/if}
		{#if modelError}<div class="field-error" role="alert">{modelError} <button type="button" class="btn ghost small" onclick={() => modelRetry++}>Retry</button></div>{/if}
		<div class="field" data-path="identity.prompt">
			<label for="f-prompt">Instructions</label>
			{#if ed.includeOf(['identity', 'prompt'])}
				<span class="help">Included: {ed.includeOf(['identity', 'prompt'])} (edit in File).</span>
			{:else}
				<MarkdownEditor id="f-prompt" value={ed.str(['identity', 'prompt'])} onchange={(value) => set(['identity', 'prompt'], value)} />
			{/if}
		</div>
		<div data-path="capabilities.workspace">
			<NativeFolderInput id="f-ws" value={ed.str(['capabilities', 'workspace'])} onchange={(value) => set(['capabilities', 'workspace'], value)} />
		</div>
	</section>

	<section class="pane" data-section="tools" aria-label="Tools" hidden={section !== 'tools'}>
		<div class="subtabs" role="tablist" aria-label="Tool source">
			{#each toolTabs as [id, label] (id)}
				<button type="button" id="setup-tab-tools-{id}" role="tab" aria-controls="setup-tools-{id}" aria-selected={toolsTab === id} tabindex={toolsTab === id ? 0 : -1} onclick={() => toolsTab = id} onkeydown={(e) => tabKey(e, 'tools')}>{label}</button>
			{/each}
		</div>
		<div id="setup-tools-builtin" class="tool-list" role="tabpanel" aria-labelledby="setup-tab-tools-builtin" hidden={toolsTab !== 'builtin'}>
			{#each BUILTIN_TOOLS as name (name)}
				<div class="tool-row">
					<label class="toggle"><input type="checkbox" data-tool={name} checked={ed.toolNames().includes(name)} onchange={(e) => setBuiltin(name, e.currentTarget.checked)} /><span class="mono">{name}</span></label>
					<span class="tool-desc">{TOOL_INFO[name].about}</span>
				</div>
			{/each}
		</div>
		<div id="setup-tools-custom" class="pane" role="tabpanel" aria-labelledby="setup-tab-tools-custom" hidden={toolsTab !== 'custom'}>
			{#each customTools as { index, name } (index)}
				{@const p = ['capabilities', 'tools', index]}
				<div class="entry" data-path={pathText(p)}>
					<div class="entry-head"><strong>{name || 'Custom tool'}</strong><button type="button" class="btn ghost small remove" onclick={() => remove(['capabilities', 'tools'], index)}>Remove tool</button></div>
					<div class="fields">
						{@render field([...p, 'name'], 'Name')}
						{@render select([...p, 'risk'], 'Risk', [['', 'Runtime default'], ['none', 'None'], ['network', 'Network'], ['write', 'Write'], ['execute', 'Execute']])}
						{@render field([...p, 'description'], 'Description')}
						{@render field([...p, 'command'], 'Command')}
						{@render field([...p, 'args'], 'Arguments (JSON)', 'array', '["--flag", "value"]')}
						{@render field([...p, 'env'], 'Environment (JSON)', 'env', '{"KEY": "${VALUE}"}')}
						{@render field([...p, 'schema'], 'Input schema (JSON)', 'schema', '{"type":"object","properties":{}}')}
					</div>
				</div>
			{:else}<span class="empty">No custom tools.</span>{/each}
			<button type="button" class="btn secondary small add" onclick={() => add(['capabilities', 'tools'], { name: '', command: '' })}>+ Add custom tool</button>
		</div>
		<div id="setup-tools-mcp" class="pane" role="tabpanel" aria-labelledby="setup-tab-tools-mcp" hidden={toolsTab !== 'mcp'}>
			{#each ed.list(['capabilities', 'mcp']) as _, index (index)}
				{@const p = ['capabilities', 'mcp', index]}
				<div class="entry" data-path={pathText(p)}>
					<div class="entry-head"><strong>{ed.str([...p, 'name']) || 'MCP server'}</strong><button type="button" class="btn ghost small remove" onclick={() => remove(['capabilities', 'mcp'], index)}>Remove server</button></div>
					<div class="fields">
						{@render field([...p, 'name'], 'Name')}
						{@render field([...p, 'command'], 'Command')}
						{@render field([...p, 'url'], 'URL', 'text', 'https://')}
						{@render field([...p, 'args'], 'Arguments (JSON)', 'array', '[]')}
					</div>
				</div>
			{:else}<span class="empty">No MCP servers.</span>{/each}
			<button type="button" class="btn secondary small add" onclick={() => add(['capabilities', 'mcp'], { name: '' })}>+ Add MCP server</button>
		</div>
		<div id="setup-tools-subagents" class="pane" role="tabpanel" aria-labelledby="setup-tab-tools-subagents" hidden={toolsTab !== 'subagents'}>
			{#each ed.list(['capabilities', 'subagents']) as _, index (index)}
				{@const p = ['capabilities', 'subagents', index]}
				<div class="entry" data-path={pathText(p)}>
					<div class="entry-head"><strong>{ed.str([...p, 'name']) || 'Subagent'}</strong><button type="button" class="btn ghost small remove" onclick={() => remove(['capabilities', 'subagents'], index)}>Remove subagent</button></div>
					<div class="fields">
						{@render field([...p, 'name'], 'Name')}
						{@render field([...p, 'description'], 'Description')}
						{@render field([...p, 'model'], 'Model', 'text', 'Inherit agent model')}
						{@render field([...p, 'tools'], 'Tools (JSON)', 'array', '["read", "grep"]')}
						{@render field([...p, 'prompt'], 'Instructions', 'textarea')}
					</div>
				</div>
			{:else}<span class="empty">No subagents.</span>{/each}
			<button type="button" class="btn secondary small add" onclick={() => add(['capabilities', 'subagents'], { name: '', description: '', prompt: '' })}>+ Add subagent</button>
		</div>
	</section>

	<section class="pane" data-section="policy" aria-label="Policy" hidden={section !== 'policy'}>
		<div class="subtabs" role="tablist" aria-label="Policy type">
			{#each policyTabs as [id, label] (id)}
				<button type="button" id="setup-tab-policy-{id}" role="tab" aria-controls="setup-policy-{id}" aria-selected={policyTab === id} tabindex={policyTab === id ? 0 : -1} onclick={() => policyTab = id} onkeydown={(e) => tabKey(e, 'policy')}>{label}</button>
			{/each}
		</div>
		<div id="setup-policy-access" class="pane" role="tabpanel" aria-labelledby="setup-tab-policy-access" hidden={policyTab !== 'access'}>
			<div class="policy-default"><span>Other tools</span>{@render access('default', 'Other tools access')}</div>
			<div class="tool-list">
				{#each [...knownTools, ...overrides] as name (name)}
					<div class="tool-row">
						<div class="tool-access-name"><span class="mono">{name}</span>{#if !Object.hasOwn(verdicts, name)}<small>Default</small>{/if}</div>
						{@render access(name, `Access for ${name}`)}
						{#if Object.hasOwn(verdicts, name)}<button type="button" class="btn ghost small" aria-label={knownTools.includes(name) ? `Use default for ${name}` : `Remove override ${name}`} onclick={() => set(['policy', 'tools', name], null)}>{knownTools.includes(name) ? 'Use default' : 'Remove'}</button>{/if}
					</div>
				{/each}
			</div>
			<div class="override-add">
				<label class="field"><span>Tool name</span><input class="input mono" aria-invalid={!!overrideError} placeholder="MCP tool name" value={overrideName} oninput={(e) => { overrideName = e.currentTarget.value; overrideError = ''; }} onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); addOverride(); } }} /></label>
				<button type="button" class="btn secondary small" onclick={addOverride}>+ Tool override</button>
			</div>
			{#if overrideError}<span class="field-error" role="alert">{overrideError}</span>{/if}
		</div>
		<div id="setup-policy-rules" class="pane" role="tabpanel" aria-labelledby="setup-tab-policy-rules" hidden={policyTab !== 'rules'}>
			{#each ed.list(['policy', 'rules']) as _, index (index)}
				{@const p = ['policy', 'rules', index]}
				<div class="entry" data-path={pathText(p)}>
					<div class="entry-head"><strong>Blocked call {index + 1}</strong><button type="button" class="btn ghost small remove" onclick={() => remove(['policy', 'rules'], index)}>Remove rule</button></div>
					<div class="fields">
						{@render field([...p, 'tool'], 'Tool')}
						{@render field([...p, 'label'], 'Rule name')}
						{@render field([...p, 'pattern'], 'Argument pattern (regex)', 'textarea', 'rm\\s+-rf')}
					</div>
				</div>
			{:else}<span class="empty">No blocked argument patterns.</span>{/each}
			<button type="button" class="btn secondary small add" onclick={() => add(['policy', 'rules'], { tool: 'bash', label: '', pattern: '', action: 'deny' })}>+ Block a call</button>
		</div>
		<div id="setup-policy-redact" class="pane" role="tabpanel" aria-labelledby="setup-tab-policy-redact" hidden={policyTab !== 'redact'}>
			{#each ed.list(['policy', 'redact']) as _, index (index)}
				{@const p = ['policy', 'redact', index]}
				<div class="entry" data-path={pathText(p)}>
					<div class="entry-head"><strong>Output mask {index + 1}</strong><button type="button" class="btn ghost small remove" onclick={() => remove(['policy', 'redact'], index)}>Remove mask</button></div>
					{@render field([...p, 'pattern'], 'Pattern (regex)')}
					{@render field([...p, 'with'], 'Replace with')}
				</div>
			{:else}<span class="empty">No output masks.</span>{/each}
			<button type="button" class="btn secondary small add" onclick={() => add(['policy', 'redact'], { pattern: '', with: '[redacted]' })}>+ Mask a pattern</button>
		</div>
		<div id="setup-policy-network" class="pane" role="tabpanel" aria-labelledby="setup-tab-policy-network" hidden={policyTab !== 'network'}>
			{@render field(['policy', 'network', 'allow'], 'Allowed hosts', 'lines', 'All public hosts')}
			{@render field(['policy', 'network', 'deny'], 'Blocked hosts', 'lines', '*.internal')}
			<span class="help">Network tools only. Private addresses stay blocked.</span>
		</div>
	</section>

	<section class="pane" data-section="context" aria-label="Context" hidden={section !== 'context'}>
		{@render toggle(['context', 'inject'], 'Load AGENTS.md', true)}
		{@render toggle(['context', 'compaction', 'enabled'], 'Compact history', true)}
		<div class="fields">
			{@render select(['context', 'compaction', 'mode'], 'History strategy', [['summary', 'Summarize old turns'], ['trim', 'Trim old messages']], 'trim')}
			<div hidden={ed.str(['context', 'compaction', 'mode']) !== 'summary'}>{@render field(['context', 'compaction', 'keep_turns'], 'Recent turns to keep', 'number', '0')}</div>
			<div hidden={ed.str(['context', 'compaction', 'mode']) === 'summary'}>{@render field(['context', 'compaction', 'keep'], 'Messages to keep', 'number', '20')}</div>
			{@render field(['context', 'window'], 'Context ceiling (tokens)', 'number', 'Provider model limit', 'f-window')}
		</div>
	</section>

	<section class="pane" data-section="output" aria-label="Output" hidden={section !== 'output'}>
		<div class="field">
			<label for="f-output-format">Response format</label>
			<select id="f-output-format" class="input" value={structuredOutput ? 'json' : 'text'} onchange={(e) => outputFormat(e.currentTarget.value === 'json')}>
				<option value="text">Text</option><option value="json">Structured JSON</option>
			</select>
		</div>
		<div hidden={!structuredOutput}>{@render field(['output', 'schema'], 'Response schema (JSON)', 'schema')}</div>
	</section>

	<section class="pane" data-section="limits" aria-label="Limits" hidden={section !== 'limits'}>
		<div class="fields">
			{@render field(['limits', 'max_tokens'], 'Tokens per run', 'number', 'No limit', 'f-max_tokens')}
			{@render field(['limits', 'max_output_tokens'], 'Output tokens per call', 'number', 'Provider model limit', 'f-max_output_tokens')}
			{@render field(['limits', 'max_tool_calls'], 'Tool calls per run', 'number', 'No limit', 'f-max_tool_calls')}
			{@render field(['limits', 'max_iterations'], 'Model iterations', 'number', 'No limit', 'f-max_iterations')}
			{@render field(['limits', 'max_duration'], 'Run timeout', 'text', 'No limit', 'f-max_duration')}
			{@render field(['limits', 'tool_timeout'], 'Tool timeout', 'text', 'No limit', 'f-tool_timeout')}
			{@render field(['limits', 'subagent_depth'], 'Subagent depth', 'number', '5', 'f-subagent_depth')}
		</div>
	</section>

	<section class="pane" data-section="run" aria-label="Run" hidden={section !== 'run'}>
		<div class="subhead"><h2>Triggers</h2><button type="button" class="btn secondary small" onclick={() => add(['run', 'triggers'], { type: 'every', prompt: '' })}>+ Add trigger</button></div>
		{#each ed.list(['run', 'triggers']) as _, index (index)}
			{@const p = ['run', 'triggers', index]}
			{@const type = ed.str([...p, 'type'])}
			<div class="entry" data-path={pathText(p)}>
				<div class="entry-head"><strong>{ed.str([...p, 'name']) || `Trigger ${index + 1}`}</strong><button type="button" class="btn ghost small remove" onclick={() => remove(['run', 'triggers'], index)}>Remove trigger</button></div>
				<div class="fields">
					{@render select([...p, 'type'], 'Type', [['every', 'Interval'], ['daily', 'Daily'], ['webhook', 'Webhook']])}
					{@render field([...p, 'name'], 'Name')}
					<div hidden={type !== 'every'}>{@render field([...p, 'every'], 'Every', 'text', '30m')}</div>
					<div hidden={type !== 'daily'}>{@render field([...p, 'at'], 'At', 'text', '07:00')}</div>
					<div hidden={type !== 'webhook'}>{@render field([...p, 'addr'], 'Listen address', 'text', ':8080')}</div>
					<div hidden={type !== 'webhook'}>{@render field([...p, 'path'], 'Path', 'text', '/hook')}</div>
					<div hidden={type !== 'webhook'}>{@render field([...p, 'token'], 'Token', 'password', '${MANI_WEBHOOK_TOKEN}')}</div>
					{@render select([...p, 'memory'], 'Memory', [['', 'Fresh'], ['persistent', 'Persistent']])}
					{@render field([...p, 'prompt'], 'Task', 'textarea')}
				</div>
				{@render toggle([...p, 'catch_up'], 'Catch up a missed run')}
			</div>
		{:else}<span class="empty">On demand only.</span>{/each}
		{#if ed.list(['run', 'triggers']).length && (fallback === 'ask' || Object.values(verdicts).includes('ask'))}<span class="notice">Scheduled runs deny calls that require approval.</span>{/if}
		<h2>Task queue</h2>
		<div class="fields">
			{@render field(['run', 'scheduler', 'path'], 'Queue folder', 'text', 'In-memory')}
			{@render field(['run', 'scheduler', 'concurrency'], 'Concurrent tasks', 'number', '1')}
			{@render field(['run', 'scheduler', 'max_pending'], 'Pending task limit', 'number', '64')}
			{@render field(['run', 'scheduler', 'retry', 'max_attempts'], 'Retry attempts', 'number', '3')}
			{@render field(['run', 'scheduler', 'retry', 'backoff'], 'Retry backoff', 'text', '30s')}
		</div>
	</section>

	<section class="pane" data-section="records" aria-label="Records" hidden={section !== 'records'}>
		{@render toggle(['observability', 'tracing'], 'Trace model and tool calls', true)}
		{@render toggle(['observability', 'journal', 'enabled'], 'Record runs')}
		<div class="fields">
			{@render select(['observability', 'journal', 'backend'], 'Journal backend', [['jsonl', 'JSONL'], ['sqlite', 'SQLite']], 'jsonl')}
			{@render field(['observability', 'journal', 'retention'], 'Retention entries', 'number', 'All entries')}
			{@render field(['observability', 'journal', 'path'], 'Journal path', 'text', './runs')}
		</div>
	</section>
</div>

<style>
	[hidden] { display: none !important; }
	.agent-setup { min-width: 0; }
	.pane { display: flex; flex-direction: column; gap: 22px; min-width: 0; }
	.fields { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
	.field { min-width: 0; }
	.field.full { grid-column: 1 / -1; }
	.identity-wide { grid-column: 1 / -1; }
	.field > span:first-child { font-size: 14px; line-height: 20px; font-weight: 600; }
	textarea.input { height: auto; min-height: 80px; padding: 10px 12px; resize: vertical; }
	.subtabs { display: flex; gap: 18px; border-bottom: 1px solid var(--bd); overflow-x: auto; }
	.subtabs button { background: none; border: 0; border-bottom: 2px solid transparent; padding: 0 0 11px; font-size: 13px; font-weight: 600; color: var(--mu); white-space: nowrap; cursor: pointer; }
	.subtabs button[aria-selected="true"] { border-color: var(--pr); color: var(--pr); }
	.toggle { display: inline-flex; align-items: center; gap: 9px; font-size: 14px; cursor: pointer; }
	.toggle input { width: 17px; height: 17px; accent-color: var(--pr); margin: 0; }
	.tool-row { display: flex; align-items: center; gap: 12px; padding: 13px 0; border-bottom: 1px solid var(--bd); flex-wrap: wrap; }
	.tool-row:first-child { padding-top: 0; }
	.tool-row:last-child { border-bottom: 0; }
	.tool-row .toggle, .tool-access-name { flex: 1; min-width: 0; }
	.tool-desc { font-size: 13px; color: var(--mu); }
	.tool-access-name { display: flex; align-items: baseline; gap: 10px; flex-wrap: wrap; overflow-wrap: anywhere; }
	.tool-access-name small { color: var(--mu); font-size: 12px; }
	.policy-default { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding-bottom: 16px; border-bottom: 1px solid var(--bd); font-size: 14px; font-weight: 600; flex-wrap: wrap; }
	.seg { flex-shrink: 0; }
	.seg button { padding: 0 12px; }
	.entry { display: grid; gap: 12px; border: 1px solid var(--bd); border-radius: var(--r-card); padding: 16px; min-width: 0; }
	.entry-head, .subhead { display: flex; justify-content: space-between; align-items: center; gap: 10px; }
	.entry-head strong { font-size: 14px; overflow-wrap: anywhere; }
	.entry .fields { gap: 12px; }
	.remove, .field-error { color: var(--er); }
	.field-error { font-size: 13px; line-height: 20px; overflow-wrap: anywhere; }
	.empty { color: var(--mu); font-size: 14px; }
	.add { align-self: flex-start; }
	.override-add { display: flex; align-items: flex-end; gap: 12px; flex-wrap: wrap; }
	.override-add .field { flex: 1; }
	h2 { font-size: 16px; line-height: 24px; }
	.notice { font-size: 13px; color: var(--mu); }
	@media (max-width: 640px) {
		.fields { grid-template-columns: minmax(0, 1fr); }
		.tool-desc { display: none; }
	}
</style>

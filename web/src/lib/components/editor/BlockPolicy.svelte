<script lang="ts">
	import type { AgentEditor } from '../../agents/editor.svelte.ts';
	import { RISK_CLASS, RISK_LABEL, TOOL_INFO } from '../../agents/tools.ts';
	import Icon from '../Icon.svelte';

	let { ed }: { ed: AgentEditor } = $props();

	const tools = $derived(ed.toolNames().filter(Boolean));
	const verdicts = $derived(ed.map(['policy', 'tools']) as Record<string, string>);
	const rules = $derived(ed.list(['policy', 'rules']) as Record<string, string>[]);
	const redact = $derived(ed.list(['policy', 'redact']) as Record<string, string>[]);
	const triggered = $derived(ed.list(['run', 'triggers']).length > 0);
	const asking = $derived(tools.filter((t) => verdicts[t] === 'ask'));
	const choices = [
		['', 'Default'],
		['allow', 'Allow'],
		['ask', 'Ask'],
		['deny', 'Deny']
	] as const;

	const lines = (v: string) => v.split('\n').map((s) => s.trim()).filter(Boolean);
</script>

{#if triggered && asking.length}
	<div class="banner warn-banner">
		<Icon name="warn" />
		<span>This agent starts by itself. Nobody answers then, so "Ask" for {asking.join(', ')} works as "Deny" on those runs.</span>
	</div>
{/if}

<div class="stack" style="gap:4px">
	<span class="label">Tools <code>policy.tools</code></span>
	{#each tools as t (t)}
		{@const info = TOOL_INFO[t]}
		<div class="tool-row" data-path="policy.tools.{t}">
			<div class="grow"><b class="mono">{t}</b><span class="cap">{info?.about ?? 'external tool'}</span></div>
			{#if info}<span class="badge {RISK_CLASS[info.risk]}">{RISK_LABEL[info.risk]}</span>{/if}
			<div class="seg small" role="group" aria-label="Decision for {t}">
				{#each choices as [v, label] (v)}
					<button class={v} aria-pressed={(verdicts[t] ?? '') === v} onclick={() => ed.set(['policy', 'tools', t], v)}>{label}</button>
				{/each}
			</div>
		</div>
	{:else}
		<p class="sm mu">No tools: add some in the Tools block.</p>
	{/each}
	<p class="help">Default decides by risk: tools that only read run alone, the others ask.</p>
</div>

<div class="stack" style="gap:8px">
	<div class="subhead">
		<span class="label">Rules on commands <code>policy.rules</code></span>
		<button class="btn secondary small" onclick={() => ed.edit((m) => m.push(['policy', 'rules'], { tool: tools.includes('bash') ? 'bash' : (tools[0] ?? 'bash'), pattern: '', action: 'deny', label: '' }))}><Icon name="plus" size={14} />Add rule</button>
	</div>
	{#each rules as r, i (i)}
		<div class="rule" data-path="policy.rules[{i}]">
			<select class="input" aria-label="Tool" value={r.tool} onchange={(e) => ed.set(['policy', 'rules', i, 'tool'], e.currentTarget.value)}>
				{#each [...new Set([...tools, r.tool])] as t (t)}<option value={t}>{t}</option>{/each}
			</select>
			<input class="input mono" aria-label="Pattern (regular expression)" placeholder="rm\s+-rf" data-path="policy.rules[{i}].pattern" value={r.pattern ?? ''} oninput={(e) => ed.set(['policy', 'rules', i, 'pattern'], e.currentTarget.value === '' ? null : e.currentTarget.value)} />
			<input class="input" aria-label="Label" placeholder="what it stops" value={r.label ?? ''} oninput={(e) => ed.set(['policy', 'rules', i, 'label'], e.currentTarget.value)} />
			<button class="icon-btn" aria-label="Remove rule {i + 1}" onclick={() => ed.edit((m) => m.removeAt(['policy', 'rules'], i))}><Icon name="trash" size={16} /></button>
		</div>
	{/each}
	<p class="help">A call whose arguments match a rule is denied before it runs, even when the tool is allowed, and the journal records it.</p>
</div>

<div class="form-grid">
	<div class="field">
		<label for="f-allow">Hosts it may reach <code>policy.network.allow</code></label>
		<textarea id="f-allow" class="input mono" rows="3" placeholder="api.github.com" data-path="policy.network.allow" value={ed.strings(['policy', 'network', 'allow']).join('\n')} onchange={(e) => ed.set(['policy', 'network', 'allow'], lines(e.currentTarget.value))}></textarea>
	</div>
	<div class="field">
		<label for="f-deny">Hosts it may never reach <code>policy.network.deny</code></label>
		<textarea id="f-deny" class="input mono" rows="3" placeholder="*.internal" data-path="policy.network.deny" value={ed.strings(['policy', 'network', 'deny']).join('\n')} onchange={(e) => ed.set(['policy', 'network', 'deny'], lines(e.currentTarget.value))}></textarea>
	</div>
</div>
<p class="help" style="margin-top:-12px">One host per line. Private, local and loopback addresses are always refused.</p>

<div class="stack" style="gap:8px">
	<div class="subhead">
		<span class="label">Redaction <code>policy.redact</code></span>
		<button class="btn secondary small" onclick={() => ed.edit((m) => m.push(['policy', 'redact'], { pattern: '', with: '[redacted]' }))}><Icon name="plus" size={14} />Add</button>
	</div>
	{#each redact as r, i (i)}
		<div class="rule redact" data-path="policy.redact[{i}]">
			<input class="input mono" aria-label="Pattern" data-path="policy.redact[{i}].pattern" value={r.pattern ?? ''} oninput={(e) => ed.set(['policy', 'redact', i, 'pattern'], e.currentTarget.value === '' ? null : e.currentTarget.value)} />
			<input class="input" aria-label="Replace with" value={r.with ?? ''} oninput={(e) => ed.set(['policy', 'redact', i, 'with'], e.currentTarget.value)} />
			<button class="icon-btn" aria-label="Remove redaction {i + 1}" onclick={() => ed.edit((m) => m.removeAt(['policy', 'redact'], i))}><Icon name="trash" size={16} /></button>
		</div>
	{/each}
	<p class="help">Text matching a pattern is masked in what tools return, before the model sees it.</p>
</div>

<style>
	.warn-banner {
		background: var(--wat);
		border-color: var(--wa);
	}
	.tool-row {
		display: flex;
		flex-wrap: wrap;
		gap: 8px 12px;
		align-items: center;
		padding: 10px 0;
		border-top: 1px solid var(--bd);
	}
	.tool-row .grow {
		display: flex;
		flex-direction: column;
		flex: 1 1 200px;
		min-width: 0;
	}
	.rule {
		display: grid;
		grid-template-columns: 110px minmax(0, 1.4fr) minmax(0, 1fr) 36px;
		gap: 8px;
		align-items: center;
	}
	.rule.redact {
		grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr) 36px;
	}
	.icon-btn {
		width: 36px;
		height: 36px;
		border-radius: 8px;
		border: 1px solid var(--bd);
		background: var(--sf);
		display: inline-grid;
		place-items: center;
		cursor: pointer;
	}
	@media (max-width: 640px) {
		.rule,
		.rule.redact {
			grid-template-columns: minmax(0, 1fr) 36px;
		}
	}
</style>

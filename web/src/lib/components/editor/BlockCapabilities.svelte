<script lang="ts">
	import type { ManifestState as AgentEditor } from '../../agents/editor.svelte.ts';
	import { RISK_CLASS, RISK_LABEL, TOOL_INFO } from '../../agents/tools.ts';
	import { BUILTIN_TOOLS } from '../../agents/doc.ts';
	import FolderPicker from './FolderPicker.svelte';

	let { ed }: { ed: AgentEditor } = $props();

	let picking = $state(false);
	const names = $derived(ed.toolNames());
	const custom = $derived(ed.customTools());
	const mcp = $derived(ed.list(['capabilities', 'mcp']) as Record<string, unknown>[]);
	const subagents = $derived(ed.list(['capabilities', 'subagents']) as Record<string, unknown>[]);
</script>

<div class="field">
	<label for="f-ws">Working folder <code>capabilities.workspace</code></label>
	<div class="row" style="flex-wrap:nowrap">
		<input id="f-ws" class="input mono" data-path="capabilities.workspace" placeholder="." value={ed.str(['capabilities', 'workspace'])} oninput={(e) => ed.set(['capabilities', 'workspace'], e.currentTarget.value)} />
		<button type="button" class="btn secondary" onclick={() => (picking = true)}>Choose…</button>
	</div>
	<p class="help">Every file tool stays inside it. A relative path counts from the folder mani web runs on.</p>
</div>

<div class="stack" style="gap:8px" data-path="capabilities.tools">
	<span class="label">Built-in tools <code>capabilities.tools</code></span>
	<div class="tools">
		{#each BUILTIN_TOOLS as t (t)}
			{@const info = TOOL_INFO[t]}
			<label class="tool" class:on={names.includes(t)}>
				<input type="checkbox" checked={names.includes(t)} onchange={(e) => ed.edit((m) => m.setTool(t, e.currentTarget.checked))} />
				<span class="grow"><b class="mono">{t}</b><span class="cap">{info.about}</span></span>
				<span class="badge {RISK_CLASS[info.risk]}">{RISK_LABEL[info.risk]}</span>
			</label>
		{/each}
	</div>
</div>

{#if custom.length || mcp.length || subagents.length}
	<div class="stack" style="gap:8px">
		<span class="label">Also declared</span>
		<div class="readonly-list">
			{#each custom as t (t.name)}<div><b class="mono">{t.name}</b><span class="cap">external tool - {t.command}</span></div>{/each}
			{#each mcp as s, i (i)}<div><b class="mono">{s.name}</b><span class="cap">MCP server - {s.command ?? s.url}</span></div>{/each}
			{#each subagents as s, i (i)}<div><b class="mono">{s.name}</b><span class="cap">subagent - {s.description}</span></div>{/each}
		</div>
		<p class="help">External tools, MCP servers and subagents are edited in the YAML panel.</p>
	</div>
{/if}

<FolderPicker bind:open={picking} current={ed.str(['capabilities', 'workspace'])} onpick={(p) => ed.set(['capabilities', 'workspace'], p)} />

<style>
	.tools {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 8px;
	}
	.tool {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 10px 12px;
		border: 1px solid var(--bd);
		border-radius: 10px;
		cursor: pointer;
	}
	.tool.on {
		border-color: var(--pr);
		background: var(--prt);
	}
	.tool input {
		width: 18px;
		height: 18px;
		accent-color: var(--pr);
	}
	.tool .grow {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}
	.tool b {
		font-size: 14px;
	}
	@media (max-width: 640px) {
		.tools {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>

<script lang="ts">
	import type { AgentEditor } from '../../agents/editor.svelte.ts';

	let { ed, onyaml }: { ed: AgentEditor; onyaml: (path: string) => void } = $props();

	const structured = $derived(ed.has(['output', 'schema']));
	const fields = $derived(Object.entries(ed.map(['output', 'schema', 'properties'])) as [string, Record<string, unknown>][]);
	const required = $derived(ed.strings(['output', 'schema', 'required']));

	function toggle(on: boolean) {
		if (on) ed.set(['output', 'schema'], { type: 'object', properties: { answer: { type: 'string' } }, required: ['answer'] });
		else ed.set(['output'], null);
	}
</script>

<div class="seg" role="group" aria-label="Answer format" data-path="output.schema">
	<button aria-pressed={!structured} onclick={() => toggle(false)}>Free text</button>
	<button aria-pressed={structured} onclick={() => toggle(true)}>JSON with a schema</button>
</div>
{#if structured}
	<p class="sm mu">The agent answers through the <code>respond</code> tool; an answer that breaks the schema is sent back for another try.</p>
	<div class="readonly-list">
		{#each fields as [name, p] (name)}
			<div>
				<b class="mono">{name}</b>
				<span class="badge muted">{p.type ?? 'any'}</span>
				{#if Array.isArray(p.enum)}<span class="cap">one of {(p.enum as unknown[]).join(', ')}</span>{/if}
				{#if required.includes(name)}<span class="cap">required</span>{/if}
			</div>
		{:else}
			<div><span class="cap">No fields yet.</span></div>
		{/each}
	</div>
	<div class="row">
		<button class="btn secondary small" onclick={() => onyaml('output.schema')}>Edit the schema in YAML</button>
		<span class="help">A long list of values can live in a file: <code>enum: !include products.txt</code>.</span>
	</div>
{:else}
	<p class="sm mu">The agent answers in words. Choose a schema when a program or a flow reads the answer.</p>
{/if}

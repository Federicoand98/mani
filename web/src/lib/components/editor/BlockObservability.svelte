<script lang="ts">
	import type { ManifestState as AgentEditor } from '../../agents/editor.svelte.ts';

	let { ed }: { ed: AgentEditor } = $props();
	const on = $derived(ed.bool(['observability', 'journal', 'enabled']));
	const num = (v: string) => (v === '' ? '' : Number(v));
</script>

<label class="toggle" data-path="observability.tracing">
	<input type="checkbox" checked={ed.bool(['observability', 'tracing'])} onchange={(e) => ed.set(['observability', 'tracing'], e.currentTarget.checked)} />
	Trace every step in the logs
</label>
<label class="toggle" data-path="observability.journal.enabled">
	<input type="checkbox" checked={on} onchange={(e) => ed.set(['observability', 'journal', 'enabled'], e.currentTarget.checked)} />
	Keep every run in the journal, to read it back in Runs and with mani runs
</label>
{#if on}
	<div class="form-grid three">
		<div class="field">
			<label for="j-backend">Format <code>journal.backend</code></label>
			<select id="j-backend" class="input" data-path="observability.journal.backend" value={ed.str(['observability', 'journal', 'backend']) || 'jsonl'} onchange={(e) => ed.set(['observability', 'journal', 'backend'], e.currentTarget.value === 'jsonl' ? null : e.currentTarget.value)}>
				<option value="jsonl">JSONL files</option>
				<option value="sqlite">SQLite</option>
			</select>
		</div>
		<div class="field">
			<label for="j-path">Where <code>journal.path</code></label>
			<input id="j-path" class="input mono" placeholder="./runs" data-path="observability.journal.path" value={ed.str(['observability', 'journal', 'path'])} oninput={(e) => ed.set(['observability', 'journal', 'path'], e.currentTarget.value)} />
		</div>
		<div class="field">
			<label for="j-ret">Runs kept <code>journal.retention</code></label>
			<input id="j-ret" class="input" type="number" min="0" placeholder="all" data-path="observability.journal.retention" value={ed.num(['observability', 'journal', 'retention'])} oninput={(e) => ed.set(['observability', 'journal', 'retention'], num(e.currentTarget.value))} />
		</div>
	</div>
{/if}

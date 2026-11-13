<script lang="ts">
	import type { ManifestState as AgentEditor } from '../../agents/editor.svelte.ts';

	let { ed }: { ed: AgentEditor } = $props();
	const num = (v: string) => (v === '' ? '' : Number(v));
	const nums = [
		['max_tokens', 'Tokens per run', ''],
		['max_tool_calls', 'Tool calls per run', ''],
		['max_iterations', 'Turns of the loop', ''],
		['subagent_depth', 'Subagent depth', '']
	] as const;
	const durations = [
		['max_duration', 'Longest run', '2m'],
		['tool_timeout', 'Longest tool call', '60s']
	] as const;
</script>

<div class="form-grid three">
	{#each nums as [k, label] (k)}
		<div class="field">
			<label for="f-{k}">{label} <code>{k}</code></label>
			<input id="f-{k}" class="input" type="number" min="0" placeholder="no limit" data-path="limits.{k}" value={ed.num(['limits', k])} oninput={(e) => ed.set(['limits', k], num(e.currentTarget.value))} />
		</div>
	{/each}
	{#each durations as [k, label, ph] (k)}
		<div class="field">
			<label for="f-{k}">{label} <code>{k}</code></label>
			<input id="f-{k}" class="input mono" placeholder={ph} data-path="limits.{k}" value={ed.str(['limits', k])} oninput={(e) => ed.set(['limits', k], e.currentTarget.value)} />
		</div>
	{/each}
</div>
<p class="help">The first limit reached stops the run and is written in the journal. Durations read like 90s, 2m, 1h.</p>

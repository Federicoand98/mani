<script lang="ts">
	import type { ManifestState as AgentEditor } from '../../agents/editor.svelte.ts';

	let { ed }: { ed: AgentEditor } = $props();
	const num = (v: string) => (v === '' ? '' : Number(v));
</script>

<div class="form-grid">
	<div class="field">
		<label for="f-window">Context window, in tokens <code>context.window</code></label>
		<input id="f-window" class="input" type="number" min="0" step="1000" placeholder="from config.json" data-path="context.window" value={ed.num(['context', 'window'])} oninput={(e) => ed.set(['context', 'window'], num(e.currentTarget.value))} />
	</div>
	<div class="field">
		<label for="f-keep">Messages kept when compacting <code>context.compaction.keep</code></label>
		<input id="f-keep" class="input" type="number" min="2" placeholder="20" data-path="context.compaction.keep" value={ed.num(['context', 'compaction', 'keep'])} oninput={(e) => ed.set(['context', 'compaction', 'keep'], num(e.currentTarget.value))} />
	</div>
</div>
<label class="toggle" data-path="context.compaction.enabled">
	<input type="checkbox" checked={ed.bool(['context', 'compaction', 'enabled'], true)} onchange={(e) => ed.set(['context', 'compaction', 'enabled'], e.currentTarget.checked)} />
	Drop the oldest messages when the window is full
</label>
<label class="toggle" data-path="context.inject">
	<input type="checkbox" checked={ed.bool(['context', 'inject'], true)} onchange={(e) => ed.set(['context', 'inject'], e.currentTarget.checked)} />
	Read AGENTS.md from the working folder and add it to the prompt
</label>

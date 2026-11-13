<script lang="ts">
	import type { ManifestState as AgentEditor } from '../../agents/editor.svelte.ts';
	import Icon from '../Icon.svelte';

	let { ed }: { ed: AgentEditor } = $props();

	const triggers = $derived(ed.list(['run', 'triggers']) as Record<string, unknown>[]);
	const num = (v: string) => (v === '' ? '' : Number(v));
	const t = (i: number, k: string) => ['run', 'triggers', i, k];
	const s = (v: unknown) => (v === undefined || v === null ? '' : String(v));

	const add = (type: string) =>
		ed.edit((m) =>
			m.push(['run', 'triggers'], type === 'daily' ? { type, at: '07:00', prompt: '' } : type === 'every' ? { type, every: '30m', prompt: '' } : { type, path: '/hook', prompt: '' })
		);
</script>

<p class="sm mu">Without triggers the agent runs only when asked: from the chat, mani run or the API. Triggers start while mani run (or, later, mani web) is running.</p>

{#each triggers as tr, i (i)}
	<div class="card trigger" data-path="run.triggers[{i}]">
		<div class="subhead">
			<span class="badge muted">{tr.type}</span>
			<button class="btn ghost small" onclick={() => ed.edit((m) => m.removeAt(['run', 'triggers'], i))}><Icon name="trash" size={14} />Remove</button>
		</div>
		<div class="form-grid">
			{#if tr.type === 'daily'}
				<div class="field"><label for="t{i}-at">At <code>at</code></label><input id="t{i}-at" class="input mono" data-path="run.triggers[{i}].at" value={s(tr.at)} oninput={(e) => ed.set(t(i, 'at'), e.currentTarget.value)} /></div>
			{:else if tr.type === 'every'}
				<div class="field"><label for="t{i}-every">Every <code>every</code></label><input id="t{i}-every" class="input mono" data-path="run.triggers[{i}].every" value={s(tr.every)} oninput={(e) => ed.set(t(i, 'every'), e.currentTarget.value)} /></div>
			{:else if tr.type === 'webhook'}
				<div class="field"><label for="t{i}-path">Path <code>path</code></label><input id="t{i}-path" class="input mono" placeholder="/hook" data-path="run.triggers[{i}].path" value={s(tr.path)} oninput={(e) => ed.set(t(i, 'path'), e.currentTarget.value)} /></div>
			{/if}
			<div class="field"><label for="t{i}-name">Name <code>name</code></label><input id="t{i}-name" class="input mono" placeholder="optional" data-path="run.triggers[{i}].name" value={s(tr.name)} oninput={(e) => ed.set(t(i, 'name'), e.currentTarget.value)} /></div>
		</div>
		<div class="field"><label for="t{i}-prompt">Task <code>prompt</code></label><input id="t{i}-prompt" class="input" value={s(tr.prompt)} oninput={(e) => ed.set(t(i, 'prompt'), e.currentTarget.value)} /></div>
		<div class="row">
			<label class="toggle"><input type="checkbox" checked={tr.memory === 'persistent'} onchange={(e) => ed.set(t(i, 'memory'), e.currentTarget.checked ? 'persistent' : null)} />Remember previous runs</label>
			{#if tr.type !== 'webhook'}
				<label class="toggle"><input type="checkbox" checked={tr.catch_up === true} onchange={(e) => ed.set(t(i, 'catch_up'), e.currentTarget.checked || null)} />Catch up a missed run</label>
			{/if}
		</div>
		{#if tr.type === 'webhook'}<p class="help">The token comes from <code>${'{MANI_WEBHOOK_TOKEN}'}</code> or <code>token:</code> in the YAML; never paste a secret here.</p>{/if}
	</div>
{/each}

<div class="row">
	<button class="btn secondary small" onclick={() => add('every')}><Icon name="plus" size={14} />Every N minutes</button>
	<button class="btn secondary small" onclick={() => add('daily')}><Icon name="plus" size={14} />Every day</button>
	<button class="btn secondary small" onclick={() => add('webhook')}><Icon name="plus" size={14} />Webhook</button>
</div>

{#if triggers.length}
	<details class="queue">
		<summary class="label">Queue <code>run.scheduler</code></summary>
		<div class="form-grid" style="margin-top:12px">
			<div class="field"><label for="q-path">Queue folder <code>path</code></label><input id="q-path" class="input mono" placeholder="in memory" data-path="run.scheduler.path" value={ed.str(['run', 'scheduler', 'path'])} oninput={(e) => ed.set(['run', 'scheduler', 'path'], e.currentTarget.value)} /></div>
			<div class="field"><label for="q-conc">Runs at once <code>concurrency</code></label><input id="q-conc" class="input" type="number" min="0" placeholder="1" data-path="run.scheduler.concurrency" value={ed.num(['run', 'scheduler', 'concurrency'])} oninput={(e) => ed.set(['run', 'scheduler', 'concurrency'], num(e.currentTarget.value))} /></div>
			<div class="field"><label for="q-att">Attempts <code>retry.max_attempts</code></label><input id="q-att" class="input" type="number" min="1" placeholder="3" value={ed.num(['run', 'scheduler', 'retry', 'max_attempts'])} oninput={(e) => ed.set(['run', 'scheduler', 'retry', 'max_attempts'], num(e.currentTarget.value))} /></div>
			<div class="field"><label for="q-back">Wait between attempts <code>retry.backoff</code></label><input id="q-back" class="input mono" placeholder="30s" data-path="run.scheduler.retry.backoff" value={ed.str(['run', 'scheduler', 'retry', 'backoff'])} oninput={(e) => ed.set(['run', 'scheduler', 'retry', 'backoff'], e.currentTarget.value)} /></div>
		</div>
		<p class="help">With a queue folder, triggered runs survive a restart.</p>
	</details>
{/if}

<style>
	.trigger {
		display: flex;
		flex-direction: column;
		gap: 12px;
		padding: 16px;
	}
	.queue summary {
		cursor: pointer;
	}
</style>

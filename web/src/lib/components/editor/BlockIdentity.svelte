<script lang="ts">
	import { api } from '../../api/client.ts';
	import type { ManifestState as AgentEditor } from '../../agents/editor.svelte.ts';
	import { app } from '../../state.svelte.ts';

	let { ed }: { ed: AgentEditor } = $props();

	const provider = $derived(ed.str(['identity', 'provider']));
	const include = $derived(ed.includeOf(['identity', 'prompt']));
	let models = $state<string[]>([]);

	$effect(() => {
		const p = provider;
		models = [];
		if (app.providers.some((x) => x.name === p && x.enabled)) {
			api.models(p).then(
				(r) => provider === p && (models = r.models),
				() => {}
			);
		}
	});

	const known = $derived(app.providers.map((p) => p.name));
</script>

<div class="form-grid">
	<div class="field">
		<label for="f-name">Name <code>identity.name</code></label>
		<input id="f-name" class="input mono" data-path="identity.name" value={ed.str(['identity', 'name'])} oninput={(e) => ed.set(['identity', 'name'], e.currentTarget.value)} />
	</div>
	<div class="field">
		<label for="f-provider">Model provider <code>identity.provider</code></label>
		<select id="f-provider" class="input" data-path="identity.provider" value={provider} onchange={(e) => ed.set(['identity', 'provider'], e.currentTarget.value)}>
			{#if !provider}<option value="">Choose…</option>{/if}
			{#each app.providers as p (p.name)}
				<option value={p.name}>{p.label}{p.enabled ? '' : ' (not connected)'}</option>
			{/each}
			{#if provider && !known.includes(provider)}<option value={provider}>{provider}</option>{/if}
		</select>
	</div>
</div>
<div class="field">
	<label for="f-desc">Description <code>identity.description</code></label>
	<input id="f-desc" class="input" data-path="identity.description" value={ed.str(['identity', 'description'])} oninput={(e) => ed.set(['identity', 'description'], e.currentTarget.value)} />
	<p class="help">One sentence on what it does. A model calling it through MCP reads this.</p>
</div>
<div class="field">
	<label for="f-model">Model <code>identity.model</code></label>
	<input id="f-model" class="input mono" list="f-models" data-path="identity.model" value={ed.str(['identity', 'model'])} oninput={(e) => ed.set(['identity', 'model'], e.currentTarget.value)} />
	<datalist id="f-models">
		{#each models as mo (mo)}<option value={mo}></option>{/each}
	</datalist>
	{#if models.length}<p class="help">{models.length} models available from this provider.</p>{/if}
</div>
<div class="field">
	<label for="f-prompt">Prompt <code>identity.prompt</code></label>
	{#if include}
		<p class="banner info">The prompt comes from <code>{include}</code> (<code>!include</code>). Edit that file, or change the line in the YAML panel.</p>
	{:else}
		<textarea id="f-prompt" class="input" rows="8" data-path="identity.prompt" value={ed.str(['identity', 'prompt'])} oninput={(e) => ed.set(['identity', 'prompt'], e.currentTarget.value)}></textarea>
		<p class="help">What it must do, in your words. It is all the model knows about its job.</p>
	{/if}
</div>

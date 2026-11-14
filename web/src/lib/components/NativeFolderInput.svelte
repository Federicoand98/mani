<script lang="ts">
	import { onDestroy } from 'svelte';
	import { agentsApi } from '../api/client.ts';
	import Icon from './Icon.svelte';

	let { id, value, onchange, label = 'Workspace folder' }: {
		id: string; value: string; onchange: (value: string) => void; label?: string;
	} = $props();
	let picking = $state(false);
	let error = $state('');
	let alive = true;
	let controller: AbortController | undefined;
	onDestroy(() => { alive = false; controller?.abort(); });

	async function browse() {
		if (picking) return;
		picking = true;
		error = '';
		const initial = value;
		controller = new AbortController();
		try {
			const result = await agentsApi.pickFolder(initial, controller.signal);
			if (!alive) return;
			if (initial !== value) error = 'The workspace changed while the picker was open. Choose again.';
			else if (!result.cancelled) onchange(result.path);
		} catch (e) {
			if (alive) error = (e as Error).message;
		} finally {
			picking = false;
		}
	}
</script>

<div class="field">
	<label for={id}>{label}</label>
	<div class="folder-input">
		<input {id} class="input mono" data-path="capabilities.workspace" {value} placeholder="." aria-describedby={error ? `${id}-error` : undefined} oninput={(e) => { error = ''; onchange(e.currentTarget.value); }} />
		<button type="button" class="btn secondary" onclick={browse} disabled={picking} aria-haspopup="dialog"><Icon name="folder" size={16} />{picking ? 'Choosing…' : 'Browse'}</button>
	</div>
	{#if error}<p id="{id}-error" class="help error" role="alert">{error} Enter a path manually instead.</p>{/if}
</div>

<style>
	.folder-input { display: flex; align-items: center; gap: 8px; }
	.folder-input .input { flex: 1; min-width: 0; }
	.folder-input .btn { flex-shrink: 0; }
</style>

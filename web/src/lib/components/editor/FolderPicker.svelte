<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { agentsApi } from '../../api/client.ts';
	import Icon from '../Icon.svelte';

	let { open = $bindable(false), current, onpick }: { open: boolean; current: string; onpick: (path: string) => void } = $props();

	let path = $state<string[]>([]);
	let folders = $state<string[]>([]);
	let error = $state('');

	$effect(() => {
		if (!open) return;
		const start = current && current !== '.' && !current.startsWith('/') && !current.startsWith('..') ? current.replace(/^\.\//, '').split('/').filter(Boolean) : [];
		go(start);
	});

	async function go(p: string[]) {
		error = '';
		try {
			folders = (await agentsApi.folders(p.join('/') || '.')).folders;
			path = p;
		} catch (e) {
			error = (e as Error).message;
			if (p.length) go([]);
		}
	}

	function pick() {
		onpick(path.length ? './' + path.join('/') : '.');
		open = false;
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="scrim" />
		<Dialog.Content class="modal">
			<Dialog.Title>{#snippet child({ props })}<h2 {...props}>Choose the working folder</h2>{/snippet}</Dialog.Title>
			<nav class="crumbs" aria-label="Folder">
				<button class="btn ghost small" onclick={() => go([])}>this folder</button>
				{#each path as seg, i (i)}<span class="mu">/</span><button class="btn ghost small" onclick={() => go(path.slice(0, i + 1))}>{seg}</button>{/each}
			</nav>
			<div class="folders">
				{#each folders as f (f)}
					<button onclick={() => go([...path, f])}><Icon name="folder" size={16} />{f}/</button>
				{:else}
					<p class="sm mu">No folders inside.</p>
				{/each}
			</div>
			{#if error}<p class="help error">{error}</p>{/if}
			<p class="help">Folders inside the one mani web runs on. File tools will not reach outside the one you choose.</p>
			<div class="modal-foot">
				<Dialog.Close class="btn secondary">Cancel</Dialog.Close>
				<button class="btn primary" onclick={pick}>Use {path.length ? path.at(-1) + '/' : 'this folder'}</button>
			</div>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	.crumbs {
		display: flex;
		align-items: center;
		gap: 2px;
		flex-wrap: wrap;
	}
	.folders {
		border: 1px solid var(--bd);
		border-radius: 10px;
		padding: 4px;
		min-height: 140px;
		max-height: 300px;
		overflow: auto;
		display: flex;
		flex-direction: column;
	}
	.folders button {
		display: flex;
		align-items: center;
		gap: 10px;
		text-align: left;
		border: 0;
		background: none;
		padding: 9px 10px;
		border-radius: 8px;
		cursor: pointer;
		font-size: 14px;
	}
	.folders button:hover {
		background: var(--prt);
		color: var(--pr);
	}
	.folders p {
		padding: 10px;
	}
</style>

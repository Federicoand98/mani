<script lang="ts">
	import { onMount } from 'svelte';
	import type { Problem } from '../../api/types.ts';
	import type { YamlView } from '../../agents/cm.ts';

	let { text, problems, onchange, ref = $bindable() }: { text: string; problems: Problem[]; onchange: (t: string) => void; ref?: { gotoLine(n: number): void } } = $props();

	let host: HTMLDivElement;
	let view: YamlView | null = null;

	onMount(() => {
		let alive = true;
		import('../../agents/cm.ts').then(({ createYamlView }) => {
			if (!alive) return;
			view = createYamlView(host, text, onchange);
			view.setProblems(problems);
			ref = { gotoLine: (n) => view?.gotoLine(n) };
			ready = true;
		});
		return () => {
			alive = false;
			view?.destroy();
		};
	});

	// Read the props before touching the view: an effect that only reaches them
	// through `view?.` would not depend on them while the view is still loading.
	let ready = $state(false);
	$effect(() => {
		const t = text;
		if (ready) view?.setText(t);
	});

	$effect(() => {
		const ps = problems;
		if (ready) view?.setProblems(ps);
	});
</script>

<div class="yaml" bind:this={host}></div>

<style>
	.yaml {
		border-radius: 10px;
		overflow: hidden;
		height: 100%;
		min-height: 320px;
		background: var(--code-bg);
	}
	.yaml :global(.cm-editor) {
		height: 100%;
	}
</style>

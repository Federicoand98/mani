<script lang="ts">
	import { onMount, type Snippet } from 'svelte';
	import { read, write } from '../storage.ts';

	let { left, right }: { left: Snippet; right: Snippet } = $props();
	let root: HTMLDivElement;
	let splitter: HTMLDivElement;
	let ratio = $state(52);
	let minimum = $state(0);
	let maximum = $state(100);
	let dragging = $state(false);
	let mobile = $state<'edit' | 'try'>('edit');
	const KEY = 'mani:agent-workbench-split';

	function resize(percent: number, persist = true) {
		const available = root.clientWidth - 12;
		if (available < 680) return;
		minimum = 360 / available * 100;
		maximum = (available - 320) / available * 100;
		ratio = Math.max(minimum, Math.min(percent, maximum));
		if (persist) write(KEY, String(ratio));
	}
	function move(e: PointerEvent) {
		if (dragging) resize((e.clientX - root.getBoundingClientRect().left) / (root.clientWidth - 12) * 100);
	}
	function keyboard(e: KeyboardEvent) {
		if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) return;
		e.preventDefault();
		const change = 24 / (root.clientWidth - 12) * 100;
		resize(e.key === 'Home' ? minimum : e.key === 'End' ? maximum : ratio + (e.key === 'ArrowRight' ? change : -change));
	}
	onMount(() => {
		const saved = Number(read(KEY));
		if (saved > 0 && saved < 100) ratio = saved;
		const observer = new ResizeObserver(() => resize(ratio, false));
		observer.observe(root);
		return () => observer.disconnect();
	});
</script>

<div class="mobile-switch" role="group" aria-label="Workbench panel">
	<button type="button" class="btn small" aria-pressed={mobile === 'edit'} onclick={() => (mobile = 'edit')}>Configure</button>
	<button type="button" class="btn small" aria-pressed={mobile === 'try'} onclick={() => (mobile = 'try')}>Try agent</button>
</div>
<div class="panes" class:dragging bind:this={root} data-mobile={mobile} style:grid-template-columns="minmax(0, {ratio}fr) 12px minmax(0, {100 - ratio}fr)">
	<div class="left">{@render left()}</div>
	<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (ARIA window splitters require a focusable separator.) -->
	<div class="splitter" bind:this={splitter} role="separator" tabindex="0" aria-label="Resize configuration and chat" aria-orientation="vertical" aria-valuenow={Math.round(ratio)} aria-valuemin={Math.ceil(minimum)} aria-valuemax={Math.floor(maximum)} onkeydown={keyboard}
		onpointerdown={(e) => { if (e.button !== 0) return; e.preventDefault(); splitter.setPointerCapture(e.pointerId); dragging = true; }}
		onpointermove={move} onpointerup={() => (dragging = false)} onpointercancel={() => (dragging = false)} onlostpointercapture={() => (dragging = false)}
		ondblclick={() => resize(52)} title="Drag to resize. Arrow keys adjust; double-click resets."></div>
	<div class="right">{@render right()}</div>
</div>

<style>
	.panes { flex: 1; min-height: 0; min-width: 0; display: grid; }
	.left, .right { display: flex; flex-direction: column; min-width: 0; min-height: 0; }
	.left { background: var(--sf); }
	.splitter { cursor: col-resize; position: relative; border: 0; border-inline: 1px solid var(--bd); background: var(--bg); touch-action: none; padding: 0; }
	.splitter::after { content: ''; position: absolute; width: 3px; height: 36px; border-radius: 3px; background: var(--bs); top: 50%; left: 3px; transform: translateY(-50%); }
	.splitter:hover, .splitter:focus-visible { background: var(--prt); }
	.dragging, .dragging :global(*) { cursor: col-resize !important; user-select: none !important; }
	.mobile-switch { display: none; }
	@media (max-width: 760px) {
		.panes { display: flex; flex-direction: column; }
		.splitter { display: none; }
		.left, .right { flex: 1; }
		.panes[data-mobile='edit'] .right, .panes[data-mobile='try'] .left { display: none; }
		.mobile-switch { display: flex; padding: 8px 16px; gap: 8px; border-bottom: 1px solid var(--bd); }
		.mobile-switch button { flex: 1; }
		.mobile-switch button[aria-pressed='true'] { background: var(--prt); color: var(--pr); }
	}
</style>

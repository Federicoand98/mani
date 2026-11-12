<script lang="ts">
	import { toasts } from '../state.svelte.ts';
</script>

<div class="toasts" role="status" aria-live="polite">
	{#each toasts as t (t.id)}
		<div class="toast">{t.text}</div>
	{/each}
</div>

<style>
	.toasts {
		position: fixed;
		right: max(16px, env(safe-area-inset-right, 0px));
		bottom: calc(16px + env(safe-area-inset-bottom, 0px));
		display: flex;
		flex-direction: column;
		gap: 8px;
		z-index: 90;
	}
	.toast {
		background: var(--tx);
		color: var(--bg);
		padding: 10px 16px;
		border-radius: 10px;
		font-size: 14px;
		font-weight: 600;
		box-shadow: var(--sh2);
		max-width: 380px;
	}
</style>

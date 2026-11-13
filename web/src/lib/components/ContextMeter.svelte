<script lang="ts">
	import type { ContextUsage } from '../api/types.ts';
	import { m } from '../messages/en.ts';

	let { context = null, known = false, checking = false, onconfigure }: {
		context?: ContextUsage | null; known?: boolean; checking?: boolean; onconfigure?: () => void;
	} = $props();
	const t = m.context;
	const num = (n: number) => n.toLocaleString('en');
	const valid = $derived(!!context && context.input_budget > 0);
	const percent = $derived(valid ? Math.min(100, Math.max(0, context!.estimated_input / context!.input_budget * 100)) : 0);
</script>

<div class="context-meter" data-testid="context-meter">
	<details>
		<summary>
			<span>{t.title}</span>
			{#if context?.state === 'compacting'}<span role="status">{t.compacting}</span>
			{:else if valid}
				<span class="mono">~{num(context!.estimated_input)} / {num(context!.input_budget)}</span>
				<span class="track" aria-hidden="true"><span style:width="{percent}%"></span></span>
			{:else if context}<span class="mono">~{num(context.estimated_input)} / ?</span>
			{:else}<span>{checking ? t.checking : known ? t.known : t.unknown}</span>{/if}
		</summary>
		<div class="detail">
			{#if context}
				<dl>
					<dt>{t.estimated}</dt><dd>~{num(context.estimated_input)}</dd>
					<dt>{t.inputBudget}</dt><dd>{context.input_budget ? num(context.input_budget) : t.unknown}</dd>
					<dt>{t.output}</dt><dd>{num(context.output_limit)}</dd>
					<dt>{t.window}</dt><dd>{context.context_window ? num(context.context_window) : t.unreported}</dd>
					<dt>{t.maxInput}</dt><dd>{context.max_input_tokens ? num(context.max_input_tokens) : t.unreported}</dd>
					<dt>{t.maxOutput}</dt><dd>{context.max_output_tokens ? num(context.max_output_tokens) : t.unreported}</dd>
					<dt>{t.source}</dt><dd>{t.sources[context.limit_source] ?? (context.limit_source || t.unreported)}</dd>
					<dt>{t.lastInput}</dt><dd>{context.last_input === undefined ? t.unreported : num(context.last_input)}</dd>
				</dl>
				<p>{t.lastHelp}</p>
				{#if context.state === 'error'}<p class="error" role="status">{t.error}</p>{/if}
			{/if}
			<p>{t.draftHelp}</p>
			{#if onconfigure}<button type="button" class="btn secondary small" onclick={onconfigure}>{t.configure}</button>{/if}
		</div>
	</details>
	{#if !valid && !known && !checking && onconfigure}
		<button type="button" class="btn ghost small" onclick={onconfigure}>{t.configure}</button>
	{/if}
</div>

<style>
	.context-meter { display: flex; align-items: center; gap: 8px; min-width: 0; font-size: 11px; color: var(--mu); }
	details { position: relative; }
	summary { cursor: pointer; display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
	summary::marker { content: ''; }
	.track { width: 40px; height: 4px; border-radius: 4px; background: var(--bd); overflow: hidden; }
	.track > span { display: block; height: 100%; background: var(--pr); }
	.detail { position: absolute; bottom: calc(100% + 12px); right: 0; width: min(340px, 85vw); z-index: 10; padding: 14px; border: 1px solid var(--bd); border-radius: var(--r-card); background: var(--sf); box-shadow: var(--shadow); }
	dl { display: grid; grid-template-columns: 1fr auto; gap: 6px 12px; margin: 0; }
	dd { margin: 0; color: var(--tx); }
	p { margin: 10px 0; }
	@media (max-width: 600px) {
		.detail { position: fixed; bottom: 112px; left: 16px; right: 16px; width: auto; max-height: 60dvh; overflow: auto; }
	}
</style>

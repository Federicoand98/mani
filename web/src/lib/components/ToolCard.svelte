<script lang="ts">
	import { RISK_CLASS, RISK_LABEL, TOOL_INFO } from '../agents/tools.ts';
	import Icon from './Icon.svelte';

	let {
		name,
		input = '',
		result,
		isError = false,
		running = false
	}: { name: string; input?: string | Record<string, unknown>; result?: string; isError?: boolean; running?: boolean } = $props();

	const args = $derived(typeof input === 'string' ? input : JSON.stringify(input));
	// The one argument people read first: the command, the path, the URL.
	const summary = $derived.by(() => {
		try {
			const o = typeof input === 'string' ? JSON.parse(input || '{}') : input;
			return String(o.command ?? o.path ?? o.url ?? o.pattern ?? o.task ?? Object.values(o)[0] ?? '');
		} catch {
			return args;
		}
	});
	const info = $derived(TOOL_INFO[name]);
</script>

<details class="tool" open={isError || undefined}>
	<summary>
		<Icon name={name === 'bash' ? 'code' : 'tool'} size={14} />
		<b class="mono">{name}</b>
		<span class="arg mono">{summary}</span>
		{#if info && info.risk !== 'none'}<span class="badge {RISK_CLASS[info.risk]}">{RISK_LABEL[info.risk]}</span>{/if}
		{#if running}<span class="badge">Running</span>
		{:else if isError}<span class="badge error">Error</span>
		{:else if result !== undefined}<span class="badge ok">Done</span>{/if}
	</summary>
	<div class="body">
		<div class="label-row cap">Input</div>
		<pre>{args}</pre>
		{#if result !== undefined}
			<div class="label-row cap">{isError ? 'Error' : 'Result'}</div>
			<pre class:err={isError}>{result}</pre>
		{/if}
	</div>
</details>

<style>
	.tool {
		max-width: 760px;
		width: 100%;
		margin: 0 auto;
		border: 1px solid var(--bd);
		border-radius: 10px;
		background: var(--sf);
		font-size: 13px;
		overflow: hidden;
	}
	summary {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 8px 12px;
		cursor: pointer;
		list-style: none;
	}
	summary::-webkit-details-marker {
		display: none;
	}
	.arg {
		flex: 1;
		min-width: 0;
		color: var(--mu);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 12.5px;
	}
	.body {
		border-top: 1px solid var(--bd);
		padding: 8px 12px 12px;
		background: var(--sk);
	}
	.label-row {
		margin: 4px 0;
	}
	pre {
		margin: 0;
		font-family: var(--f-mono);
		font-size: 12px;
		line-height: 18px;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		max-height: 240px;
		overflow: auto;
	}
	pre.err {
		color: var(--er);
	}
</style>

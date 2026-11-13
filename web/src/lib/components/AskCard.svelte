<script lang="ts">
	import type { Ask } from '../chat/turn.ts';
	import { RISK_CLASS, RISK_LABEL } from '../agents/tools.ts';
	import Icon from './Icon.svelte';

	let { ask, agent, onanswer }: { ask: Ask; agent: string; onanswer: (decision: string) => void } = $props();

	const risk = $derived((ask.risk in RISK_LABEL ? ask.risk : 'none') as keyof typeof RISK_LABEL);
	// What the tool will act on, as a person reads it: the command, the path, the URL.
	const shown = $derived(
		ask.preview || String(ask.input.command ?? ask.input.path ?? ask.input.url ?? '') || JSON.stringify(ask.input, null, 2)
	);
	const outcome: Record<string, string> = {
		allow_once: 'Allowed once',
		allow_conversation: 'Allowed for this conversation',
		deny: 'Denied'
	};
</script>

{#if ask.state === 'open'}
	<div class="ask" role="alert">
		<div class="q">
			<Icon name="lock" size={16} />
			<span><b>{agent}</b> asks to use <code>{ask.tool}</code></span>
			<span class="badge {RISK_CLASS[risk]}">{RISK_LABEL[risk]}</span>
		</div>
		<pre>{shown}</pre>
		<div class="row">
			<button class="btn secondary small" onclick={() => onanswer('deny')}>Deny</button>
			<button class="btn secondary small" onclick={() => onanswer('allow_conversation')}>Allow for this conversation</button>
			<button class="btn primary small" onclick={() => onanswer('allow_once')}>Allow once</button>
		</div>
		<span class="cap">The permissions of this agent say "Ask" for {ask.tool}. Unanswered, the request is denied after 10 minutes. The manifest does not change.</span>
	</div>
{:else}
	<div class="done cap"><Icon name="lock" size={14} />{ask.tool}: {outcome[ask.state] ?? ask.state}</div>
{/if}

<style>
	.ask {
		max-width: 760px;
		width: 100%;
		margin: 0 auto;
		border: 1px solid var(--wa);
		background: var(--wat);
		border-radius: 12px;
		padding: 14px 16px;
		display: flex;
		flex-direction: column;
		gap: 10px;
	}
	.q {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
		font-size: 14px;
	}
	pre {
		margin: 0;
		background: var(--code-bg);
		color: var(--code-tx);
		border-radius: 8px;
		padding: 10px 12px;
		font-family: var(--f-mono);
		font-size: 12.5px;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		max-height: 200px;
		overflow: auto;
	}
	.done {
		max-width: 760px;
		width: 100%;
		margin: 0 auto;
		display: flex;
		align-items: center;
		gap: 6px;
	}
</style>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { onDestroy, onMount, tick } from 'svelte';
	import { agentsApi } from '../api/client.ts';
	import type { ContextUsage } from '../api/types.ts';
	import type { ServerMsg } from '../api/wire.ts';
	import { ChatSocket } from '../chat/socket.ts';
	import { idle, reduce, start, type Ask, type Segment, type Turn } from '../chat/turn.ts';
	import { getKeep } from '../prefs.ts';
	import AskCard from './AskCard.svelte';
	import ChatMessage from './ChatMessage.svelte';
	import Icon from './Icon.svelte';
	import ToolCard from './ToolCard.svelte';
	import ContextMeter from './ContextMeter.svelte';
	import { contextSummarized, reduceContext } from '../chat/context.ts';
	import { m as messages } from '../messages/en.ts';
	import { toast } from '../state.svelte.ts';

	// A trial of an agent that is not saved yet (M4-4): a real conversation, tools
	// and approvals included, that mani keeps in memory and drops at the end.
	let { text, folder, who, onworked, onbusy }: { text: string; folder: string; who: string; onworked: () => void; onbusy?: (busy: boolean) => void } = $props();

	type Entry = { kind: 'user'; text: string } | { kind: 'reply'; segments: Segment[]; error: string };
	let history = $state<Entry[]>([]);
	let turn = $state<Turn>(idle());
	let input = $state('');
	let context = $state<ContextUsage | null>(null);
	let summarized = $state(false);
	let error = $state('');
	let ready = $state(false);
	let log: HTMLElement | undefined = $state();
	let socket: ChatSocket | null = null;
	let draftId = '';
	let destroyed = false;
	$effect(() => onbusy?.(turn.status !== 'idle'));

	onMount(async () => {
		try {
			const d = await agentsApi.draft(text, folder);
			if (destroyed) {
				await agentsApi.endDraft(d.id);
				return;
			}
			draftId = d.id;
			socket = new ChatSocket(d.id, onMessage, () => {}, `/api/drafts/${encodeURIComponent(d.id)}/ws`);
			ready = true;
		} catch (e) {
			if (!destroyed) error = `The agent cannot start yet: ${(e as Error).message}. Review the agent settings.`;
			else toast((e as Error).message);
		}
	});

	onDestroy(() => {
		destroyed = true;
		socket?.close();
		onbusy?.(false);
		if (draftId) agentsApi.endDraft(draftId).catch((e: Error) => toast(e.message));
	});

	async function onMessage(m: ServerMsg) {
		if (destroyed) return;
		if (contextSummarized(context, m)) summarized = true;
		context = reduceContext(context, m);
		turn = reduce(turn, m, Date.now());
		if (m.type === 'done' || m.type === 'cancelled' || m.type === 'error') {
			history = [...history, { kind: 'reply', segments: turn.segments, error: turn.error }];
			if (m.type === 'done') onworked();
			turn = idle();
		}
		await tick();
		if (log) log.scrollTop = log.scrollHeight;
	}

	function send(e: Event) {
		e.preventDefault();
		const t = input.trim();
		if (!t || !socket || turn.status !== 'idle') return;
		history = [...history, { kind: 'user', text: t }];
		input = '';
		turn = start(Date.now());
		socket.send({ type: 'input', input: t, keep: getKeep() });
	}

	const answer = (a: Ask, decision: string) => socket?.send({ type: 'permission_response', request_id: a.id, decision });
</script>

<div class="trial">
	<div class="log" bind:this={log} role="log" aria-busy={turn.status !== 'idle'}>
		{#if error}
			<div class="banner error" role="alert"><Icon name="warn" /><span>{error}</span></div>
		{:else if !history.length && turn.status === 'idle'}
			<p class="sm mu hint">Tools run for real. Trial history is not saved.</p>
		{/if}
		{#each history as h, i (i)}
			{#if h.kind === 'user'}
				<ChatMessage role="user" parts={[{ type: 'text', text: h.text }]} />
			{:else}
				{#each h.segments as s, k (k)}
					{#if s.kind === 'text'}<ChatMessage role="assistant" parts={[{ type: 'text', text: s.text }]} {who} />
					{:else if s.kind === 'tool'}<ToolCard name={s.name} input={s.input} result={s.result} isError={s.isError} />
					{:else}<AskCard ask={s} agent={who} onanswer={() => {}} />{/if}
				{/each}
				{#if h.error}<div class="banner error" role="alert"><Icon name="x" /><span>{h.error}</span></div>{/if}
			{/if}
		{/each}
		{#if turn.status !== 'idle'}
			{#if !turn.segments.length}<ChatMessage role="assistant" parts={[]} {who} streaming />{/if}
			{#each turn.segments as s, k (k)}
				{#if s.kind === 'text'}<ChatMessage role="assistant" parts={[{ type: 'text', text: s.text }]} {who} streaming={k === turn.segments.length - 1} />
				{:else if s.kind === 'tool'}<ToolCard name={s.name} input={s.input} result={s.result} isError={s.isError} running={s.result === undefined} />
				{:else}<AskCard ask={s} agent={who} onanswer={(d) => answer(s, d)} />{/if}
			{/each}
		{/if}
	</div>
	{#if summarized}<p class="sm mu" role="note">{messages.context.summarized}</p>{/if}
	<ContextMeter {context} onconfigure={() => goto('/providers')} />
	<form class="row" onsubmit={send} style="flex-wrap:nowrap">
		<label class="sr-only" for="trial-input">Task for the trial</label>
		<input id="trial-input" class="input" placeholder="Write a task…" bind:value={input} disabled={!ready || turn.status !== 'idle'} autocomplete="off" />
		{#if turn.status !== 'idle'}
			<button type="button" class="btn secondary" onclick={() => socket?.send({ type: 'cancel' })}><Icon name="stop" size={14} />Stop</button>
		{:else}
			<button type="submit" class="btn primary" disabled={!ready || !input.trim()} aria-label="Send"><Icon name="send" size={16} /></button>
		{/if}
	</form>
</div>

<style>
	.trial {
		display: flex;
		flex-direction: column;
		gap: 12px;
		flex: 1;
		min-height: 0;
	}
	.log {
		min-height: 0;
		flex: 1;
		overflow: auto;
		background: var(--bg);
		padding: 8px 0;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.hint {
		margin: auto;
		text-align: center;
		max-width: 48ch;
	}
</style>

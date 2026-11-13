<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import { agentsApi } from '../api/client.ts';
	import type { ServerMsg } from '../api/wire.ts';
	import { ChatSocket } from '../chat/socket.ts';
	import { idle, reduce, start, type Ask, type Segment, type Turn } from '../chat/turn.ts';
	import { getKeep } from '../prefs.ts';
	import AskCard from './AskCard.svelte';
	import ChatMessage from './ChatMessage.svelte';
	import Icon from './Icon.svelte';
	import ToolCard from './ToolCard.svelte';

	// A trial of an agent that is not saved yet (M4-4): a real conversation, tools
	// and approvals included, that mani keeps in memory and drops at the end.
	let { text, folder, who, onworked }: { text: string; folder: string; who: string; onworked: () => void } = $props();

	type Entry = { kind: 'user'; text: string } | { kind: 'reply'; segments: Segment[]; error: string };
	let history = $state<Entry[]>([]);
	let turn = $state<Turn>(idle());
	let input = $state('');
	let error = $state('');
	let ready = $state(false);
	let log: HTMLElement | undefined = $state();
	let socket: ChatSocket | null = null;
	let draftId = '';

	onMount(async () => {
		try {
			const d = await agentsApi.draft(text, folder);
			draftId = d.id;
			socket = new ChatSocket(d.id, onMessage, () => {}, `/api/drafts/${encodeURIComponent(d.id)}/ws`);
			ready = true;
		} catch (e) {
			error = `The agent cannot start yet: ${(e as Error).message}. Go back to Check.`;
		}
	});

	onDestroy(() => {
		socket?.close();
		if (draftId) agentsApi.endDraft(draftId).catch(() => {});
	});

	async function onMessage(m: ServerMsg) {
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
			<p class="sm mu hint">Give it a real task. Tools run for real, inside its working folder; nothing of this conversation is kept.</p>
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
	}
	.log {
		min-height: 260px;
		max-height: 460px;
		overflow: auto;
		border: 1px solid var(--bd);
		border-radius: var(--r-card);
		background: var(--bg);
		padding: 16px;
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

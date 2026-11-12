<script lang="ts">
	import type { ChatPart } from '../api/types.ts';
	import { renderMarkdown } from '../chat/markdown.ts';
	import { m } from '../messages/en.ts';

	let {
		role,
		parts,
		who = '',
		streaming = false,
		thinking = '',
		thinkingMs = 0,
		thinkingLive = false,
		showThinking = false,
		meta = ''
	}: {
		role: string;
		parts: ChatPart[];
		who?: string;
		streaming?: boolean;
		thinking?: string;
		thinkingMs?: number;
		thinkingLive?: boolean;
		showThinking?: boolean;
		meta?: string;
	} = $props();

	const t = m.chat;
	const text = $derived(parts.filter((p) => p.type === 'text').map((p) => p.text ?? '').join(''));
	const images = $derived(parts.filter((p) => p.type === 'image'));
	const html = $derived(role === 'assistant' ? renderMarkdown(text) : '');

	function secs(ms: number) {
		return ms < 1000 ? '<1 s' : `${(ms / 1000).toFixed(ms < 10000 ? 1 : 0)} s`;
	}

	// Copy buttons are part of the rendered Markdown, so one handler serves them all.
	async function onclick(e: MouseEvent) {
		const btn = (e.target as HTMLElement).closest('[data-copy]') as HTMLButtonElement | null;
		if (!btn) return;
		const code = btn.closest('.code-block')?.querySelector('code')?.textContent ?? '';
		try {
			await navigator.clipboard.writeText(code);
			btn.textContent = t.copied;
			setTimeout(() => (btn.textContent = t.copy), 1500);
		} catch {
			// clipboard refused: the code stays selectable
		}
	}
</script>

{#if role === 'user'}
	<div class="msg user">
		{#if images.length}
			<div class="thumbs">
				{#each images as img, i (i)}
					<img src={`data:${img.mediaType};base64,${img.data}`} alt="" />
				{/each}
			</div>
		{/if}
		{#if text}<div class="bubble">{text}</div>{/if}
	</div>
{:else}
	<div class="msg assistant">
		<div class="who"><span class="avatar" aria-hidden="true">{who.slice(0, 1).toUpperCase() || 'm'}</span>{who}</div>
		{#if thinking}
			<details class="thinking" open={showThinking || undefined}>
				<summary>{thinkingLive ? t.thinkingNow : t.thinkingFor(secs(thinkingMs))}</summary>
				<div class="thinking-text">{thinking}</div>
			</details>
		{/if}
		<!-- eslint-disable-next-line svelte/no-at-html-tags -- sanitized by renderMarkdown -->
		<div class="md" class:caret={streaming} {onclick} role="presentation">{@html html}</div>
		{#if meta}<div class="meta">{meta}</div>{/if}
	</div>
{/if}

<style>
	.msg {
		max-width: 760px;
		width: 100%;
		margin: 0 auto;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.user {
		align-items: flex-end;
	}
	.bubble {
		background: var(--pr);
		color: var(--onpr);
		padding: 10px 14px;
		border-radius: 14px 14px 4px 14px;
		max-width: 85%;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.thumbs {
		display: flex;
		gap: 6px;
		flex-wrap: wrap;
		justify-content: flex-end;
	}
	.thumbs img {
		max-width: 220px;
		max-height: 160px;
		border-radius: 10px;
		border: 1px solid var(--bd);
		object-fit: cover;
	}
	.who {
		display: flex;
		align-items: center;
		gap: 8px;
		font-size: 12px;
		font-weight: 700;
		color: var(--mu);
		font-family: var(--f-mono);
	}
	.avatar {
		width: 22px;
		height: 22px;
		border-radius: 6px;
		background: var(--prt);
		color: var(--pr);
		display: grid;
		place-items: center;
		font-family: var(--f-ui);
		font-size: 11px;
		font-weight: 800;
	}
	.thinking {
		font-size: 13px;
		color: var(--mu);
		border-left: 2px solid var(--bd);
		padding-left: 12px;
	}
	.thinking summary {
		cursor: pointer;
		font-weight: 600;
		width: max-content;
	}
	.thinking-text {
		white-space: pre-wrap;
		margin-top: 6px;
		max-height: 320px;
		overflow: auto;
	}
	.meta {
		font-size: 12px;
		color: var(--mu);
		font-variant-numeric: tabular-nums;
	}
	.md {
		line-height: 26px;
		overflow-wrap: anywhere;
		min-width: 0;
	}
	.md :global(p) {
		margin: 0 0 10px;
	}
	.md :global(p:last-child) {
		margin-bottom: 0;
	}
	.md :global(ul),
	.md :global(ol) {
		margin: 0 0 10px;
		padding-left: 22px;
	}
	.md :global(h1),
	.md :global(h2),
	.md :global(h3) {
		font-size: 17px;
		line-height: 24px;
		margin: 14px 0 6px;
	}
	.md :global(table) {
		border-collapse: collapse;
		font-size: 14px;
		margin: 0 0 10px;
		display: block;
		overflow-x: auto;
	}
	.md :global(th),
	.md :global(td) {
		border: 1px solid var(--bd);
		padding: 6px 10px;
		text-align: left;
	}
	.md :global(blockquote) {
		margin: 0 0 10px;
		padding-left: 12px;
		border-left: 3px solid var(--bd);
		color: var(--mu);
	}
	.md :global(.code-block) {
		margin: 0 0 10px;
		background: var(--code-bg);
		border: 1px solid var(--bd);
		border-radius: 10px;
		overflow: hidden;
	}
	.md :global(.code-head) {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
		padding: 6px 8px 0 16px;
	}
	.md :global(pre) {
		margin: 0;
		background: var(--code-bg);
		color: var(--code-tx);
		padding: 8px 16px 14px;
		overflow-x: auto;
		font-size: 13px;
		line-height: 20px;
	}
	.md :global(pre code) {
		background: none;
		padding: 0;
		font-size: 13px;
	}
	.md :global(.code-copy) {
		background: #2e3347;
		color: #eceef8;
		border: 0;
		border-radius: 6px;
		height: 26px;
		padding: 0 10px;
		font-size: 12px;
		font-weight: 600;
		cursor: pointer;
	}
	.md :global(.code-lang) {
		font-family: var(--f-mono);
		font-size: 11px;
		color: var(--code-mu);
	}
	.caret :global(> :last-child::after),
	.caret:empty::after {
		content: '▍';
		color: var(--pr);
		animation: blink 1s steps(1) infinite;
	}
	@keyframes blink {
		50% {
			opacity: 0;
		}
	}
</style>

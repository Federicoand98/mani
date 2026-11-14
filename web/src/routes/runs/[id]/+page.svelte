<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { agentsApi, ApiError, runsApi } from '#lib/api/client.ts';
	import type { Run, RunDetail } from '#lib/api/types.ts';
	import { Manifest } from '#lib/agents/doc.ts';
	import Icon from '#lib/components/Icon.svelte';
	import { m } from '#lib/messages/en.ts';
	import { duration, sourceLabel, timeline, when, type Kind } from '#lib/runs/runs.ts';
	import { toast } from '#lib/state.svelte.ts';

	const t = m.runs;
	const POLL_MS = 3000;

	const id = $derived(page.params.id ?? '');
	let run = $state<RunDetail | null>(null);
	let missing = $state(false);
	let error = $state('');
	let chip = $state<'all' | Kind>('all');
	let siblings = $state<Run[]>([]);
	let changed = $state('');
	let now = $state(Date.now());
	let newSeq = $state(-1);

	const lines = $derived(run ? timeline(run.events, run.source) : []);
	const count = (k: Kind) => lines.filter((l) => (k === 'rules' ? l.rules : l.kind === k)).length;
	const shown = $derived(chip === 'all' ? lines : lines.filter((l) => (chip === 'rules' ? l.rules : l.kind === chip)));
	const running = $derived(run?.status === 'running');
	const command = $derived(run ? `mani runs ${run.id}${run.agentPath ? ` --config ${run.agentPath}` : ''}` : '');
	const position = $derived(run ? siblings.findIndex((r) => r.id === run!.id) : -1);

	// Iterations are the agent's own model answers; its subagents have theirs.
	const iterations = $derived(run ? run.events.filter((e) => e.kind === 'llm_response' && e.depth === 0).length : 0);
	const seconds = (d: string) => {
		const m = /^(\d+(?:\.\d+)?)(ms|s|m|h)$/.exec(d.trim());
		if (!m) return 0;
		return Number(m[1]) * { ms: 0.001, s: 1, m: 60, h: 3600 }[m[2] as 'ms' | 's' | 'm' | 'h'];
	};
	const elapsed = $derived(run ? ((run.endedAt ? Date.parse(run.endedAt) : now) - Date.parse(run.startedAt)) / 1000 : 0);
	const limits = $derived.by(() => {
		const l = run?.limits;
		if (!run || !l) return [];
		const out: { label: string; used: number; max: number; text: string }[] = [];
		const fmt = (n: number) => n.toLocaleString('en-US');
		if (l.maxToolCalls) out.push({ label: t.limitTools, used: run.summary.toolCalls, max: l.maxToolCalls, text: `${run.summary.toolCalls} / ${l.maxToolCalls}` });
		if (l.maxIterations) out.push({ label: t.limitIterations, used: iterations, max: l.maxIterations, text: `${iterations} / ${l.maxIterations}` });
		const tok = run.summary.inTokens + run.summary.outTokens;
		if (l.maxTokens) out.push({ label: t.limitTokens, used: tok, max: l.maxTokens, text: `${fmt(tok)} / ${fmt(l.maxTokens)}` });
		const max = seconds(l.maxDuration);
		if (max) out.push({ label: t.limitDuration, used: elapsed, max, text: `${duration(run.startedAt, run.endedAt, now)} / ${l.maxDuration}` });
		return out;
	});

	async function load() {
		try {
			run = await runsApi.get(id);
			missing = false;
			error = '';
		} catch (e) {
			if (e instanceof ApiError && e.status === 404) missing = true;
			else error = (e as Error).message;
			return;
		}
		const r = run;
		if (r.source === 'web' && r.sessionId) {
			runsApi
				.list(`?period=all&limit=200&session=${encodeURIComponent(r.sessionId)}`)
				.then((res) => (siblings = res.runs.filter((x) => x.source === r.source).reverse()), () => {});
		} else siblings = [];
		compareLimits(r);
	}

	// compareLimits says which limits the manifest has changed since the run (M5-3).
	async function compareLimits(r: RunDetail) {
		changed = '';
		if (!r.agentPath || !r.limits) return;
		try {
			const doc = new Manifest((await agentsApi.get(r.agentPath)).text);
			const now = (k: string) => doc.get(['limits', k]);
			const diffs: string[] = [];
			const pairs: [string, unknown][] = [
				['max_tool_calls', r.limits.maxToolCalls || undefined],
				['max_tokens', r.limits.maxTokens || undefined],
				['max_duration', r.limits.maxDuration || undefined]
			];
			for (const [k, then] of pairs) {
				const today = now(k) ?? undefined;
				if (String(today ?? '') !== String(then ?? '')) diffs.push(`${k} ${then ?? 'none'} then, ${today ?? 'none'} now`);
			}
			changed = diffs.join('; ');
		} catch {
			// the manifest may be gone: the run still stands on its own
		}
	}

	// poll appends the steps written since the last one, while the run goes on.
	async function poll() {
		if (!run || run.status !== 'running') return;
		const last = run.events.at(-1)?.seq ?? -1;
		try {
			const d = await runsApi.get(run.id, last);
			if (!run || d.id !== run.id) return;
			newSeq = d.events[0]?.seq ?? -1;
			run = { ...d, events: [...run.events, ...d.events] };
		} catch {
			// retried on the next tick
		}
	}

	function download() {
		if (!run) return;
		const blob = new Blob([JSON.stringify(run, null, 2)], { type: 'application/json' });
		const a = document.createElement('a');
		a.href = URL.createObjectURL(blob);
		a.download = `run-${run.id}.json`;
		a.click();
		URL.revokeObjectURL(a.href);
	}

	async function copy() {
		try {
			await navigator.clipboard.writeText(command);
			toast(`${t.copied}: ${command}`);
		} catch {
			toast(command);
		}
	}

	$effect(() => {
		id;
		run = null;
		chip = 'all';
		load();
	});

	onMount(() => {
		const tick = setInterval(() => {
			now = Date.now();
			if (document.visibilityState === 'visible' && Math.floor(now / 1000) % (POLL_MS / 1000) === 0) poll();
		}, 1000);
		return () => clearInterval(tick);
	});

	const statusClass = (s: string) => (s === 'ok' ? 'ok' : s === 'error' ? 'error' : s === 'running' ? 'live' : 'muted');
	const pct = (used: number, max: number) => Math.min(100, Math.round((used / max) * 100));
</script>

<svelte:head><title>{run?.agent || t.title} - mani</title></svelte:head>

{#if missing}
	<div class="card">
		<div class="empty">
			<div class="icon"><Icon name="runs" size={24} /></div>
			<h2 class="h3">{t.notFound}</h2>
			<a class="btn secondary" href="/runs">{t.title}</a>
		</div>
	</div>
{:else if error}
	<div class="banner error" role="alert"><Icon name="warn" /><span>{error}</span></div>
{:else if !run}
	<div class="card stack" aria-busy="true"><span class="sr-only">{t.loading}</span><div class="skeleton" style="width:40%"></div><div class="skeleton"></div></div>
{:else}
	<div class="head">
		<div class="title">
			<div class="cap crumb">
				<a href="/runs">{t.title}</a> /
				{#if run.conversationTitle}<a href="/runs?source=web">{run.conversationTitle}</a> /{/if}
				<span class="mono">{run.id}</span>
			</div>
			<h1>
				{run.agent || t.unknownAgent}
				<span class="badge {statusClass(run.status)}">
					{#if running}<span class="dot"></span>{:else}<Icon name={run.status === 'ok' ? 'check' : run.status === 'error' ? 'x' : 'stop'} size={12} />{/if}
					{t.statuses[run.status] ?? run.status}
				</span>
			</h1>
		</div>
		<div class="row">
			{#if run.conversation}<a class="btn secondary" href="/chat?c={encodeURIComponent(run.conversation)}"><Icon name="chat" size={16} />{t.openConversation}</a>{/if}
			<button class="btn secondary" onclick={copy}><Icon name="copy" size={16} />{t.copyCommand}</button>
			<button class="btn secondary" onclick={download}><Icon name="download" size={16} />{t.downloadJson}</button>
		</div>
	</div>

	{#if siblings.length > 1 && position >= 0}
		<div class="row sm mu">
			{t.messageOf(position + 1, siblings.length)}
			<a class="btn secondary small" class:off={position === 0} href={position > 0 ? `/runs/${siblings[position - 1].id}` : undefined}>{t.previous}</a>
			<a class="btn secondary small" class:off={position === siblings.length - 1} href={position < siblings.length - 1 ? `/runs/${siblings[position + 1].id}` : undefined}>{t.next}</a>
		</div>
	{/if}

	{#if run.status === 'error'}
		<div class="banner error">
			<Icon name="x" />
			<div class="stack grow" style="gap:8px">
				<span><b>{t.stopped}</b> {run.error}</span>
				{#if run.agentPath && /max_|budget|deadline|limit/i.test(run.error)}
					<div><a class="btn secondary small" href="/agents/{run.agentPath}?block=limits">{t.openLimits}</a></div>
				{/if}
			</div>
		</div>
	{/if}
	{#if !run.agent && !run.task}
		<div class="banner info"><Icon name="warn" /><span><b>{t.oldTitle}</b> {t.oldBody}</span></div>
	{/if}

	<div class="layout">
		<div class="stack main">
			{#if run.task}
				<section class="card"><h2 class="h3">{t.task}</h2><p class="text">{run.task}</p></section>
			{/if}
			{#if running}
				<section class="card"><h2 class="h3">{t.answer}</h2><p class="sm mu">{t.stillWorking}</p></section>
			{:else if run.answer}
				<section class="card"><h2 class="h3">{t.answer}</h2><p class="text">{run.answer}</p></section>
			{:else if run.result}
				<section class="card"><h2 class="h3">{t.result}</h2><pre class="code">{JSON.stringify(run.result, null, 2)}</pre></section>
			{:else if run.agent && run.status !== 'error'}
				<section class="card"><h2 class="h3">{t.answer}</h2><p class="sm mu">{t.noAnswer}</p></section>
			{/if}

			<section class="card flush">
				<div class="tl-head">
					<h2 class="h3">{t.timeline}</h2>
					<div class="row">
						{#each ['all', 'tool', 'rules', 'model'] as const as k (k)}
							<button class="chip" aria-pressed={chip === k} onclick={() => (chip = k)}>{t.chips[k]} {k === 'all' ? lines.length : count(k)}</button>
						{/each}
					</div>
				</div>
				<ol class="tl">
					{#each shown as l (l.ev.seq)}
						<li class="ev" class:blocked={l.blocked} class:new={l.ev.seq >= newSeq && newSeq >= 0}>
							<span class="ts mono">{new Date(l.ev.at).toLocaleTimeString('en-GB')}</span>
							<span class="ic {l.tone}"><Icon name={l.icon} size={13} /></span>
							<div class="de" class:sub={l.depth > 0} style:margin-left="{Math.min(l.depth, 4) * 16}px">
								<div class="l">
									{#if l.tool}<b class="mono">{l.tool}</b>{#if l.arg}<span class="arg mono">{l.arg}</span>{/if}{:else}<b>{l.title}</b>{/if}
								</div>
								<div class="d">{l.detail}</div>
								{#if l.output}
									<details>
										<summary>{l.cut ? t.preview : t.output}</summary>
										<pre class="code">{l.output}</pre>
										{#if l.cut}<p class="cap">{t.previewNote}</p>{/if}
									</details>
								{/if}
							</div>
						</li>
					{:else}
						<li class="sm mu pad">{t.noEvents}</li>
					{/each}
				</ol>
				{#if running}<div class="foot sm mu"><Icon name="spark" size={14} />{t.waiting}</div>{/if}
			</section>
		</div>

		<aside class="stack">
			<section class="card">
				<h2 class="h3">{t.summary}</h2>
				<dl class="kv">
					<dt>{t.manifest}</dt><dd class="mono">{run.agentPath || run.manifest || t.notRecorded}</dd>
					<dt>{t.model}</dt><dd class="mono">{run.model || t.notRecorded}</dd>
					<dt>{t.startedFrom}</dt><dd>{sourceLabel(run.source)}</dd>
					<dt>{t.started}</dt><dd>{when(run.startedAt)}</dd>
					<dt>{t.duration}</dt><dd class="num">{duration(run.startedAt, run.endedAt, now)}</dd>
					<dt>{t.tokens}</dt><dd class="num">{run.summary.inTokens.toLocaleString('en-US')} in - {run.summary.outTokens.toLocaleString('en-US')} out</dd>
					<dt>{t.modelCalls}</dt><dd class="num">{run.summary.llmCalls}</dd>
					<dt>{t.toolCalls}</dt><dd class="num">{run.summary.toolCalls}</dd>
					<dt>{t.blocked}</dt><dd class="num">{run.summary.blocked}</dd>
					{#if run.summary.masked}<dt>{t.masked}</dt><dd class="num">{run.summary.masked}</dd>{/if}
				</dl>
			</section>
			{#if run.limits}
				<section class="card">
					<h2 class="h3">{t.limits}</h2>
					<p class="cap">{t.limitsNote}</p>
					<div class="stack bars">
						{#each limits as l (l.label)}
							{@const p = pct(l.used, l.max)}
							<div>
								<div class="row between sm"><span>{l.label}</span><span class="mu num">{l.text}</span></div>
								<div class="bar" class:warn={p >= 80 && p < 100} class:full={p >= 100}><i style:width="{p}%"></i></div>
							</div>
						{:else}
							<p class="sm mu">{t.noLimits}</p>
						{/each}
					</div>
					{#if changed}<p class="cap changed">{t.limitsChanged(changed)}</p>{/if}
				</section>
			{/if}
		</aside>
	</div>
{/if}

<style>
	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 16px;
		flex-wrap: wrap;
	}
	.title {
		min-width: 0;
	}
	.title h1 {
		display: flex;
		align-items: center;
		gap: 12px;
		flex-wrap: wrap;
	}
	.crumb a {
		color: var(--mu);
	}
	.h3 {
		font-size: 16px;
		line-height: 24px;
	}
	.grow {
		flex: 1;
		min-width: 0;
	}
	.btn.off {
		opacity: 0.45;
		pointer-events: none;
	}
	.layout {
		display: grid;
		grid-template-columns: minmax(0, 1fr) 340px;
		gap: 16px;
		align-items: start;
	}
	.main {
		min-width: 0;
	}
	@media (max-width: 1100px) {
		.layout {
			grid-template-columns: minmax(0, 1fr);
		}
	}
	.text {
		margin-top: 8px;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
	}
	.code {
		margin: 8px 0 0;
		background: var(--code-bg);
		color: var(--code-tx);
		border-radius: 8px;
		padding: 12px 14px;
		font-family: var(--f-mono);
		font-size: 12px;
		line-height: 18px;
		white-space: pre-wrap;
		overflow-wrap: anywhere;
		max-height: 260px;
		overflow: auto;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 30px;
		padding: 0 12px;
		border-radius: 999px;
		border: 1px solid var(--bs);
		background: var(--sf);
		color: var(--tx);
		font-size: 12.5px;
		font-weight: 600;
		cursor: pointer;
	}
	.chip[aria-pressed='true'] {
		background: var(--prt);
		border-color: var(--pr);
		color: var(--pr);
	}
	.badge.live {
		background: var(--prt);
		color: var(--pr);
	}
	.badge.live .dot {
		animation: pulse 1.4s ease-in-out infinite;
	}
	@keyframes pulse {
		50% {
			opacity: 0.25;
		}
	}
	.tl-head {
		display: flex;
		justify-content: space-between;
		align-items: center;
		gap: 12px;
		flex-wrap: wrap;
		padding: 14px 16px;
	}
	.tl {
		list-style: none;
		margin: 0;
		padding: 0;
	}
	.ev {
		display: grid;
		grid-template-columns: 70px 22px minmax(0, 1fr);
		gap: 10px;
		padding: 10px 16px;
		border-top: 1px solid var(--bd);
		align-items: start;
		font-size: 13.5px;
		line-height: 20px;
	}
	.ev.blocked {
		background: var(--ert);
	}
	.ev.new {
		animation: fresh 2.4s ease-out;
	}
	@keyframes fresh {
		from {
			background: var(--prt);
		}
	}
	.ts {
		font-size: 12px;
		color: var(--mu);
	}
	.ic {
		width: 22px;
		height: 22px;
		border-radius: 6px;
		display: grid;
		place-items: center;
		background: var(--sk);
		color: var(--mu);
	}
	.ic.ok {
		background: var(--okt);
		color: var(--ok);
	}
	.ic.warn {
		background: var(--wat);
		color: var(--wa);
	}
	.ic.error {
		background: var(--ert);
		color: var(--er);
	}
	.ic.accent {
		background: var(--prt);
		color: var(--pr);
	}
	.de {
		min-width: 0;
	}
	.de.sub {
		padding-left: 12px;
		border-left: 2px solid var(--bd);
	}
	.l {
		display: flex;
		gap: 8px;
		align-items: baseline;
		flex-wrap: wrap;
	}
	.arg {
		font-size: 12.5px;
		background: var(--sk);
		border-radius: 6px;
		padding: 1px 8px;
		overflow-wrap: anywhere;
	}
	.d {
		font-size: 12.5px;
		color: var(--mu);
	}
	details {
		margin-top: 6px;
	}
	summary {
		cursor: pointer;
		font-size: 12.5px;
		color: var(--pr);
		font-weight: 600;
		width: max-content;
	}
	.pad {
		padding: 16px;
	}
	.foot {
		display: flex;
		align-items: center;
		gap: 8px;
		padding: 12px 16px;
		border-top: 1px solid var(--bd);
	}
	.kv {
		display: grid;
		grid-template-columns: auto minmax(0, 1fr);
		gap: 8px 16px;
		font-size: 13.5px;
		line-height: 20px;
		margin: 12px 0 0;
	}
	.kv dt {
		color: var(--mu);
	}
	.kv dd {
		margin: 0;
		font-weight: 600;
		text-align: right;
		overflow-wrap: anywhere;
	}
	.bars {
		margin-top: 12px;
		gap: 12px;
	}
	.between {
		justify-content: space-between;
	}
	.bar {
		height: 8px;
		border-radius: 999px;
		background: var(--sk);
		overflow: hidden;
		margin-top: 6px;
	}
	.bar i {
		display: block;
		height: 100%;
		background: var(--pr);
		border-radius: 999px;
	}
	.bar.warn i {
		background: var(--wa);
	}
	.bar.full i {
		background: var(--er);
	}
	.changed {
		margin-top: 12px;
	}
	.num {
		font-variant-numeric: tabular-nums;
	}
	@media (max-width: 640px) {
		.ev {
			grid-template-columns: 22px minmax(0, 1fr);
		}
		.ts {
			display: none;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.badge .dot,
		.ev.new {
			animation: none;
		}
	}
</style>

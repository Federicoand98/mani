<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { agentsApi, runsApi } from '#lib/api/client.ts';
	import type { AgentRef, Run } from '#lib/api/types.ts';
	import Icon from '#lib/components/Icon.svelte';
	import { m } from '#lib/messages/en.ts';
	import {
		apiQuery,
		DEFAULT_FILTERS,
		duration,
		filtersFrom,
		filtersQuery,
		groupRuns,
		journalOn,
		SOURCES,
		sourceLabel,
		tokens,
		when,
		type Filters
	} from '#lib/runs/runs.ts';
	import { toast } from '#lib/state.svelte.ts';

	const t = m.runs;
	const POLL_MS = 3000;

	const f = $derived(filtersFrom(page.url.searchParams));
	let runs = $state<Run[] | null>(null);
	let next = $state('');
	let unrecorded = $state<AgentRef[]>([]);
	let agentNames = $state<string[]>([]);
	let fresh = $state(new Set<string>());
	let open = $state(new Set<string>());
	let loadingMore = $state(false);
	let checkedAt = $state(0);
	let now = $state(Date.now());
	let visible = $state(true);
	let confirming = $state('');
	let error = $state('');

	const rows = $derived(groupRuns(runs ?? []));

	function setFilters(change: Partial<Filters>) {
		goto(`/runs${filtersQuery({ ...f, ...change })}`, { replaceState: true, reset: false });
	}

	async function load() {
		const query = apiQuery(f);
		try {
			const res = await runsApi.list(query);
			if (query !== apiQuery(f)) return; // the filters changed meanwhile
			runs = res.runs;
			next = res.next;
			unrecorded = res.unrecorded;
			fresh = new Set();
			checkedAt = Date.now();
			error = '';
		} catch (e) {
			error = (e as Error).message;
		}
	}

	// poll refreshes the newest page and keeps what "Load more" added below it.
	async function poll() {
		if (!runs) return;
		const query = apiQuery(f);
		try {
			const res = await runsApi.list(query);
			if (query !== apiQuery(f) || !runs) return;
			const ids = new Set(runs.map((r) => r.id));
			const oldest = res.runs.at(-1)?.startedAt ?? '';
			const kept = res.next ? runs.filter((r) => r.startedAt < oldest && !res.runs.some((x) => x.id === r.id)) : [];
			fresh = new Set(res.runs.filter((r) => !ids.has(r.id)).map((r) => r.id));
			runs = [...res.runs, ...kept];
			if (!kept.length) next = res.next;
			unrecorded = res.unrecorded;
			checkedAt = Date.now();
		} catch {
			// a missed poll is retried on the next tick
		}
	}

	async function loadMore() {
		loadingMore = true;
		try {
			const res = await runsApi.list(apiQuery(f, next));
			runs = [...(runs ?? []), ...res.runs.filter((r) => !runs?.some((x) => x.id === r.id))];
			next = res.next;
		} catch (e) {
			toast((e as Error).message);
		} finally {
			loadingMore = false;
		}
	}

	async function turnOn(a: AgentRef) {
		try {
			const file = await agentsApi.get(a.path);
			await agentsApi.save(a.path, { text: journalOn(file.text, a.name), version: file.version });
			toast(t.turnedOn(a.name));
			confirming = '';
			load();
		} catch (e) {
			toast((e as Error).message);
		}
	}

	function toggle(key: string) {
		const s = new Set(open);
		if (s.has(key)) s.delete(key);
		else s.add(key);
		open = s;
	}

	$effect(() => {
		page.url.search;
		runs = null;
		load();
	});

	onMount(() => {
		agentsApi.list().then((r) => (agentNames = [...new Set(r.agents.map((a) => a.name))].sort()), () => {});
		const onVis = () => {
			visible = document.visibilityState === 'visible';
			if (visible) poll();
		};
		document.addEventListener('visibilitychange', onVis);
		// Decision M5-5: every 3 s, only while the tab is visible.
		const id = setInterval(() => {
			now = Date.now();
			if (visible && now - checkedAt >= POLL_MS) poll();
		}, 1000);
		return () => {
			clearInterval(id);
			document.removeEventListener('visibilitychange', onVis);
		};
	});

	const filtered = $derived(f.status !== '' || f.agent !== '' || f.source !== '' || f.blocked || f.period !== DEFAULT_FILTERS.period);
	const statusClass = (s: string) => (s === 'ok' ? 'ok' : s === 'error' ? 'error' : s === 'running' ? 'live' : 'muted');
</script>

<svelte:head><title>{t.title} - mani</title></svelte:head>

{#snippet status(s: string)}
	<span class="badge {statusClass(s)}">
		{#if s === 'running'}<span class="dot"></span>{:else}<Icon name={s === 'ok' ? 'check' : s === 'error' ? 'x' : 'stop'} size={12} />{/if}
		{t.statuses[s] ?? s}
	</span>
{/snippet}

{#snippet runRow(r: Run, n: number)}
	<tr class="click" class:fresh={fresh.has(r.id)} class:child={n > 0} onclick={() => goto(`/runs/${r.id}`)}>
		<td>{@render status(r.status)}</td>
		<td>
			{#if n > 0}
				<span class="cap">{t.message(n)}</span>
			{:else if r.agent}
				<a class="who" href="/runs/{r.id}" onclick={(e) => e.stopPropagation()}><b>{r.agent}</b></a>
				{#if r.model}<div class="cap mono">{r.model}</div>{/if}
			{:else}
				<a class="who" href="/runs/{r.id}" onclick={(e) => e.stopPropagation()}><b class="mu">{t.unknownAgent}</b></a>
				<div class="cap">{t.beforeAgents}</div>
			{/if}
		</td>
		<td class="task">{#if r.task}{r.task}{:else}<span class="mu">{t.notRecorded}</span>{/if}</td>
		<td class="sm hide-s">{sourceLabel(r.source)}</td>
		<td class="sm num hide-s">{when(r.startedAt)}</td>
		<td class="sm num r hide-s">{duration(r.startedAt, r.endedAt, now)}</td>
		<td class="sm num r">{tokens(r).toLocaleString('en-US')}</td>
		<td class="sm num r hide-s">{#if r.summary.blocked}<span class="badge warn"><Icon name="lock" size={12} />{r.summary.blocked}</span>{:else}<span class="mu">0</span>{/if}</td>
	</tr>
{/snippet}

<div class="head">
	<div>
		<h1>{t.title}</h1>
		<p class="sm mu">{t.subtitle}</p>
	</div>
	{#if runs}
		<span class="badge local" aria-live="off"><span class="dot"></span>{visible ? t.checked(Math.round((now - checkedAt) / 1000)) : t.paused}</span>
	{/if}
</div>

{#if unrecorded.length && (runs?.length || filtered)}
	<div class="banner info">
		<Icon name="eye" />
		<div class="stack grow" style="gap:8px">
			<span><b>{t.unrecordedTitle(unrecorded.length)}</b> {t.unrecordedBody}</span>
			{@render offList()}
		</div>
	</div>
{/if}

{#snippet offList()}
	<div class="off">
		{#each unrecorded as a (a.path)}
			<div class="off-row">
				<span><b>{a.name}</b> <span class="cap mono">{a.path}</span></span>
				{#if confirming === a.path}
					<div class="confirm">
						<span class="sm">{t.turnOnAdds}</span>
						<pre class="snippet">observability:
  journal:
    enabled: true
    path: runs/{a.name}</pre>
						<span class="cap">{t.turnOnKeeps}</span>
						<div class="row" style="justify-content:flex-end">
							<button class="btn ghost small" onclick={() => (confirming = '')}>{t.cancel}</button>
							<button class="btn primary small" onclick={() => turnOn(a)}>{t.turnOn}</button>
						</div>
					</div>
				{:else}
					<button class="btn secondary small" onclick={() => (confirming = a.path)}>{t.turnOn}</button>
				{/if}
			</div>
		{/each}
	</div>
{/snippet}

<div class="row" role="group" aria-label="Filters">
	<div class="seg">
		{#each ['', 'running', 'ok', 'error', 'cancelled'] as s (s)}
			<button aria-pressed={f.status === s} onclick={() => setFilters({ status: s })}>{s ? t.statuses[s] : t.all}</button>
		{/each}
	</div>
	<label class="sr-only" for="f-agent">{t.colAgent}</label>
	<select id="f-agent" class="input auto" value={f.agent} onchange={(e) => setFilters({ agent: e.currentTarget.value })}>
		<option value="">{t.allAgents}</option>
		{#each agentNames as n (n)}<option value={n}>{n}</option>{/each}
		<option value="unknown">{t.unknownAgent}</option>
	</select>
	<label class="sr-only" for="f-source">{t.colSource}</label>
	<select id="f-source" class="input auto" value={f.source} onchange={(e) => setFilters({ source: e.currentTarget.value })}>
		<option value="">{t.anySource}</option>
		{#each Object.entries(SOURCES) as [k, label] (k)}<option value={k}>{label}</option>{/each}
	</select>
	<label class="sr-only" for="f-period">Period</label>
	<select id="f-period" class="input auto" value={f.period} onchange={(e) => setFilters({ period: e.currentTarget.value })}>
		{#each Object.entries(t.periods) as [k, label] (k)}<option value={k}>{label}</option>{/each}
	</select>
	<button class="chip" aria-pressed={f.blocked} onclick={() => setFilters({ blocked: !f.blocked })}><Icon name="lock" size={12} />{t.hadBlocked}</button>
</div>

{#if error}
	<div class="banner error" role="alert"><Icon name="warn" /><span>{error}</span></div>
{:else if runs === null}
	<div class="card stack" aria-busy="true"><span class="sr-only">{t.loading}</span><div class="skeleton" style="width:40%"></div><div class="skeleton"></div><div class="skeleton" style="width:70%"></div></div>
{:else if !runs.length}
	<div class="card">
		<div class="empty">
			<div class="icon"><Icon name="runs" size={24} /></div>
			{#if filtered}
				<h2 class="h3">{t.noMatchTitle}</h2>
				<p>{t.noMatchBody}</p>
				<button class="btn secondary" onclick={() => goto('/runs', { replaceState: true })}>{t.clear}</button>
			{:else if unrecorded.length}
				<h2 class="h3">{t.noAgentTitle}</h2>
				<p>{t.noAgentBody}</p>
				<div class="empty-list">{@render offList()}</div>
			{:else}
				<h2 class="h3">{t.emptyTitle}</h2>
				<p>{t.emptyBody}</p>
			{/if}
		</div>
	</div>
{:else}
	<div class="card flush">
		<div class="table-wrap">
			<table class="table">
				<thead>
					<tr>
						<th>{t.colStatus}</th><th>{t.colAgent}</th><th>{t.colTask}</th><th class="hide-s">{t.colSource}</th><th class="hide-s">{t.colStarted}</th>
						<th class="r hide-s">{t.colDuration}</th><th class="r">{t.colTokens}</th><th class="r hide-s">{t.colBlocked}</th>
					</tr>
				</thead>
				<tbody>
					{#each rows as row (row.kind === 'run' ? row.run.id : row.key)}
						{#if row.kind === 'run'}
							{@render runRow(row.run, 0)}
						{:else}
							<tr class="click group" class:fresh={row.runs.some((r) => fresh.has(r.id))} aria-expanded={open.has(row.key)} onclick={() => toggle(row.key)}>
								<td>{@render status(row.status)}</td>
								<td>
									<b>{row.agent || t.unknownAgent}</b>
									{#if row.model}<div class="cap mono">{row.model}</div>{/if}
								</td>
								<td class="task">
									<button class="expand" aria-expanded={open.has(row.key)} onclick={(e) => (e.stopPropagation(), toggle(row.key))}>
										<span class="chev"><Icon name="chevron" size={14} /></span>
										<b>{row.runs[0].title || row.runs.at(-1)?.task}</b>
										<span class="mu"> - {t.messages(row.runs.length)}</span>
									</button>
								</td>
								<td class="sm hide-s">{sourceLabel(row.source)}</td>
								<td class="sm num hide-s">{when(row.startedAt)} - {when(row.lastAt).split(' ').at(-1)}</td>
								<td class="sm num r hide-s"></td>
								<td class="sm num r">{row.tokens.toLocaleString('en-US')}</td>
								<td class="sm num r hide-s">{#if row.blocked}<span class="badge warn"><Icon name="lock" size={12} />{row.blocked}</span>{:else}<span class="mu">0</span>{/if}</td>
							</tr>
							{#if open.has(row.key)}
								{#each row.runs as r, i (r.id)}{@render runRow(r, row.runs.length - i)}{/each}
							{/if}
						{/if}
					{/each}
				</tbody>
			</table>
		</div>
		<div class="foot">
			<span class="sm mu">{t.shown(runs.length)}</span>
			{#if next}
				<button class="btn secondary small" disabled={loadingMore} onclick={loadMore}>{loadingMore ? t.loading : t.loadMore}</button>
			{:else}
				<span class="sm mu">{t.noMore}</span>
			{/if}
		</div>
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
	.h3 {
		font-size: 16px;
		line-height: 24px;
	}
	.grow {
		flex: 1;
		min-width: 0;
	}
	.input.auto {
		width: auto;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		gap: 6px;
		height: 32px;
		padding: 0 12px;
		border-radius: 999px;
		border: 1px solid var(--bs);
		background: var(--sf);
		color: var(--tx);
		font-size: 13px;
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
	.badge.live .dot,
	.badge.local .dot {
		animation: pulse 1.4s ease-in-out infinite;
	}
	@keyframes pulse {
		50% {
			opacity: 0.25;
		}
	}
	.off {
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.off-row {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 12px;
		flex-wrap: wrap;
	}
	.confirm {
		display: flex;
		flex-direction: column;
		gap: 6px;
		max-width: 420px;
	}
	.snippet {
		margin: 0;
		background: var(--code-bg);
		color: var(--code-tx);
		border-radius: 8px;
		padding: 10px 12px;
		font-family: var(--f-mono);
		font-size: 12.5px;
		line-height: 19px;
	}
	.empty-list {
		width: 100%;
		max-width: 520px;
		text-align: left;
		margin-top: 6px;
	}
	.table tr.click {
		cursor: pointer;
	}
	.table tr.click:hover td {
		background: var(--sk);
	}
	.table .task {
		max-width: 360px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.table .r {
		text-align: right;
	}
	.table tr.child td {
		background: var(--bg);
	}
	.table tr.child td.task {
		padding-left: 36px;
	}
	.table tr.fresh td {
		animation: fresh 2.4s ease-out;
	}
	@keyframes fresh {
		from {
			background: var(--prt);
		}
	}
	.who {
		color: inherit;
		text-decoration: none;
	}
	.expand {
		border: 0;
		background: none;
		padding: 0;
		font: inherit;
		color: inherit;
		cursor: pointer;
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		text-align: left;
	}
	.chev {
		display: inline-grid;
		vertical-align: -2px;
		margin-right: 4px;
		color: var(--mu);
		transition: transform 0.15s;
	}
	.expand[aria-expanded='true'] .chev {
		transform: rotate(90deg);
	}
	.foot {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 12px;
		padding: 12px 16px;
		border-top: 1px solid var(--bd);
		flex-wrap: wrap;
	}
	.num {
		font-variant-numeric: tabular-nums;
	}
	@media (max-width: 720px) {
		.hide-s {
			display: none;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.badge .dot,
		.table tr.fresh td,
		.chev {
			animation: none;
			transition: none;
		}
	}
</style>

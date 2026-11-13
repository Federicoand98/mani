<script lang="ts">
	import { agentsApi } from '#lib/api/client.ts';
	import type { AgentSummary } from '#lib/api/types.ts';
	import { copyAgent, NAME_RE, siblingPath } from '#lib/agents/files.ts';
	import Icon from '#lib/components/Icon.svelte';
	import { m } from '#lib/messages/en.ts';
	import { toast } from '#lib/state.svelte.ts';

	const t = m.agents;
	let agents = $state<AgentSummary[] | null>(null);
	let filter = $state('');
	let action = $state<{ kind: 'duplicate' | 'rename' | 'delete'; path: string } | null>(null);
	let name = $state('');
	let error = $state('');

	async function load() {
		try {
			agents = (await agentsApi.list()).agents;
		} catch (e) {
			toast((e as Error).message);
		}
	}
	load();

	const shown = $derived(
		(agents ?? []).filter((a) => !filter || `${a.name} ${a.path} ${a.description}`.toLowerCase().includes(filter.toLowerCase()))
	);

	function begin(kind: 'duplicate' | 'rename' | 'delete', a: AgentSummary) {
		action = { kind, path: a.path };
		name = kind === 'duplicate' ? `${a.name}-copy` : a.name;
		error = '';
	}

	async function confirm() {
		if (!action) return;
		const { kind, path } = action;
		try {
			if (kind === 'delete') {
				await agentsApi.remove(path);
				toast(t.deleted(path));
			} else {
				if (!NAME_RE.test(name)) {
					error = t.badName;
					return;
				}
				const to = siblingPath(path, name);
				if (agents?.some((a) => a.path === to)) {
					error = t.exists(to);
					return;
				}
				await copyAgent(path, name, kind === 'rename');
				toast(kind === 'rename' ? t.renamed(to) : t.duplicated(to));
			}
			action = null;
			load();
		} catch (e) {
			error = (e as Error).message;
		}
	}
</script>

<svelte:head><title>{t.title} - mani</title></svelte:head>

<div class="head">
	<div>
		<h1>{t.title}</h1>
		<p class="sm mu">{t.subtitle}</p>
	</div>
	<a class="btn primary" href="/agents/new"><Icon name="plus" size={16} />New agent</a>
</div>

{#if agents === null}
	<div class="card stack" aria-busy="true"><span class="sr-only">Loading</span><div class="skeleton" style="width:40%"></div><div class="skeleton"></div><div class="skeleton" style="width:70%"></div></div>
{:else if !agents.length}
	<div class="card">
		<div class="empty">
			<div class="icon"><Icon name="agent" size={24} /></div>
			<h2 class="h3">{t.emptyTitle}</h2>
			<p>{t.emptyBody}</p>
			<a class="btn primary" href="/agents/new"><Icon name="plus" size={16} />New agent</a>
		</div>
	</div>
{:else}
	<div class="row">
		<label class="sr-only" for="agent-filter">{t.filter}</label>
		<input id="agent-filter" class="input" style="max-width:360px" placeholder={t.filter} bind:value={filter} />
	</div>
	<div class="card flush table-wrap">
		<table class="table">
			<thead><tr><th>{t.colAgent}</th><th>{t.colModel}</th><th>{t.colTools}</th><th>{t.colStart}</th><th>{t.colStatus}</th><th><span class="sr-only">Actions</span></th></tr></thead>
			<tbody>
				{#each shown as a (a.path)}
					<tr>
						<td>
							<a class="name" href="/agents/{a.path}">{a.name}</a>
							{#if a.description}<div class="cap">{a.description}</div>{/if}
							<div class="cap mono">{a.path}</div>
						</td>
						<td><span class="sm">{a.provider || '—'}</span><div class="cap mono">{a.model}</div></td>
						<td class="sm">{a.tools}</td>
						<td class="sm">{a.triggers.length ? a.triggers.join(', ') : t.onDemand}</td>
						<td>
							{#if a.status === 'ok'}<span class="badge ok">{t.ok}</span>
							{:else if a.status === 'invalid'}<span class="badge error">{t.invalid}</span>
							{:else}<span class="badge warn">{t.problems(a.problems)}</span>{/if}
						</td>
						<td class="actions">
							{#if action?.path === a.path}
								<form class="act" onsubmit={(e) => (e.preventDefault(), confirm())}>
									{#if action.kind === 'delete'}
										<span class="sm">{t.confirmDelete(a.path)}</span>
									{:else}
										<label class="sr-only" for="act-name">{t.newName}</label>
										<!-- svelte-ignore a11y_autofocus -->
										<input id="act-name" class="input mono" bind:value={name} autofocus aria-describedby="act-help" />
										<span class="help" class:error={!!error} id="act-help">{error || t.nameHelp}</span>
									{/if}
									<div class="row">
										<button type="button" class="btn ghost small" onclick={() => (action = null)}>{t.cancel}</button>
										<button type="submit" class="btn small" class:danger={action.kind === 'delete'} class:primary={action.kind !== 'delete'}>{t[action.kind]}</button>
									</div>
								</form>
							{:else}
								<div class="row icons">
									<button class="icon-btn" title={t.duplicate} aria-label="{t.duplicate} {a.name}" onclick={() => begin('duplicate', a)}><Icon name="copy" size={16} /></button>
									<button class="icon-btn" title={t.rename} aria-label="{t.rename} {a.name}" onclick={() => begin('rename', a)}><Icon name="pencil" size={16} /></button>
									<button class="icon-btn danger" title={t.delete} aria-label="{t.delete} {a.name}" onclick={() => begin('delete', a)}><Icon name="trash" size={16} /></button>
								</div>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
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
	.name {
		font-weight: 700;
		text-decoration: none;
	}
	.actions .row {
		justify-content: flex-end;
		flex-wrap: nowrap;
	}
	.icons {
		gap: 2px;
	}
	.icon-btn {
		width: 32px;
		height: 32px;
		border-radius: 8px;
		border: 0;
		background: transparent;
		color: var(--mu);
		display: inline-grid;
		place-items: center;
		cursor: pointer;
	}
	.icon-btn:hover {
		background: var(--sk);
		color: var(--tx);
	}
	.icon-btn.danger:hover {
		background: var(--ert);
		color: var(--er);
	}
	.act {
		display: flex;
		flex-direction: column;
		gap: 6px;
		align-items: flex-end;
		min-width: 240px;
	}
</style>

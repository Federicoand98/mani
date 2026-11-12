<script lang="ts">
	import AddProviderDialog from '#lib/components/AddProviderDialog.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import { api, ApiError } from '#lib/api/client.ts';
	import type { Provider } from '#lib/api/types.ts';
	import { m } from '#lib/messages/en.ts';
	import { modelSummary } from '#lib/providers.ts';
	import { app, enabledProviders, loadProviders, toast } from '#lib/state.svelte.ts';

	const t = m.providers;
	let addOpen = $state(false);

	type Models = { state: 'loading' | 'ok' | 'error'; list: string[]; error?: string };
	let models = $state<Record<string, Models>>({});
	let tests = $state<Record<string, { ok: boolean; text: string } | 'running'>>({});
	let confirming = $state<string | null>(null);

	const enabled = $derived(enabledProviders());

	async function fetchModels(name: string) {
		models[name] = { state: 'loading', list: [] };
		try {
			models[name] = { state: 'ok', list: (await api.models(name)).models };
		} catch (e) {
			models[name] = { state: 'error', list: [], error: (e as Error).message };
		}
	}

	$effect(() => {
		for (const p of enabled) if (!(p.name in models)) fetchModels(p.name);
	});

	async function test(p: Provider) {
		tests[p.name] = 'running';
		try {
			const r = await api.testProvider(p.name, {});
			tests[p.name] = r.ok ? { ok: true, text: t.testOk(r.models.length) } : { ok: false, text: r.error ?? '' };
			if (r.ok) models[p.name] = { state: 'ok', list: r.models };
		} catch (e) {
			tests[p.name] = { ok: false, text: e instanceof ApiError && e.status === 0 ? m.errors.unreachable : (e as Error).message };
		}
	}

	async function remove(p: Provider) {
		confirming = null;
		try {
			await api.removeProvider(p.name);
			delete models[p.name];
			delete tests[p.name];
			await loadProviders();
			toast(t.removed(p.label));
		} catch (e) {
			toast((e as Error).message);
		}
	}

	async function added(p: Provider) {
		await loadProviders();
		toast(t.added(p.label));
	}
</script>

<svelte:head><title>{t.title} · mani</title></svelte:head>

<div class="head">
	<div>
		<h1>{t.title}</h1>
		<p class="sm mu">{t.subtitle}</p>
	</div>
	{#if enabled.length}
		<button class="btn primary" onclick={() => (addOpen = true)}><Icon name="plus" size={16} />{t.add}</button>
	{/if}
</div>

{#if !app.providersLoaded}
	<div class="card stack" aria-busy="true">
		<span class="sr-only">Loading</span>
		<div class="skeleton" style="width:40%"></div>
		<div class="skeleton"></div>
		<div class="skeleton" style="width:70%"></div>
	</div>
{:else if !enabled.length}
	<div class="card">
		<div class="empty">
			<div class="icon"><Icon name="cube" size={24} /></div>
			<h3>{t.emptyTitle}</h3>
			<p>{t.emptyBody}</p>
			<button class="btn primary" onclick={() => (addOpen = true)}><Icon name="plus" size={16} />{t.add}</button>
		</div>
	</div>
{:else}
	<div class="card flush table-wrap">
		<table class="table">
			<thead>
				<tr><th>{t.colProvider}</th><th>{t.colWhere}</th><th>{t.colModels}</th><th><span class="sr-only">Actions</span></th></tr>
			</thead>
			<tbody>
				{#each enabled as p (p.name)}
					{@const ms = models[p.name]}
					{@const tr = tests[p.name]}
					<tr>
						<td>
							<b>{p.label}</b>
							<div class="cap mono">{p.keyHint ? `key ${p.keyHint}` : p.baseUrl}</div>
						</td>
						<td>
							{#if p.local}
								<span class="badge local"><span class="dot"></span>{t.local}</span>
							{:else}
								<span class="badge warn"><Icon name="net" size={12} />{t.cloud}</span>
							{/if}
						</td>
						<td class="sm">
							{#if !ms || ms.state === 'loading'}
								<div class="skeleton" style="width:140px"></div>
							{:else if ms.state === 'error'}
								<span class="badge error" title={ms.error}>{ms.error}</span>
							{:else}
								{@const s = modelSummary(ms.list)}
								{t.models(ms.list.length)}
								{#each s.first as name (name)} <code>{name}</code>{/each}
								{#if s.more}<span class="cap">{t.more(s.more)}</span>{/if}
							{/if}
						</td>
						<td class="actions">
							<div class="row">
								{#if confirming === p.name}
									<button class="btn ghost small" onclick={() => (confirming = null)}>{t.cancel}</button>
									<button class="btn danger small" onclick={() => remove(p)}>{t.confirmRemove}</button>
								{:else}
									<button class="btn ghost small" disabled={tr === 'running'} onclick={() => test(p)}>{tr === 'running' ? t.testing : t.test}</button>
									<button class="btn ghost small" onclick={() => (confirming = p.name)}>{t.remove}</button>
								{/if}
							</div>
							{#if tr && tr !== 'running'}
								<p class="help" class:error={!tr.ok} class:ok={tr.ok} role="status">{tr.text}</p>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
{/if}

<AddProviderDialog bind:open={addOpen} providers={app.providers} onadded={added} />

<style>
	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 16px;
		flex-wrap: wrap;
	}
	.actions .row {
		justify-content: flex-end;
		flex-wrap: nowrap;
	}
	.actions .help {
		text-align: right;
		max-width: 320px;
		margin-left: auto;
		overflow-wrap: anywhere;
	}
	.help.ok {
		color: var(--ok);
		font-weight: 600;
	}
	td .badge.error {
		white-space: normal;
		max-width: 320px;
	}
</style>

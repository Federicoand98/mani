<script lang="ts">
	import { beforeNavigate } from '$app/navigation';
	import { page } from '$app/state';
	import { onMount, tick } from 'svelte';
	import type { Problem } from '#lib/api/types.ts';
	import { BLOCKS, blockOf, problemsIn, type BlockKey } from '#lib/agents/blocks.ts';
	import { AgentEditor } from '#lib/agents/editor.svelte.ts';
	import Icon from '#lib/components/Icon.svelte';
	import BlockCapabilities from '#lib/components/editor/BlockCapabilities.svelte';
	import BlockContext from '#lib/components/editor/BlockContext.svelte';
	import BlockIdentity from '#lib/components/editor/BlockIdentity.svelte';
	import BlockLimits from '#lib/components/editor/BlockLimits.svelte';
	import BlockObservability from '#lib/components/editor/BlockObservability.svelte';
	import BlockOutput from '#lib/components/editor/BlockOutput.svelte';
	import BlockPolicy from '#lib/components/editor/BlockPolicy.svelte';
	import BlockRun from '#lib/components/editor/BlockRun.svelte';
	import YamlPanel from '#lib/components/editor/YamlPanel.svelte';
	import { m } from '#lib/messages/en.ts';
	import { toast } from '#lib/state.svelte.ts';
	import { read, write } from '#lib/storage.ts';

	const t = m.editor;
	const path = page.params.path ?? '';
	const ed = new AgentEditor(path);
	let block = $state<BlockKey>('identity');
	let yaml: { gotoLine(n: number): void } | undefined = $state();
	// Form or YAML, one at a time and full width; the choice is remembered.
	let view = $state<'form' | 'yaml'>(read('mani:editor-view') === 'yaml' ? 'yaml' : 'form');
	let pendingLine = $state(0);

	function setView(v: 'form' | 'yaml') {
		view = v;
		write('mani:editor-view', v === 'yaml' ? 'yaml' : null);
	}

	// The YAML panel loads CodeMirror lazily: a line asked for before it is ready waits here.
	$effect(() => {
		if (yaml && pendingLine) {
			yaml.gotoLine(pendingLine);
			pendingLine = 0;
		}
	});

	onMount(() => {
		ed.load();
		const stop = ed.watch();
		const keys = (e: KeyboardEvent) => {
			if ((e.ctrlKey || e.metaKey) && e.key === 's') {
				e.preventDefault();
				save();
			}
		};
		const unload = (e: BeforeUnloadEvent) => {
			if (ed.dirty) e.preventDefault();
		};
		window.addEventListener('keydown', keys);
		window.addEventListener('beforeunload', unload);
		return () => {
			stop();
			window.removeEventListener('keydown', keys);
			window.removeEventListener('beforeunload', unload);
		};
	});

	beforeNavigate((nav) => {
		if (ed.dirty && !confirm(t.leave)) nav.cancel();
	});

	// A file reloaded from disk while clean: say so, quietly.
	let lastVersion = '';
	$effect(() => {
		const v = ed.version;
		if (lastVersion && v !== lastVersion && !ed.saving && !ed.dirty) toast(t.reloadedExternal);
		lastVersion = v;
	});

	async function save(force = false) {
		try {
			lastVersion = '';
			if (await ed.save(force)) toast(t.saved);
		} catch (e) {
			toast((e as Error).message);
		}
	}

	async function goto(p: Problem) {
		const b = blockOf(p);
		if (view === 'yaml' || !b) {
			setView('yaml');
			pendingLine = p.line || 1;
			return;
		}
		block = b;
		await tick();
		const target = findField(p.path);
		if (target) {
			target.scrollIntoView({ block: 'center' });
			(target.matches('input,select,textarea,button') ? target : target.querySelector<HTMLElement>('input,select,textarea,button'))?.focus();
		}
	}

	/** The field for a path, or the closest enclosing one. */
	function findField(p: string): HTMLElement | null {
		let key = p;
		while (key) {
			const el = document.querySelector<HTMLElement>(`[data-path="${CSS.escape(key)}"]`);
			if (el) return el;
			key = key.replace(/(\[\d+\]|\.[^.[]+)$/, '');
		}
		return null;
	}

	function showYaml(fieldPath: string) {
		const p = ed.problems.find((x) => x.path.startsWith(fieldPath));
		const line = p?.line ?? ed.text.split('\n').findIndex((l) => l.trimStart().startsWith(fieldPath.split('.').at(-1) + ':')) + 1;
		setView('yaml');
		pendingLine = Math.max(line, 1);
	}

	const name = $derived(ed.str(['identity', 'name']) || path.split('/').at(-1));
</script>

<svelte:head><title>{name} - mani</title></svelte:head>

{#if ed.error}
	<div class="card">
		<div class="empty">
			<div class="icon"><Icon name="warn" size={24} /></div>
			<h1 class="h3">{t.notFound}</h1>
			<p class="mono">{path}</p>
			<a class="btn primary" href="/agents">{t.back}</a>
		</div>
	</div>
{:else}
	<header class="head">
		<div class="min0">
			<div class="cap crumb"><a href="/agents">Agents</a> / <span class="mono">{path}</span></div>
			<h1>{name}</h1>
		</div>
		<div class="row">
			{#if ed.dirty}<span class="badge warn">{t.unsaved}</span>{/if}
			<button class="btn secondary" aria-pressed={view === 'yaml'} onclick={() => setView(view === 'yaml' ? 'form' : 'yaml')}>
				<Icon name={view === 'yaml' ? 'form' : 'code'} size={16} />{view === 'yaml' ? t.editAsForm : t.editAsYaml}
			</button>
			<button class="btn primary" disabled={!ed.dirty || ed.saving} onclick={() => save()}>{ed.saving ? t.saving : t.save}</button>
		</div>
	</header>

	{#if ed.conflict}
		<div class="banner error" role="alert">
			<Icon name="warn" />
			<div class="grow"><b>{t.conflictTitle}</b></div>
			<button class="btn secondary small" onclick={() => ed.reload()}>{t.reload}</button>
			<button class="btn danger small" onclick={() => save(true)}>{t.overwrite}</button>
		</div>
	{:else if ed.external}
		<div class="banner info" role="alert">
			<Icon name="warn" />
			<div class="grow"><b>{t.externalTitle}</b> {t.externalBody}</div>
			<button class="btn secondary small" onclick={() => ed.reload()}>{t.reload}</button>
		</div>
	{/if}

	{#if ed.loading}
		<div class="card stack" aria-busy="true"><div class="skeleton" style="width:40%"></div><div class="skeleton"></div><div class="skeleton" style="width:60%"></div></div>
	{:else}
		<div class="check" class:ok={!ed.problems.length} role="status">
			{#if ed.problems.length}
				<b>{t.problemCount(ed.problems.length)}</b>
				<ul>
					{#each ed.problems as p, i (i)}
						<li>
							<span>{#if p.line}<b>{t.line(p.line)}.</b> {/if}{p.message}</span>
							<button class="btn ghost small" onclick={() => goto(p)}>{view === 'yaml' || !blockOf(p) ? t.goToLine : t.goToField}</button>
						</li>
					{/each}
				</ul>
			{:else}
				<span class="mono">{t.ok(path.split('/').at(-1) ?? path)}</span>
			{/if}
		</div>

		{#if view === 'yaml'}
			<div class="yaml-view">
				<YamlPanel text={ed.text} problems={ed.problems} onchange={(x) => ed.setText(x)} bind:ref={yaml} />
			</div>
		{:else}
			<div class="editor">
				<nav class="blocks" aria-label={t.blocks}>
					{#each BLOCKS as b, i (b.key)}
						{@const n = problemsIn(ed.problems, b.key).length}
						<button class="blk" aria-current={block === b.key ? 'true' : undefined} onclick={() => (block = b.key)}>
							<span class="n" class:error={n > 0}>{n > 0 ? '!' : i + 1}</span>
							<span class="min0"><b>{b.title}</b><span>{b.hint}</span></span>
						</button>
					{/each}
				</nav>

				<section class="card form" aria-label={BLOCKS.find((b) => b.key === block)?.title}>
					<div>
						<h2>{BLOCKS.find((b) => b.key === block)?.title}</h2>
						<span class="cap">{t.inFile}: <span class="mono">{block}</span></span>
					</div>
					{#if !ed.parses}
						<div class="banner error"><Icon name="warn" /><span>{t.noParse}</span><button class="btn secondary small" onclick={() => setView('yaml')}>{t.editAsYaml}</button></div>
					{:else if block === 'identity'}<BlockIdentity {ed} />
					{:else if block === 'capabilities'}<BlockCapabilities {ed} />
					{:else if block === 'context'}<BlockContext {ed} />
					{:else if block === 'output'}<BlockOutput {ed} onyaml={showYaml} />
					{:else if block === 'policy'}<BlockPolicy {ed} />
					{:else if block === 'limits'}<BlockLimits {ed} />
					{:else if block === 'run'}<BlockRun {ed} />
					{:else}<BlockObservability {ed} />{/if}
				</section>
			</div>
		{/if}
	{/if}
{/if}

<style>
	.h3 {
		font-size: 16px;
		line-height: 24px;
	}
	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 16px;
		flex-wrap: wrap;
	}
	.crumb a {
		color: var(--mu);
	}
	.min0 {
		min-width: 0;
	}
	.grow {
		flex: 1;
	}
	.banner {
		align-items: center;
	}
	.editor {
		display: grid;
		grid-template-columns: 220px minmax(0, 880px);
		gap: 16px;
		align-items: start;
	}
	.yaml-view {
		height: calc(100dvh - 300px);
		min-height: 480px;
	}
	.blocks {
		display: flex;
		flex-direction: column;
		gap: 2px;
		position: sticky;
		top: 88px;
	}
	.blk {
		display: flex;
		align-items: flex-start;
		gap: 10px;
		padding: 10px;
		border-radius: 8px;
		border: 0;
		background: none;
		text-align: left;
		cursor: pointer;
	}
	.blk:hover {
		background: var(--sk);
	}
	.blk[aria-current='true'] {
		background: var(--prt);
	}
	.blk .n {
		flex: none;
		width: 22px;
		height: 22px;
		border-radius: 50%;
		display: grid;
		place-items: center;
		font-size: 11px;
		font-weight: 800;
		background: var(--sk);
		color: var(--mu);
		margin-top: 1px;
	}
	.blk .n.error {
		background: var(--ert);
		color: var(--er);
	}
	.blk b {
		display: block;
		font-size: 14px;
		line-height: 20px;
	}
	.blk span span {
		display: block;
		font-size: 12px;
		line-height: 16px;
		color: var(--mu);
	}
	.form {
		display: flex;
		flex-direction: column;
		gap: 20px;
	}
	.check {
		border: 1px solid var(--er);
		background: var(--ert);
		border-radius: 10px;
		padding: 10px 12px;
		font-size: 13px;
		line-height: 18px;
		max-height: 180px;
		overflow: auto;
	}
	.check.ok {
		border-color: var(--ok);
		background: var(--okt);
	}
	.check ul {
		list-style: none;
		margin: 6px 0 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.check li {
		display: flex;
		gap: 8px;
		align-items: flex-start;
		justify-content: space-between;
		overflow-wrap: anywhere;
	}
	.check li .btn {
		flex: none;
	}
	@media (max-width: 1023px) {
		.editor {
			grid-template-columns: minmax(0, 1fr);
		}
		.blocks {
			position: static;
			flex-direction: row;
			overflow-x: auto;
		}
		.blk {
			min-width: 160px;
		}
	}
</style>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { agentsApi } from '#lib/api/client.ts';
	import type { Problem, Template } from '#lib/api/types.ts';
	import { Manifest } from '#lib/agents/doc.ts';
	import { ManifestState } from '#lib/agents/editor.svelte.ts';
	import { RISK_CLASS, TOOL_INFO } from '#lib/agents/tools.ts';
	import { loadDraft, saveDraft, STEPS, stepBlockers, stepFor, targetPath, type Draft } from '#lib/agents/wizard.ts';
	import BlockCapabilities from '#lib/components/editor/BlockCapabilities.svelte';
	import BlockIdentity from '#lib/components/editor/BlockIdentity.svelte';
	import BlockLimits from '#lib/components/editor/BlockLimits.svelte';
	import BlockPolicy from '#lib/components/editor/BlockPolicy.svelte';
	import FolderPicker from '#lib/components/editor/FolderPicker.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import TrialChat from '#lib/components/TrialChat.svelte';
	import { app, enabledProviders, toast } from '#lib/state.svelte.ts';

	let templates = $state<Template[]>([]);
	let existing = $state<string[]>([]);
	let defaultFolder = $state('.');
	let template = $state('');
	let step = $state(0);
	let folder = $state('.');
	let tried = $state(false);
	let restored = $state(false);
	let picking = $state(false);
	let saving = $state(false);
	let saveError = $state('');
	let saved = $state<string | null>(null);
	let importing = $state(false);
	let importText = $state('');
	let importProblems = $state<Problem[] | null>(null);
	let st = $state(new ManifestState(''));

	const name = $derived(st.str(['identity', 'name']));
	const path = $derived(targetPath(folder, name || 'new-agent'));
	const providers = $derived(enabledProviders().map((p) => p.name));
	const blockers = $derived((void st.rev, stepBlockers(step, st.text, folder, existing, providers)));

	onMount(async () => {
		try {
			const [t, a, f] = await Promise.all([agentsApi.templates(), agentsApi.list(), agentsApi.folders('.')]);
			templates = t.templates;
			existing = a.agents.map((x) => x.path);
			defaultFolder = f.folders.includes('agents') ? 'agents' : '.';
		} catch (e) {
			toast((e as Error).message);
		}
		const d = loadDraft();
		if (d) {
			template = d.template;
			folder = d.folder;
			tried = d.tried;
			st = new ManifestState(targetPath(d.folder, new Manifest(d.text).str(['identity', 'name']) || 'new-agent'), d.text);
			step = d.step;
			restored = true;
		} else folder = defaultFolder;
	});

	// The draft follows every change, so leaving and coming back resumes it (M4-6).
	$effect(() => {
		void st.rev;
		if (template && !saved) saveDraft({ template, text: st.text, step, folder, tried } satisfies Draft);
	});

	// Problems are checked against the folder the file will land in.
	$effect(() => {
		st.path = path;
	});

	function pick(t: Template) {
		template = t.name;
		folder = defaultFolder;
		tried = false;
		st = new ManifestState(targetPath(folder, new Manifest(t.text).str(['identity', 'name'])), t.text);
		step = 1;
		restored = false;
	}

	function startOver() {
		saveDraft(null);
		template = '';
		step = 0;
		tried = false;
		restored = false;
		importing = false;
		st = new ManifestState('');
	}

	function next() {
		if (blockers.length) return;
		step = Math.min(step + 1, STEPS.length - 1);
		if (step === 5) st.check();
	}

	async function save() {
		saving = true;
		saveError = '';
		try {
			await agentsApi.create(path, st.text);
			saveDraft(null);
			saved = path;
		} catch (e) {
			saveError = (e as Error).message;
		} finally {
			saving = false;
		}
	}

	const templateTools = (t: Template) => new Manifest(t.text).toolNames().filter(Boolean);

	// --- import (M4-7) ---
	async function readImport(e: Event) {
		const f = (e.currentTarget as HTMLInputElement).files?.[0];
		if (f) importText = await f.text();
		checkImport();
	}

	async function checkImport() {
		const n = new Manifest(importText).str(['identity', 'name']) || 'imported';
		try {
			importProblems = (await agentsApi.check(importText, targetPath(folder, n))).problems;
		} catch (e) {
			importProblems = [{ path: '', line: 0, message: (e as Error).message }];
		}
	}

	async function saveImport() {
		const n = new Manifest(importText).str(['identity', 'name']);
		if (!n) {
			saveError = 'The file has no identity.name: it names the file. Add one and try again.';
			return;
		}
		const to = targetPath(folder, n);
		try {
			await agentsApi.create(to, importText);
			saveDraft(null);
			goto(`/agents/${to}`);
		} catch (e) {
			saveError = (e as Error).message;
		}
	}
</script>

<svelte:head><title>New agent - mani</title></svelte:head>

<header class="head">
	<div>
		<div class="cap"><a href="/agents">Agents</a> / new</div>
		<h1>New agent</h1>
		<p class="sm mu">A few steps with careful defaults. The blocks left out here are in the editor once the agent is saved.</p>
	</div>
	{#if template || importing}<button class="btn ghost" onclick={startOver}>Start over</button>{/if}
</header>

{#if saved}
	<section class="card done">
		<div class="icon-ok"><Icon name="check" size={24} /></div>
		<h2>{tried ? 'Your agent works' : 'Your agent is saved'}</h2>
		<p class="sm mu">Saved as <code>{saved}</code>. {tried ? 'The trial ended without errors.' : 'Try it in the chat whenever you like.'}</p>
		<div class="row">
			<a class="btn primary" href="/chat?agent={encodeURIComponent(saved)}"><Icon name="chat" size={16} />Open in chat</a>
			<a class="btn secondary" href="/agents/{saved}">Open in the editor</a>
			<a class="btn secondary" href="/agents/{saved}?block=run">Schedule it</a>
		</div>
	</section>
{:else}
	<ol class="steps" aria-label="Progress">
		{#each STEPS as s, i (s)}
			<li class:done={i < step} class:cur={i === step} aria-current={i === step ? 'step' : undefined}>{i ? `${i}. ` : ''}{s}</li>
		{/each}
	</ol>

	{#if restored && template}
		<div class="banner info"><Icon name="flow" /><span>Your draft is back where you left it.</span><button class="btn ghost small" onclick={startOver}>Start over</button></div>
	{/if}

	{#if importing}
		<section class="card stack">
			<h2>Import YAML</h2>
			<p class="sm mu">Paste a manifest or choose a file. It is checked, then saved as it is, comments included, and opened in the editor.</p>
			<div class="field">
				<label for="imp-file">File</label>
				<input id="imp-file" type="file" accept=".yaml,.yml" class="input" style="padding-top:7px" onchange={readImport} />
			</div>
			<div class="field">
				<label for="imp-text">Or paste it</label>
				<textarea id="imp-text" class="input mono" rows="12" bind:value={importText} onchange={checkImport}></textarea>
			</div>
			<div class="row">
				<span class="label">Save in</span><code>{targetPath(folder, new Manifest(importText).str(['identity', 'name']) || '<name>')}</code>
				<button class="btn ghost small" onclick={() => (picking = true)}>Change folder</button>
			</div>
			{#if importProblems?.length}
				<div class="check bad"><b>{importProblems.length} problem{importProblems.length === 1 ? '' : 's'}</b><ul>{#each importProblems as p, i (i)}<li>{#if p.line}<b>Line {p.line}.</b> {/if}{p.message}</li>{/each}</ul><span class="cap">You can still save it and fix it in the editor.</span></div>
			{:else if importProblems}
				<div class="check ok">The manifest is valid.</div>
			{/if}
			{#if saveError}<p class="help error" role="alert">{saveError}</p>{/if}
			<div class="foot">
				<button class="btn secondary" onclick={startOver}><Icon name="back" size={16} />Back</button>
				<button class="btn primary" disabled={!importText.trim()} onclick={saveImport}>Save and open</button>
			</div>
		</section>
	{:else if step === 0}
		<div class="templates" role="group" aria-label="Start from">
			{#each templates as t (t.name)}
				<button class="tpl" onclick={() => pick(t)}>
					<b>{t.title}</b>
					<span class="sm mu">{t.about}</span>
					<span class="row tools">
						{#each templateTools(t) as tool (tool)}<span class="badge {RISK_CLASS[TOOL_INFO[tool]?.risk ?? 'none']}">{tool}</span>{:else}<span class="cap">no tools</span>{/each}
					</span>
				</button>
			{/each}
			<button class="tpl" onclick={() => ((importing = true), (folder = defaultFolder))}>
				<b>Import YAML</b>
				<span class="sm mu">You already have a manifest: paste it or pick the file.</span>
			</button>
		</div>
	{:else}
		<section class="card form">
			<div>
				<h2>{STEPS[step]}</h2>
				{#if step === 1}<p class="sm mu">What it is called and which model thinks for it.</p>
				{:else if step === 2}<p class="sm mu">What it can use besides the model. Only what it needs: the next step decides what it may do alone.</p>
				{:else if step === 3}<p class="sm mu">For each tool: alone, after asking you, or never.</p>
				{:else if step === 4}<p class="sm mu">The first limit reached stops a run. They are a ceiling, not a target.</p>
				{:else if step === 5}<p class="sm mu">The file as it will be saved, and what mani thinks of it.</p>
				{:else}<p class="sm mu">A real conversation with the agent before it exists as a file.</p>{/if}
			</div>

			{#if step === 1}<BlockIdentity ed={st} />
			{:else if step === 2}<BlockCapabilities ed={st} />
			{:else if step === 3}<BlockPolicy ed={st} />
			{:else if step === 4}<BlockLimits ed={st} />
			{:else if step === 5}
				<div class="row">
					<span class="label">Save as</span><code>{path}</code>
					<button class="btn ghost small" onclick={() => (picking = true)}>Change folder</button>
				</div>
				{#if existing.includes(path)}<p class="help error">{path} already exists: go back and pick another name.</p>{/if}
				{#if st.problems.length}
					<div class="check bad">
						<b>{st.problems.length} problem{st.problems.length === 1 ? '' : 's'}</b>
						<ul>
							{#each st.problems as p, i (i)}
								<li><span>{#if p.line}<b>Line {p.line}.</b> {/if}{p.message}</span>{#if stepFor(p) < 5}<button class="btn ghost small" onclick={() => (step = stepFor(p))}>Fix</button>{/if}</li>
							{/each}
						</ul>
					</div>
				{:else}
					<div class="check ok">The manifest is valid.</div>
				{/if}
				<pre class="yaml">{st.text}</pre>
			{:else}
				<TrialChat text={st.text} {folder} who={name} onworked={() => (tried = true)} />
				{#if tried}
					<div class="banner ok"><Icon name="check" /><span><b>Your agent works.</b> Save it to keep it.</span></div>
				{/if}
			{/if}

			{#if blockers.length}
				<ul class="blockers" role="status">{#each blockers as b (b)}<li>{b}</li>{/each}</ul>
			{/if}
			{#if saveError}<p class="help error" role="alert">{saveError}</p>{/if}

			<div class="foot">
				<button class="btn secondary" onclick={() => (step = Math.max(0, step - 1))}><Icon name="back" size={16} />Back</button>
				<div class="row">
					{#if step === 5}
						<button class="btn secondary" disabled={saving || existing.includes(path) || st.problems.length > 0} onclick={save}>Save without trying</button>
						<button class="btn primary" disabled={st.problems.length > 0 || existing.includes(path)} onclick={() => (step = 6)}>Try it</button>
					{:else if step === 6}
						<button class="btn primary" disabled={saving || existing.includes(path)} onclick={save}>{saving ? 'Saving…' : 'Save'}</button>
					{:else}
						<button class="btn primary" disabled={blockers.length > 0} onclick={next}>Continue</button>
					{/if}
				</div>
			</div>
		</section>
	{/if}
{/if}

<FolderPicker bind:open={picking} current={folder} onpick={(p) => (folder = p === '.' ? '.' : p.replace(/^\.\//, ''))} />

{#if !app.providersLoaded}<span class="sr-only">Loading</span>{/if}

<style>
	.head {
		display: flex;
		align-items: flex-end;
		justify-content: space-between;
		gap: 16px;
		flex-wrap: wrap;
	}
	.head a {
		color: var(--mu);
	}
	.steps {
		display: flex;
		gap: 4px;
		list-style: none;
		padding: 0;
		margin: 0;
		overflow-x: auto;
	}
	.steps li {
		flex: 1;
		min-width: 110px;
		display: flex;
		flex-direction: column;
		gap: 6px;
		font-size: 13px;
		font-weight: 600;
		color: var(--mu);
	}
	.steps li::before {
		content: '';
		height: 4px;
		border-radius: 4px;
		background: var(--bd);
	}
	.steps li.done::before {
		background: var(--ok);
	}
	.steps li.cur::before {
		background: var(--pr);
	}
	.steps li.cur {
		color: var(--pr);
	}
	.steps li.done {
		color: var(--tx);
	}
	.templates {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: 12px;
	}
	.tpl {
		display: flex;
		flex-direction: column;
		gap: 10px;
		text-align: left;
		padding: 18px;
		border-radius: var(--r-card);
		border: 1px solid var(--bd);
		background: var(--sf);
		cursor: pointer;
	}
	.tpl:hover {
		border-color: var(--pr);
	}
	.tools {
		gap: 4px;
	}
	.form {
		display: flex;
		flex-direction: column;
		gap: 20px;
		max-width: 880px;
	}
	.foot {
		display: flex;
		justify-content: space-between;
		gap: 12px;
		flex-wrap: wrap;
		border-top: 1px solid var(--bd);
		padding-top: 16px;
	}
	.blockers {
		margin: 0;
		padding: 10px 14px 10px 30px;
		border-radius: 10px;
		background: var(--wat);
		font-size: 14px;
	}
	.check {
		border-radius: 10px;
		padding: 10px 12px;
		font-size: 13px;
		border: 1px solid;
	}
	.check.ok {
		border-color: var(--ok);
		background: var(--okt);
	}
	.check.bad {
		border-color: var(--er);
		background: var(--ert);
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
		justify-content: space-between;
		gap: 8px;
		overflow-wrap: anywhere;
	}
	.yaml {
		margin: 0;
		background: var(--code-bg);
		color: var(--code-tx);
		border-radius: 10px;
		padding: 16px;
		font-family: var(--f-mono);
		font-size: 12.5px;
		line-height: 20px;
		max-height: 420px;
		overflow: auto;
	}
	.done {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 12px;
		max-width: 720px;
	}
	.icon-ok {
		width: 48px;
		height: 48px;
		border-radius: 12px;
		background: var(--okt);
		color: var(--ok);
		display: grid;
		place-items: center;
	}
	@media (max-width: 900px) {
		.templates {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>

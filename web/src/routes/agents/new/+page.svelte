<script lang="ts">
	import { goto } from '$app/navigation';
	import { onDestroy, onMount } from 'svelte';
	import { agentsApi } from '#lib/api/client.ts';
	import type { Problem, Template } from '#lib/api/types.ts';
	import { Manifest } from '#lib/agents/doc.ts';
	import { ManifestState } from '#lib/agents/editor.svelte.ts';
	import { MAIN_SECTIONS, ADVANCED_SECTIONS, creationBlockers, loadDraft, saveDraft, sectionFor, targetPath, type SetupSection } from '#lib/agents/wizard.ts';
	import { RISK_CLASS, TOOL_INFO } from '#lib/agents/tools.ts';
	import AgentSetup from '#lib/components/AgentSetup.svelte';
	import FolderPicker from '#lib/components/editor/FolderPicker.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import ResizablePanels from '#lib/components/ResizablePanels.svelte';
	import TrialChat from '#lib/components/TrialChat.svelte';
	import { enabledProviders } from '#lib/state.svelte.ts';

	let templates = $state<Template[]>([]);
	let existing = $state<string[]>([]);
	let defaultFolder = '.';
	let template = $state('');
	let section = $state<SetupSection>('agent');
	let advanced = $state(false);
	let folder = $state('.');
	let filename = $state<string | undefined>();
	let restored = $state(false);
	let loaded = $state(false);
	let loadError = $state('');
	let picking = $state(false);
	let saving = $state(false);
	let saveError = $state('');
	let saved = $state<string | null>(null);
	let savedWorked = $state(false);
	let importing = $state(false);
	let importText = $state('');
	let importProblems = $state<Problem[] | null>(null);
	let fieldErrors = $state<string[]>([]);
	let st = $state(new ManifestState(''));
	let preview = $state<'trial' | 'yaml'>('trial');
	let trial = $state<{ text: string; folder: string; epoch: number } | null>(null);
	let trialBusy = $state(false);
	let starting = $state(false);
	let workedText = $state('');
	let workedFolder = $state('');
	let epoch = 0;

	const name = $derived(st.str(['identity', 'name']));
	const path = $derived(targetPath(folder, name || 'new-agent', filename));
	const providers = $derived(enabledProviders().map((p) => p.name));
	const blockers = $derived((void st.rev, creationBlockers(st.text, folder, existing, providers, filename)));
	const invalid = $derived(blockers.length > 0 || fieldErrors.length > 0 || st.problems.length > 0 || !!st.checkError);
	const staleTrial = $derived(trial !== null && (trial.text !== st.text || trial.folder !== folder));
	const worked = $derived(workedText !== '' && workedText === st.text && workedFolder === folder);
	const primary = $derived(advanced ? ADVANCED_SECTIONS : MAIN_SECTIONS);

	async function load() {
		loadError = '';
		try {
			const [t, a, f] = await Promise.all([agentsApi.templates(), agentsApi.list(), agentsApi.folders('.')]);
			templates = t.templates;
			existing = a.agents.map((x) => x.path);
			defaultFolder = f.folders.includes('agents') ? 'agents' : '.';
			if (!template) folder = defaultFolder;
		} catch (e) {
			loadError = (e as Error).message;
		}
	}

	onMount(async () => {
		await load();
		const draft = loadDraft();
		if (draft) {
			st.dispose();
			folder = draft.folder;
			filename = draft.filename;
			st = new ManifestState(targetPath(folder, new Manifest(draft.text).str(['identity', 'name']) || 'new-agent', filename), draft.text);
			section = draft.section ?? 'agent';
			advanced = ADVANCED_SECTIONS.some(([key]) => key === section);
			template = draft.template;
			restored = true;
		}
		loaded = true;
	});
	onDestroy(() => st.dispose());

	$effect(() => {
		void st.rev;
		if (loaded && template && !saved) saveDraft({ template, text: st.text, step: 1, section, folder, filename, tried: false });
	});
	$effect(() => {
		const state = st;
		void state.rev;
		state.path = path;
		if (template && !saved) state.scheduleCheck();
		return () => state.dispose();
	});

	function pick(t: Template) {
		st.dispose();
		folder = defaultFolder;
		filename = undefined;
		st = new ManifestState(targetPath(folder, new Manifest(t.text).str(['identity', 'name'])), t.text);
		template = t.name;
		section = 'agent'; advanced = false; restored = false; fieldErrors = []; saveError = '';
	}
	function startOver() {
		if (trialBusy || saving) return;
		st.dispose();
		saveDraft(null); trial = null; workedText = ''; workedFolder = '';
		saved = null; savedWorked = false;
		template = ''; section = 'agent'; advanced = false; filename = undefined;
		restored = false; importing = false; importText = ''; importProblems = null;
		saveError = ''; fieldErrors = []; st = new ManifestState('');
	}
	function chooseSection(next: SetupSection) {
		section = next;
		advanced = ADVANCED_SECTIONS.some(([key]) => key === next);
	}
	async function validated() {
		if (blockers.length || fieldErrors.length) return null;
		const state = st, text = state.text, destination = path, trialFolder = folder;
		await state.check();
		if (state !== st || text !== st.text || destination !== path) {
			saveError = 'Settings changed during validation. Try again.';
			return null;
		}
		if (state.checkError || state.problems.length) return null;
		return { text, destination, folder: trialFolder };
	}
	async function save() {
		if (saving || trialBusy) return;
		saving = true; saveError = '';
		try {
			const draft = await validated();
			if (!draft) return;
			await agentsApi.create(draft.destination, draft.text);
			savedWorked = worked; saveDraft(null); saved = draft.destination;
			trial = null;
		} catch (e) {
			saveError = (e as Error).message;
		} finally { saving = false; }
	}
	async function startTrial() {
		if (trialBusy || starting) return;
		starting = true; saveError = '';
		try {
			const draft = await validated();
			if (draft) {
				trial = { text: draft.text, folder: draft.folder, epoch: ++epoch };
				workedText = ''; workedFolder = ''; preview = 'trial';
			}
		} finally { starting = false; }
	}

	async function checkImport() {
		const text = importText;
		const destination = targetPath(folder, new Manifest(text).str(['identity', 'name']) || 'imported');
		try {
			const result = await agentsApi.check(text, destination);
			if (text === importText) importProblems = result.problems;
		} catch (e) {
			if (text === importText) importProblems = [{ path: '', line: 0, message: (e as Error).message }];
		}
	}
	async function readImport(e: Event) {
		const file = (e.currentTarget as HTMLInputElement).files?.[0];
		if (!file) return;
		try { importText = await file.text(); await checkImport(); }
		catch (e) { saveError = (e as Error).message; }
	}
	async function saveImport() {
		if (saving) return;
		const n = new Manifest(importText).str(['identity', 'name']);
		if (!n) { saveError = 'The file needs identity.name.'; return; }
		saving = true; saveError = '';
		try {
			const destination = targetPath(folder, n);
			await agentsApi.create(destination, importText);
			saveDraft(null); await goto(`/agents/${destination}`);
		} catch (e) { saveError = (e as Error).message; }
		finally { saving = false; }
	}
	const templateTools = (t: Template) => new Manifest(t.text).toolNames().filter(Boolean);

	function tabKey(e: KeyboardEvent, key: SetupSection) {
		if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) return;
		e.preventDefault();
		const index = primary.findIndex(([value]) => value === key);
		const next = e.key === 'Home' ? 0 : e.key === 'End' ? primary.length - 1 : (index + (e.key === 'ArrowRight' ? 1 : -1) + primary.length) % primary.length;
		chooseSection(primary[next][0]);
		document.getElementById(`setup-tab-${primary[next][0]}`)?.focus();
	}
	function previewKey(e: KeyboardEvent) {
		if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(e.key)) return;
		e.preventDefault();
		preview = e.key === 'Home' ? 'trial' : e.key === 'End' ? 'yaml' : preview === 'trial' ? 'yaml' : 'trial';
		document.getElementById(`preview-tab-${preview}`)?.focus();
	}
</script>

<svelte:head><title>New agent - mani</title></svelte:head>

<div class="new-agent">
	<header class="head">
		<h1>New agent</h1>
		<div class="row">
			{#if template || importing}<button class="btn ghost" disabled={saving || trialBusy} onclick={startOver}>Start over</button>{/if}
			{#if template && !saved}<button class="btn primary" disabled={invalid || st.checking || saving || trialBusy || starting} onclick={save}>{saving ? 'Saving…' : 'Create agent'}</button>{/if}
		</div>
	</header>
	{#if loadError}<div class="banner error" role="alert"><span>{loadError}</span><button class="btn secondary small" onclick={load}>Retry</button></div>{/if}
	{#if saved}
		<section class="card done">
			<Icon name="check" size={28} /><h2>{savedWorked ? 'Your agent works' : 'Your agent is saved'}</h2>
			<p class="sm mu">Saved as <code>{saved}</code>.</p>
			<div class="row"><a class="btn primary" href="/chat?agent={encodeURIComponent(saved)}"><Icon name="chat" size={16} />Open in chat</a><a class="btn secondary" href="/agents/{saved}">Open in the editor</a><a class="btn secondary" href="/agents/{saved}?block=run">Schedule it</a></div>
		</section>
	{:else if importing}
		<section class="import stack">
			<h2>Import YAML</h2>
			<label class="field"><span>File</span><input type="file" accept=".yaml,.yml" onchange={readImport} /></label>
			<label class="field"><span>Or paste it</span><textarea class="input mono" rows="14" bind:value={importText} onchange={checkImport}></textarea></label>
			<div class="row"><span class="label">Save in</span><code>{targetPath(folder, new Manifest(importText).str(['identity', 'name']) || 'imported')}</code><button class="btn ghost small" onclick={() => (picking = true)}>Change folder</button></div>
			{#if importProblems?.length}<ul class="problems">{#each importProblems as p, i (i)}<li>{p.message}</li>{/each}</ul>{:else if importProblems}<p class="help">The manifest is valid.</p>{/if}
			{#if saveError}<p class="help error" role="alert">{saveError}</p>{/if}
			<div class="row"><button class="btn secondary" onclick={startOver}>Back</button><button class="btn primary" disabled={saving || !importText.trim()} onclick={saveImport}>Save and open</button></div>
		</section>
	{:else if !template}
		<section class="start">
			<h2>Start from</h2>
			{#if !loaded}<p class="sm mu">Loading templates…</p>{/if}
			<div class="templates" role="group" aria-label="Start from">
				{#each templates as t (t.name)}
					<button class="tpl" onclick={() => pick(t)}><b>{t.title}</b><span class="sm mu">{t.about}</span><span class="row">{#each templateTools(t) as tool (tool)}<span class="badge {RISK_CLASS[TOOL_INFO[tool]?.risk ?? 'none']}">{tool}</span>{/each}</span></button>
				{/each}
				<button class="tpl" onclick={() => { importing = true; folder = defaultFolder; }}><b>Import YAML</b><span class="sm mu">Use an existing manifest.</span></button>
			</div>
		</section>
	{:else}
		{#if restored}<div class="draft-note" role="status">Your draft is back where you left it.</div>{/if}
		{#if saveError}<p class="error-line help error" role="alert">{saveError}</p>{/if}
		<ResizablePanels>
			{#snippet left()}
				{#if advanced}<div class="advanced-back"><button class="btn ghost small" onclick={() => chooseSection('agent')}><Icon name="back" size={16} />Agent settings</button></div>{/if}
				<div class="tabs" role="tablist" aria-label={advanced ? 'Advanced options' : 'Agent settings'}>
					{#each primary as [key, label] (key)}
						<button role="tab" id="setup-tab-{key}" aria-controls="setup-panel" aria-selected={section === key} tabindex={section === key ? 0 : -1} onclick={() => chooseSection(key)} onkeydown={(e) => tabKey(e, key)}>{label}</button>
					{/each}
				</div>
				<div class="form-scroll" id="setup-panel" role="tabpanel" aria-labelledby="setup-tab-{section}">
					<fieldset disabled={saving} hidden={section === 'file'}>
						<AgentSetup ed={st} section={section === 'file' ? 'agent' : section} onerrors={(errors) => (fieldErrors = errors)} />
					</fieldset>
					{#if section === 'file'}
						<div class="file-panel stack">
							<label class="field"><span>File name</span><input class="input mono" value={filename ?? `${name || 'new-agent'}.yaml`} oninput={(e) => (filename = e.currentTarget.value)} disabled={saving} /></label>
							<div class="field"><span class="label">Manifest folder</span><div class="row"><code>{folder}</code><button class="btn secondary small" disabled={saving} onclick={() => (picking = true)}>Change folder</button></div></div>
						</div>
					{/if}
					{#if blockers.length || fieldErrors.length || st.problems.length || st.checkError}
						<div class="validation" role="status">
							{#each [...blockers, ...fieldErrors] as message, i (i)}<p>{message}</p>{/each}
							{#if st.checkError}<p>{st.checkError}</p><button class="btn secondary small" onclick={() => st.check()}>Retry validation</button>{/if}
							{#each st.problems as p, i (i)}<div><span>{p.message}</span><button class="btn ghost small" onclick={() => chooseSection(sectionFor(p))}>Fix</button></div>{/each}
						</div>
					{/if}
				</div>
				<footer class="editor-footer"><button class="btn ghost small" aria-expanded={advanced} onclick={() => chooseSection(advanced ? 'agent' : 'output')}><Icon name="sliders" size={16} />Advanced options</button><span class="cap mono" title={path}>{path}</span></footer>
			{/snippet}
			{#snippet right()}
				<div class="preview-head">
					<div class="tabs" role="tablist" aria-label="Preview mode">
						<button role="tab" id="preview-tab-trial" aria-controls="trial-preview" aria-selected={preview === 'trial'} tabindex={preview === 'trial' ? 0 : -1} onclick={() => (preview = 'trial')} onkeydown={previewKey}>Try agent</button>
						<button role="tab" id="preview-tab-yaml" aria-controls="yaml-preview" aria-selected={preview === 'yaml'} tabindex={preview === 'yaml' ? 0 : -1} onclick={() => (preview = 'yaml')} onkeydown={previewKey}>YAML</button>
					</div>
					{#if trial}<button class="btn ghost small" disabled={trialBusy || invalid || st.checking || starting} onclick={startTrial}>{staleTrial ? 'Apply changes' : 'Clear'}</button>{/if}
				</div>
				<div class="trial-preview" id="trial-preview" role="tabpanel" aria-labelledby="preview-tab-trial" hidden={preview !== 'trial'}>
					{#if trial}
						{#if staleTrial}<p class="trial-note">Settings changed. Apply changes for a new trial.</p>{/if}
						{#key trial.epoch}
							{@const snapshot = trial}
							<TrialChat text={snapshot.text} folder={snapshot.folder} who={new Manifest(snapshot.text).str(['identity', 'name'])} onbusy={(busy) => (trialBusy = busy)} onworked={() => { workedText = snapshot.text; workedFolder = snapshot.folder; }} />
						{/key}
						{#if worked}<p class="trial-note success" role="status">Your agent works.</p>{/if}
					{:else}
						<div class="trial-empty"><Icon name="agent" size={28} /><button class="btn primary" disabled={invalid || st.checking || starting} onclick={startTrial}>{starting ? 'Starting…' : 'Start trial'}</button><p class="help">Tools run for real. Trial history is not saved.</p></div>
					{/if}
				</div>
				<div class="yaml-preview" id="yaml-preview" role="tabpanel" aria-labelledby="preview-tab-yaml" tabindex="0" hidden={preview !== 'yaml'}><pre>{st.text}</pre></div>
			{/snippet}
		</ResizablePanels>
	{/if}
</div>
<FolderPicker bind:open={picking} current={folder} onpick={(p) => (folder = p === '.' ? '.' : p.replace(/^\.\//, ''))} />

<style>
	[hidden] { display: none !important; }
	.new-agent { height: calc(100dvh - 64px); min-height: 0; display: flex; flex-direction: column; }
	.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 20px 28px; border-bottom: 1px solid var(--bd); flex-shrink: 0; }
	.head h1 { font-size: 24px; line-height: 32px; }
	.start, .import { padding: 28px; overflow: auto; }
	.start h2 { margin-bottom: 20px; }
	.templates { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; }
	.tpl { display: flex; flex-direction: column; gap: 10px; text-align: left; padding: 18px; border-radius: var(--r-card); border: 1px solid var(--bd); background: var(--sf); cursor: pointer; }
	.tpl:hover { border-color: var(--pr); }
	.import { max-width: 880px; }
	.import textarea { height: auto; min-height: 240px; padding: 12px; resize: vertical; }
	.tabs { display: flex; flex-shrink: 0; gap: 22px; border-bottom: 1px solid var(--bd); padding: 0 28px; overflow-x: auto; }
	.tabs button { background: none; border: 0; border-bottom: 2px solid transparent; padding: 15px 0 13px; font-size: 14px; line-height: 20px; font-weight: 600; color: var(--mu); white-space: nowrap; cursor: pointer; }
	.tabs button[aria-selected='true'] { border-color: var(--pr); color: var(--pr); }
	.form-scroll { flex: 1; min-height: 0; overflow: auto; padding: 24px 28px; scrollbar-gutter: stable; }
	fieldset { border: 0; margin: 0; padding: 0; min-width: 0; }
	.editor-footer { padding: 12px 20px; display: flex; align-items: center; justify-content: space-between; gap: 8px; border-top: 1px solid var(--bd); }
	.editor-footer .cap { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.advanced-back { padding: 10px 20px; border-bottom: 1px solid var(--bd); }
	.preview-head { display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid var(--bd); padding: 0 24px; gap: 10px; }
	.preview-head .tabs { border: 0; padding: 0; }
	.trial-preview, .yaml-preview { display: flex; flex-direction: column; flex: 1; min-height: 0; padding: 20px 24px; gap: 12px; }
	.trial-empty { margin: auto; display: flex; flex-direction: column; gap: 16px; align-items: center; text-align: center; max-width: 32ch; color: var(--mu); }
	.yaml-preview { overflow: auto; }
	.yaml-preview pre { margin: 0; min-height: 100%; width: max-content; min-width: 100%; font: 12px/20px var(--f-mono); background: var(--sf); padding: 16px; border: 1px solid var(--bd); border-radius: var(--r-card); }
	.trial-note, .draft-note { font-size: 13px; line-height: 20px; color: var(--mu); }
	.draft-note { padding: 8px 28px; border-bottom: 1px solid var(--bd); background: var(--prt); color: var(--pr); }
	.success { color: var(--ok); }
	.validation { margin-top: 24px; font-size: 13px; line-height: 20px; color: var(--er); display: grid; gap: 8px; }
	.validation > div { display: flex; justify-content: space-between; align-items: center; gap: 8px; }
	.error-line { padding: 8px 28px; }
	.done { margin: 28px; max-width: 760px; display: flex; flex-direction: column; gap: 16px; align-items: flex-start; }
	.done > :global(svg) { color: var(--ok); }
	@media (max-width: 760px) {
		.head { padding: 16px; flex-wrap: wrap; }
		.head h1 { font-size: 22px; }
		.head .btn { padding: 0 12px; }
		.tabs { padding: 0 16px; gap: 20px; }
		.form-scroll { padding: 20px 16px; }
		.editor-footer { padding: 10px 12px; }
		.preview-head { padding: 0 16px; }
		.trial-preview, .yaml-preview { padding: 16px; }
		.templates { grid-template-columns: minmax(0, 1fr); }
		.start, .import { padding: 20px 16px; }
	}
</style>

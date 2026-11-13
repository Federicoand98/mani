<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '#lib/messages/en.ts';
	import { getReduceMotion, getTheme, setReduceMotion, setTheme, type Theme } from '#lib/theme.ts';
	import { getKeep, KEEP_MAX, KEEP_MIN, setKeep } from '#lib/prefs.ts';
	import { api } from '#lib/api/client.ts';
	import { toast } from '#lib/state.svelte.ts';

	const t = m.settings;
	let theme = $state<Theme>('system');
	let reduce = $state(false);
	let keep = $state(getKeep());
	let output = $state<number | undefined>();
	let outputError = $state('');
	let saving = $state(false);
	const outputValid = $derived(Number.isSafeInteger(output) && (output ?? 0) > 0 && (output ?? 0) <= 1_073_741_824);
	const keepValid = $derived(Number.isInteger(keep) && keep >= KEEP_MIN && keep <= KEEP_MAX);

	onMount(() => {
		theme = getTheme();
		reduce = getReduceMotion();
		api.settings().then((s) => { output = s.maxOutputTokens; }, (e) => { outputError = (e as Error).message; });
	});

	async function saveOutput() {
		if (!outputValid) { outputError = m.context.invalid; return; }
		saving = true; outputError = '';
		try {
			await api.saveSettings({ maxOutputTokens: output! });
			toast(m.context.outputSaved);
		} catch (e) { outputError = (e as Error).message; }
		finally { saving = false; }
	}

	const themes: Theme[] = ['system', 'light', 'dark'];
</script>

<svelte:head><title>{t.title} - mani</title></svelte:head>

<div>
	<h1>{t.title}</h1>
	<p class="sm mu">{t.subtitle}</p>
</div>

<section class="card stack" style="max-width:640px">
	<h2>{t.appearance}</h2>
	<div class="field">
		<span class="label" id="theme-label">{t.theme}</span>
		<div class="seg" role="group" aria-labelledby="theme-label">
			{#each themes as k (k)}
				<button aria-pressed={theme === k} onclick={() => setTheme((theme = k))}>{t.themes[k]}</button>
			{/each}
		</div>
	</div>
	<label class="toggle">
		<input type="checkbox" bind:checked={reduce} onchange={() => setReduceMotion(reduce)} />
		{t.reduceMotion}
	</label>
	<div class="field" style="max-width:260px">
		<label for="lang">{t.language}</label>
		<select id="lang" class="input" disabled><option>English</option></select>
		<p class="help">{t.languageHelp}</p>
	</div>
</section>

<section class="card stack" style="max-width:640px">
	<h2>{t.chat}</h2>
	<form class="stack" onsubmit={(e) => (e.preventDefault(), saveOutput())}>
		<div class="field">
			<label for="requested-output">{m.context.requestedOutput}</label>
			<input id="requested-output" class="input" type="number" min="1" max="1073741824" step="1" bind:value={output} disabled={saving} aria-describedby="requested-output-help" />
			<p class="help" id="requested-output-help">{m.context.requestedHelp}</p>
		</div>
		{#if outputError}<p class="help error" role="alert">{outputError}</p>{/if}
		<button class="btn secondary" type="submit" disabled={saving || !outputValid}>{m.context.save}</button>
	</form>
	<div class="field" style="max-width:260px">
		<label for="keep">{t.keep}</label>
		<input id="keep" class="input" type="number" min={KEEP_MIN} max={KEEP_MAX} bind:value={keep} aria-invalid={!keepValid} aria-describedby="keep-help" onchange={() => keepValid && setKeep(keep)} />
		<p class="help" class:error={!keepValid} id="keep-help">{t.keepHelp(KEEP_MIN, KEEP_MAX)}</p>
	</div>
</section>

<style>
	.toggle {
		display: flex;
		align-items: center;
		gap: 10px;
		font-size: 14px;
		cursor: pointer;
	}
	.toggle input {
		width: 18px;
		height: 18px;
		accent-color: var(--pr);
	}
</style>

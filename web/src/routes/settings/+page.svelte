<script lang="ts">
	import { onMount } from 'svelte';
	import { m } from '#lib/messages/en.ts';
	import { getReduceMotion, getTheme, setReduceMotion, setTheme, type Theme } from '#lib/theme.ts';

	const t = m.settings;
	let theme = $state<Theme>('system');
	let reduce = $state(false);

	onMount(() => {
		theme = getTheme();
		reduce = getReduceMotion();
	});

	const themes: Theme[] = ['system', 'light', 'dark'];
</script>

<svelte:head><title>{t.title} · mani</title></svelte:head>

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

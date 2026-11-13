<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { api, ApiError } from '../api/client.ts';
	import type { Provider } from '../api/types.ts';
	import { m } from '../messages/en.ts';
	import { validateBaseUrl, validateKey } from '../providers.ts';
	import CopyButton from './CopyButton.svelte';
	import Icon from './Icon.svelte';

	let {
		open = $bindable(false),
		providers,
		onadded
	}: { open: boolean; providers: Provider[]; onadded: (p: Provider) => void } = $props();

	const t = m.addProvider;

	let chosen = $state<Provider | null>(null);
	let url = $state('');
	let key = $state('');
	let busy = $state(false);
	let error = $state('');
	let fieldError = $state('');
	let found = $state<string[] | null>(null);
	let device = $state<{ code: string; uri: string; minutes: number } | null>(null);
	let poll: ReturnType<typeof setInterval> | undefined;

	function reset() {
		chosen = null;
		url = key = error = fieldError = '';
		found = null;
		device = null;
		busy = false;
		clearInterval(poll);
	}

	function pick(p: Provider) {
		reset();
		chosen = p;
		url = p.baseUrl;
		if (p.auth === 'device') startDevice();
	}

	function fail(e: unknown) {
		error = e instanceof ApiError && e.status === 0 ? m.errors.unreachable : (e as Error).message;
	}

	async function findOllama() {
		fieldError = validateBaseUrl(url) ? m.errors.badUrl : '';
		if (fieldError || !chosen) return;
		busy = true;
		error = '';
		try {
			const r = await api.testProvider(chosen.name, { baseUrl: url.trim() });
			if (r.ok) found = r.models;
			else error = r.error ?? t.refused(chosen.label);
		} catch (e) {
			fail(e);
		} finally {
			busy = false;
		}
	}

	async function save(input: { baseUrl?: string; key?: string }) {
		if (!chosen) return;
		const p = await api.saveProvider(chosen.name, input);
		open = false;
		reset();
		onadded(p);
	}

	async function addOllama() {
		busy = true;
		try {
			await save({ baseUrl: url.trim() });
		} catch (e) {
			fail(e);
		} finally {
			busy = false;
		}
	}

	async function testAndAdd() {
		if (!chosen) return;
		fieldError = validateKey(key) ? m.errors.required('API key') : '';
		if (!fieldError && validateBaseUrl(url)) fieldError = m.errors.badUrl;
		if (fieldError) return;
		busy = true;
		error = '';
		const input = { key: key.trim(), baseUrl: url.trim() };
		try {
			const r = await api.testProvider(chosen.name, input);
			if (!r.ok) {
				error = `${t.refused(chosen.label)} ${r.error ?? ''}`.trim();
				return;
			}
			await save(input);
		} catch (e) {
			fail(e);
		} finally {
			busy = false;
		}
	}

	async function startDevice() {
		busy = true;
		try {
			const d = await api.deviceStart();
			device = { code: d.userCode, uri: d.verificationUri, minutes: Math.round(d.expiresIn / 60) };
			poll = setInterval(async () => {
				try {
					const s = await api.deviceStatus();
					if (s.status === 'done') {
						clearInterval(poll);
						await save({});
					} else if (s.status === 'error') {
						clearInterval(poll);
						error = s.error ?? 'GitHub sign-in failed';
					}
				} catch (e) {
					clearInterval(poll);
					fail(e);
				}
			}, 2000);
		} catch (e) {
			fail(e);
		} finally {
			busy = false;
		}
	}
</script>

<Dialog.Root bind:open onOpenChange={(o) => !o && reset()}>
	<Dialog.Portal>
		<Dialog.Overlay class="scrim" />
		<Dialog.Content class="modal">
			{#if !chosen}
				<div>
					<Dialog.Title>{#snippet child({ props })}<h2 {...props}>{t.title}</h2>{/snippet}</Dialog.Title>
					<p class="sm mu">{t.pickBody}</p>
				</div>
				<div class="choices">
					{#each providers as p (p.name)}
						<button class="choice" disabled={p.enabled} onclick={() => pick(p)}>
							<b>{p.local ? `Local - ${p.label}` : p.label}</b>
							<span class="sm mu">{t.about[p.name]}</span>
							{#if p.enabled}
								<span class="badge ok">{t.alreadyAdded}</span>
							{:else if p.local}
								<span class="badge local">{t.recommended}</span>
							{/if}
						</button>
					{/each}
				</div>
				<div class="modal-foot">
					<span></span>
					<Dialog.Close class="btn secondary">{t.cancel}</Dialog.Close>
				</div>
			{:else}
				<div>
					<Dialog.Title>{#snippet child({ props })}<h2 {...props}>{chosen?.label}</h2>{/snippet}</Dialog.Title>
					<p class="sm mu">{t.about[chosen.name]}</p>
				</div>

				{#if chosen.auth === 'none'}
					<div class="field">
						<label for="ap-url">{t.url}</label>
						<input id="ap-url" class="input mono" bind:value={url} aria-invalid={!!fieldError} aria-describedby="ap-url-help" />
						<p class="help" class:error={!!fieldError} id="ap-url-help">{fieldError || t.urlHelp}</p>
					</div>
					{#if found}
						<div class="banner ok">
							<Icon name="check" />
							<span>
								{#if found.length}
									{t.found(found.length)}
									{#each found as name (name)}<code>{name}</code> {/each}
								{:else}
									{t.noModels} <code>ollama pull qwen3.5:9b</code>.
								{/if}
							</span>
						</div>
					{:else}
						<p class="help">{t.install} <code>ollama pull qwen3.5:9b</code>.</p>
					{/if}
				{:else if chosen.auth === 'key'}
					<div class="field">
						<label for="ap-key">{t.key}</label>
						<input id="ap-key" class="input mono" type="password" autocomplete="off" bind:value={key} aria-invalid={!!fieldError} aria-describedby="ap-key-help" />
						<p class="help" class:error={!!fieldError} id="ap-key-help">{fieldError || t.keyHelp}</p>
					</div>
					<details>
						<summary class="cap">{t.endpoint}</summary>
						<div class="field" style="margin-top:8px">
							<label for="ap-endpoint" class="sr-only">{t.endpoint}</label>
							<input id="ap-endpoint" class="input mono" bind:value={url} />
							<p class="help">{t.endpointHelp}</p>
						</div>
					</details>
				{:else if device}
					<p class="sm">{t.deviceIntro}</p>
					<div class="command"><span class="code">{device.code}</span><CopyButton text={device.code} /></div>
					<div class="row">
						<a class="btn secondary small" href={device.uri} target="_blank" rel="noreferrer">{t.openGitHub}</a>
						<span class="cap">{t.deviceWaiting} {t.deviceExpires(device.minutes)}</span>
					</div>
				{/if}

				{#if error}
					<div class="banner error" role="alert"><Icon name="x" /><span>{error}</span></div>
				{/if}

				<div class="modal-foot">
					<button class="btn secondary" onclick={reset}><Icon name="back" size={16} />{t.back}</button>
					{#if chosen.auth === 'none'}
						{#if found}
							<button class="btn primary" disabled={busy} onclick={addOllama}>{t.add}</button>
						{:else}
							<button class="btn primary" disabled={busy} onclick={findOllama}>{busy ? t.finding : t.find}</button>
						{/if}
					{:else if chosen.auth === 'key'}
						<button class="btn primary" disabled={busy} onclick={testAndAdd}>{busy ? t.working : t.testAndAdd}</button>
					{/if}
				</div>
			{/if}
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

<style>
	.choices {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 12px;
	}
	.choice {
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 8px;
		text-align: left;
		padding: 16px;
		border-radius: var(--r-card);
		border: 1px solid var(--bd);
		background: var(--sf);
		cursor: pointer;
	}
	.choice:hover:not(:disabled) {
		border-color: var(--pr);
	}
	.choice:disabled {
		opacity: 0.6;
		cursor: not-allowed;
	}
	.code {
		font-size: 20px;
		letter-spacing: 0.15em;
	}
	details summary {
		cursor: pointer;
		width: max-content;
	}
	@media (max-width: 560px) {
		.choices {
			grid-template-columns: minmax(0, 1fr);
		}
	}
</style>

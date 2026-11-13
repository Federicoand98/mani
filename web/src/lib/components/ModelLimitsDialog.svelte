<script lang="ts">
	import { Dialog } from 'bits-ui';
	import { api } from '../api/client.ts';
	import type { ModelLimits, Provider } from '../api/types.ts';
	import { validateLimits } from '../chat/context.ts';
	import { m } from '../messages/en.ts';

	let { open = $bindable(false), provider, model = '', models = [], onsaved }: {
		open: boolean; provider: Provider | null; model?: string; models?: string[]; onsaved: () => void | Promise<void>;
	} = $props();
	const t = m.context;
	let chosen = $state('');
	let window = $state<number | undefined>();
	let maxInput = $state<number | undefined>();
	let maxOutput = $state<number | undefined>();
	let busy = $state(false);
	let error = $state('');
	let info = $state('');
	let endpoint = $state<ModelLimits | null>(null);
	let requestedOutput = $state<number | undefined>();
	let settingsLoaded = $state(false);

	function fill(limits: ModelLimits) {
		window = limits.context_window || undefined;
		maxInput = limits.max_input_tokens || undefined;
		maxOutput = limits.max_output_tokens || undefined;
	}
	function choose(value: string) {
		chosen = value;
		fill(provider?.modelLimits?.[value] ?? {});
		error = info = '';
		endpoint = null;
	}
	$effect(() => {
		if (open) choose(model || models[0] || '');
	});
	$effect(() => {
		if (!open) return;
		settingsLoaded = false;
		requestedOutput = undefined;
		api.settings().then((s) => { requestedOutput = s.maxOutputTokens; settingsLoaded = true; }, (e) => { error = (e as Error).message; });
	});
	async function detect() {
		if (!provider || !chosen.trim()) return;
		busy = true; error = info = '';
		try {
			endpoint = await api.modelLimits(provider.name, chosen.trim());
			info = Object.values(endpoint).some((n) => n > 0) ? t.detected : t.unsupported;
		} catch (e) { error = (e as Error).message; }
		finally { busy = false; }
	}
	async function save() {
		if (!provider) return;
		const limits: ModelLimits = { context_window: window, max_input_tokens: maxInput, max_output_tokens: maxOutput };
		const hasManual = Object.values(limits).some((n) => n !== undefined);
		error = !chosen.trim() ? t.required : hasManual ? validateLimits(limits) : '';
		if (!Number.isSafeInteger(requestedOutput) || (requestedOutput ?? 0) <= 0 || (requestedOutput ?? 0) > 1_073_741_824) error = t.invalid;
		const contextWindow = window || endpoint?.context_window || 0;
		const outputLimit = Math.min(requestedOutput ?? 0, maxOutput || endpoint?.max_output_tokens || requestedOutput || 0);
		if (contextWindow && outputLimit >= contextWindow) error = t.incompatible;
		if (error) return;
		busy = true;
		try {
			if (hasManual) await api.saveProvider(provider.name, { modelLimits: { [chosen.trim()]: limits } });
			await api.saveSettings({ maxOutputTokens: requestedOutput! });
			await onsaved();
			open = false;
		} catch (e) { error = (e as Error).message; }
		finally { busy = false; }
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="scrim" />
		<Dialog.Content class="modal">
			<Dialog.Title>{#snippet child({ props })}<h2 {...props}>{t.limitsTitle} - {provider?.label}</h2>{/snippet}</Dialog.Title>
			<Dialog.Description class="sm mu">{t.limitsHelp}</Dialog.Description>
			<form class="stack" onsubmit={(e) => (e.preventDefault(), save())}>
				<div class="field">
					<label for="limits-model">{t.model}</label>
					<input id="limits-model" class="input" list="limits-models" value={chosen} onchange={(e) => choose(e.currentTarget.value)} disabled={busy} />
					<datalist id="limits-models">{#each models as name}<option value={name}></option>{/each}</datalist>
				</div>
				<button type="button" class="btn secondary" disabled={busy || !chosen.trim()} onclick={detect}>{busy ? t.detecting : t.detect}</button>
				{#if info}<p class="help" role="status">{info}</p>{/if}
				{#if endpoint && Object.values(endpoint).some((n) => n > 0)}
					<p class="help mono">{t.window}: {endpoint.context_window || t.unreported}<br />{t.maxInput}: {endpoint.max_input_tokens || t.unreported}<br />{t.maxOutput}: {endpoint.max_output_tokens || t.unreported}</p>
				{/if}
				<div class="field"><label for="limits-window">{t.window}</label><input id="limits-window" class="input" type="number" min="1" max="1073741824" step="1" bind:value={window} disabled={busy} /></div>
				<div class="field"><label for="limits-input">{t.maxInput}</label><input id="limits-input" class="input" type="number" min="1" max="1073741824" step="1" bind:value={maxInput} disabled={busy} /></div>
				<div class="field"><label for="limits-output">{t.maxOutput}</label><input id="limits-output" class="input" type="number" min="1" max="1073741824" step="1" bind:value={maxOutput} disabled={busy} /></div>
				<div class="field"><label for="limits-requested">{t.requestedOutput}</label><input id="limits-requested" class="input" type="number" min="1" max="1073741824" step="1" bind:value={requestedOutput} disabled={busy || !settingsLoaded} /><p class="help">{t.requestedHelp}</p></div>
				{#if error}<p class="help error" role="alert">{error}</p>{/if}
				<div class="row"><Dialog.Close class="btn secondary" disabled={busy}>{t.cancel}</Dialog.Close><button class="btn primary" type="submit" disabled={busy || !settingsLoaded}>{t.save}</button></div>
			</form>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>

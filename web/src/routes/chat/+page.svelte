<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { onDestroy, tick } from 'svelte';
	import { api } from '#lib/api/client.ts';
	import type { ChatPart, Conversation, ConversationMeta } from '#lib/api/types.ts';
	import type { ServerMsg } from '#lib/api/wire.ts';
	import { commandHelp, findModel, parseCommand, type Command } from '#lib/chat/commands.ts';
	import { checkImage, readImage, type Attachment } from '#lib/chat/images.ts';
	import { ChatSocket, type SocketState } from '#lib/chat/socket.ts';
	import { idle, reduce, start, type Turn } from '#lib/chat/turn.ts';
	import ChatMessage from '#lib/components/ChatMessage.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import { m } from '#lib/messages/en.ts';
	import { getKeep, getLastModel, getShowThinking, setLastModel, setShowThinking } from '#lib/prefs.ts';
	import { app, enabledProviders, toast } from '#lib/state.svelte.ts';

	const t = m.chat;

	let list = $state<ConversationMeta[]>([]);
	let current = $state<Conversation | null>(null);
	let turn = $state<Turn>(idle());
	let pending = $state<ChatPart[] | null>(null); // the message being sent, until the reload
	let last = $state<{ thinking: string; thinkingMs: number; meta: string } | null>(null);
	// Notes from slash commands sit where they were written: before message number at.
	let notes = $state<{ at: number; text: string }[]>([]);
	let dragDepth = $state(0);
	let socketState = $state<SocketState>('open');
	let models = $state<Record<string, string[]>>({});
	let selected = $state<{ provider: string; model: string } | null>(null);
	let input = $state('');
	let images = $state<Attachment[]>([]);
	let attachError = $state('');
	let renaming = $state<string | null>(null);
	let renameText = $state('');
	let confirmDelete = $state<string | null>(null);
	let showThinking = $state(getShowThinking());
	let showList = $state(false);
	let log: HTMLElement | undefined = $state();
	let textarea: HTMLTextAreaElement | undefined = $state();
	let fileInput: HTMLInputElement | undefined = $state();

	let socket: ChatSocket | null = null;
	let socketId = '';

	const id = $derived(page.url.searchParams.get('c'));
	const providers = $derived(enabledProviders());
	const answering = $derived(turn.status === 'answering');
	const busy = $derived(turn.status === 'busy');
	const who = $derived(current?.model ?? selected?.model ?? '');

	// --- data -------------------------------------------------------------------

	async function loadList() {
		try {
			list = (await api.conversations()).conversations;
		} catch {
			// the shell shows the connection state
		}
	}
	loadList();

	$effect(() => {
		for (const p of providers) {
			if (!(p.name in models)) {
				models[p.name] = [];
				api.models(p.name).then(
					(r) => (models[p.name] = r.models),
					() => {}
				);
			}
		}
	});

	// The model a new conversation starts from: the last used, if still offered.
	$effect(() => {
		if (selected || current) return;
		const lastUsed = getLastModel();
		if (lastUsed && models[lastUsed.provider]?.includes(lastUsed.model)) selected = lastUsed;
		else {
			const p = providers.find((p) => models[p.name]?.length);
			if (p) selected = { provider: p.name, model: models[p.name][0] };
		}
	});

	$effect(() => {
		const want = id;
		if (want === socketId) return;
		open(want);
	});

	async function open(want: string | null) {
		socket?.close();
		socket = null;
		socketId = want ?? '';
		turn = idle();
		pending = null;
		last = null;
		notes = [];
		showList = false;
		if (!want) {
			current = null;
			return;
		}
		try {
			current = await api.conversation(want);
			selected = { provider: current.provider, model: current.model };
		} catch (e) {
			toast((e as Error).message);
			goto('/chat', { replaceState: true });
			return;
		}
		connect(want);
		await scrollDown(true);
	}

	function connect(cid: string) {
		socket = new ChatSocket(cid, onMessage, (s) => (socketState = s));
	}

	async function onMessage(msg: ServerMsg) {
		turn = reduce(turn, msg, Date.now());
		if (msg.type === 'token' || msg.type === 'thinking') return scrollDown();
		if (msg.type !== 'done' && msg.type !== 'cancelled' && msg.type !== 'error') return;

		const secs = ((Date.now() - turn.startedAt) / 1000).toFixed(1) + ' s';
		const finished = { ...turn };
		if (current) {
			try {
				current = await api.conversation(current.id);
			} catch {
				// keep what is on screen
			}
		}
		pending = null;
		last =
			finished.startedAt && finished.ended !== 'error'
				? {
						thinking: finished.thinking,
						thinkingMs: finished.thinkingMs,
						meta: finished.usage ? t.usage(finished.usage.input, finished.usage.output, secs) : ''
					}
				: null;
		turn = { ...idle(), error: finished.error, ended: finished.ended };
		loadList();
		await scrollDown();
	}

	// --- sending ----------------------------------------------------------------

	async function send() {
		const text = input.trim();
		if ((!text && !images.length) || answering || busy) return;

		const cmd = parseCommand(text);
		if (cmd) {
			input = '';
			if ('error' in cmd) note(cmd.error);
			else await runCommand(cmd);
			return;
		}
		if (!selected) return;

		let cid = id;
		if (!cid) {
			try {
				const c = await api.createConversation({ provider: selected.provider, model: selected.model });
				cid = c.id;
				current = c;
				socketId = c.id;
				connect(c.id);
				goto(`/chat?c=${c.id}`, { reset: false });
			} catch (e) {
				toast((e as Error).message);
				return;
			}
		}

		const sent = images;
		pending = [
			...sent.map((i) => ({ type: 'image', mediaType: i.mediaType, data: i.data })),
			...(text ? [{ type: 'text', text }] : [])
		];
		input = '';
		images = [];
		attachError = '';
		last = null;
		turn = start(Date.now());
		setLastModel(selected.provider, selected.model);
		await scrollDown(true);
		await socket?.send({
			type: 'input',
			input: text,
			images: sent.map((i) => ({ media_type: i.mediaType, data: i.data })),
			keep: getKeep()
		});
		resize();
	}

	function stop() {
		socket?.send({ type: 'cancel' });
	}

	function note(text: string) {
		notes = [...notes, { at: shown.length, text }];
		scrollDown(true);
	}

	async function runCommand(cmd: Command) {
		switch (cmd.name) {
			case 'help':
				note(commandHelp.map(([c, d]) => `${c.padEnd(16)}${d}`).join('\n'));
				break;
			case 'clear':
				await goto('/chat');
				break;
			case 'thinking':
				showThinking = !showThinking;
				setShowThinking(showThinking);
				note(showThinking ? t.thinkingOn : t.thinkingOff);
				break;
			case 'model': {
				if (!cmd.arg) {
					const all = Object.values(models).flat().join(', ');
					note(t.modelNow(selected?.model ?? '—', all));
					break;
				}
				const found = findModel(cmd.arg, models, selected?.provider ?? '');
				if (!found) note(t.noSuchModel(cmd.arg));
				else await chooseModel(found);
			}
		}
	}

	async function chooseModel(next: { provider: string; model: string }) {
		selected = next;
		if (!current) return;
		try {
			current = await api.updateConversation(current.id, next);
			if (current.messages.length) note(t.modelChanged(next.model));
			loadList();
		} catch (e) {
			toast((e as Error).message);
		}
	}

	function onModelChange(e: Event) {
		const [provider, ...rest] = (e.target as HTMLSelectElement).value.split('|');
		chooseModel({ provider, model: rest.join('|') });
	}

	function onkeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
			e.preventDefault();
			send();
		}
	}

	function resize() {
		if (!textarea) return;
		textarea.style.height = 'auto';
		textarea.style.height = Math.min(textarea.scrollHeight, 200) + 'px';
	}

	// --- images -----------------------------------------------------------------

	async function addFiles(files: Iterable<File>) {
		attachError = '';
		for (const f of files) {
			const why = checkImage(f, images.length);
			if (why) {
				attachError = why;
				continue;
			}
			images = [...images, await readImage(f)];
		}
	}

	function onpaste(e: ClipboardEvent) {
		const files = [...(e.clipboardData?.files ?? [])];
		if (files.length) {
			e.preventDefault();
			addFiles(files);
		}
	}

	// The whole conversation is a drop target, not just the text box.
	const carriesFiles = (e: DragEvent) => !!e.dataTransfer?.types.includes('Files');

	function ondragenter(e: DragEvent) {
		if (carriesFiles(e)) dragDepth++;
	}

	function ondragleave(e: DragEvent) {
		if (carriesFiles(e)) dragDepth = Math.max(0, dragDepth - 1);
	}

	function ondragover(e: DragEvent) {
		if (carriesFiles(e)) e.preventDefault(); // allows the drop
	}

	function ondrop(e: DragEvent) {
		e.preventDefault();
		dragDepth = 0;
		if (busy) return;
		if (e.dataTransfer?.files.length) addFiles(e.dataTransfer.files);
		textarea?.focus();
	}

	// --- list -------------------------------------------------------------------

	async function saveRename(c: ConversationMeta) {
		const title = renameText.trim();
		renaming = null;
		if (!title || title === c.title) return;
		try {
			const updated = await api.updateConversation(c.id, { title });
			if (current?.id === c.id) current = updated;
			loadList();
		} catch (e) {
			toast((e as Error).message);
		}
	}

	async function remove(c: ConversationMeta) {
		confirmDelete = null;
		try {
			await api.deleteConversation(c.id);
			toast(t.deleted);
			if (current?.id === c.id) await goto('/chat');
			loadList();
		} catch (e) {
			toast((e as Error).message);
		}
	}

	// --- view -------------------------------------------------------------------

	async function scrollDown(force = false) {
		await tick();
		if (!log) return;
		const near = log.scrollHeight - log.scrollTop - log.clientHeight < 120;
		if (force || near) log.scrollTop = log.scrollHeight;
	}

	const keep = getKeep();
	const shown = $derived([
		...(current?.messages ?? []),
		...(pending ? [{ role: 'user', parts: pending }] : [])
	]);
	const forgetBefore = $derived(shown.length > keep ? shown.length - keep : -1);
	const lastAssistant = $derived(shown.findLastIndex((x) => x.role === 'assistant'));

	onDestroy(() => socket?.close());

	function relative(iso: string) {
		const d = new Date(iso);
		const days = Math.floor((Date.now() - d.getTime()) / 86400000);
		if (days < 1) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
		if (days < 7) return d.toLocaleDateString([], { weekday: 'short' });
		return d.toLocaleDateString([], { day: 'numeric', month: 'short' });
	}
</script>

<svelte:head><title>{current?.title && current.title !== '(no title)' ? current.title : t.title} - mani</title></svelte:head>

{#if app.providersLoaded && !providers.length}
	<div class="center">
		<div class="card empty">
			<div class="icon"><Icon name="chat" size={24} /></div>
			<h1 class="h3">{t.noProviderTitle}</h1>
			<p>{t.noProviderBody}</p>
			<a class="btn primary" href="/providers">{t.goProviders}</a>
		</div>
	</div>
{:else}
	<div class="chat" class:show-list={showList}>
		<aside class="list" aria-label={t.listLabel}>
			<div class="list-head">
				<a class="btn primary" href="/chat"><Icon name="plus" size={16} />{t.newChat}</a>
				<span class="cap">{t.listHelp}</span>
			</div>
			<ul>
				{#each list as c (c.id)}
					<li class:active={c.id === id}>
						{#if renaming === c.id}
							<form class="rename" onsubmit={(e) => (e.preventDefault(), saveRename(c))}>
								<label class="sr-only" for="rename-{c.id}">{t.renameLabel}</label>
								<!-- svelte-ignore a11y_autofocus -->
								<input id="rename-{c.id}" class="input" bind:value={renameText} autofocus onblur={() => saveRename(c)} onkeydown={(e) => e.key === 'Escape' && (renaming = null)} />
							</form>
						{:else if confirmDelete === c.id}
							<div class="confirm">
								<span class="sm">{t.delete} “{c.title}”?</span>
								<div class="row">
									<button class="btn ghost small" onclick={() => (confirmDelete = null)}>{t.cancel}</button>
									<button class="btn danger small" onclick={() => remove(c)}>{t.confirmDelete}</button>
								</div>
							</div>
						{:else}
							<a href="/chat?c={c.id}" aria-current={c.id === id ? 'page' : undefined}>
								<b>{c.title === '(no title)' ? t.untitled : c.title}</b>
								<span>{c.model} - {relative(c.updatedAt)}{c.running ? ' - …' : ''}</span>
							</a>
							<div class="row-actions">
								<button class="icon-btn" title={t.rename} aria-label="{t.rename} {c.title}" onclick={() => ((renaming = c.id), (renameText = c.title))}><Icon name="pencil" size={14} /></button>
								<button class="icon-btn" title={t.delete} aria-label="{t.delete} {c.title}" onclick={() => (confirmDelete = c.id)}><Icon name="trash" size={14} /></button>
							</div>
						{/if}
					</li>
				{/each}
			</ul>
		</aside>
		{#if showList}<button class="list-scrim" aria-label="Close" onclick={() => (showList = false)}></button>{/if}

		<section class="conv" {ondragenter} {ondragleave} {ondragover} {ondrop} aria-label={t.title}>
			{#if dragDepth > 0}
				<div class="drop" aria-hidden="true"><Icon name="image" size={28} /><span>{t.dropImages}</span></div>
			{/if}
			<header class="conv-head">
				<button class="icon-btn list-toggle" aria-label={t.listLabel} onclick={() => (showList = !showList)}><Icon name="runs" size={18} /></button>
				<h1 class="title">{current && current.title !== '(no title)' ? current.title : t.untitled}</h1>
				{#if selected}
					{@const p = providers.find((x) => x.name === selected?.provider)}
					{#if p}
						<span class="badge" class:local={p.local} class:warn={!p.local}>{#if p.local}<span class="dot"></span>{:else}<Icon name="net" size={12} />{/if}{p.label}</span>
					{/if}
				{/if}
			</header>

			<div class="log" bind:this={log} role="log" aria-busy={answering || busy}>
				{#if !shown.length && !notes.length}
					<div class="hello">
						<div class="empty-icon"><Icon name="chat" size={24} /></div>
						<h2>{t.helloTitle(selected?.model ?? '…')}</h2>
						<p class="sm mu">{t.helloBody}</p>
						<div class="suggestions">
							{#each t.suggestions as s (s)}
								<button class="chip" onclick={() => ((input = s), textarea?.focus())}>{s}</button>
							{/each}
						</div>
					</div>
				{/if}

				{#each shown as msg, i (i)}
					{#each notes.filter((n) => n.at === i) as n, j (j)}
						<pre class="system note">{n.text}</pre>
					{/each}
					{#if i === forgetBefore}
						<div class="forget" role="note"><span>{t.forgotten}</span> <a href="/settings">{t.forgottenHelp}</a></div>
					{/if}
					<ChatMessage
						role={msg.role}
						parts={msg.parts}
						who={who}
						{showThinking}
						thinking={i === lastAssistant && last ? last.thinking : ''}
						thinkingMs={last?.thinkingMs ?? 0}
						meta={i === lastAssistant && last ? last.meta : ''}
					/>
				{/each}
				{#each notes.filter((n) => n.at >= shown.length) as n, j (j)}
					<pre class="system note">{n.text}</pre>
				{/each}

				{#if answering}
					<ChatMessage
						role="assistant"
						parts={[{ type: 'text', text: turn.text }]}
						{who}
						streaming
						{showThinking}
						thinking={turn.thinking}
						thinkingMs={turn.thinkingMs}
						thinkingLive={!turn.firstTokenAt}
					/>
				{/if}

				{#if busy}
					<div class="banner info notice"><Icon name="chat" /><span><b>{t.busyTitle}</b> {t.busyBody}</span></div>
				{/if}
				{#if turn.ended === 'cancelled'}
					<p class="system">{t.stopped}</p>
				{/if}
				{#if turn.error}
					<div class="banner error notice" role="alert"><Icon name="x" /><span>{turn.error}</span></div>
				{/if}
			</div>

			{#if socketState === 'lost'}
				<div class="banner error lost" role="alert">
					<Icon name="warn" />
					<span><b>{t.lostTitle}</b> {t.lostBody}</span>
					<button class="btn secondary small" onclick={() => socket?.retry()}>{t.reconnect}</button>
				</div>
			{/if}

			<form class="composer" onsubmit={(e) => (e.preventDefault(), send())}>
				<div class="box">
					{#if images.length}
						<div class="attachments">
							{#each images as img, i (img.url)}
								<div class="attachment">
									<img src={img.url} alt={img.name} />
									<button type="button" aria-label={t.removeImage(img.name)} onclick={() => (images = images.filter((_, j) => j !== i))}><Icon name="x" size={12} /></button>
								</div>
							{/each}
						</div>
					{/if}
					<label class="sr-only" for="chat-input">{t.placeholder}</label>
					<textarea
						id="chat-input"
						bind:this={textarea}
						bind:value={input}
						rows="1"
						placeholder={t.placeholder}
						disabled={busy || !selected}
						oninput={resize}
						{onkeydown}
						{onpaste}
					></textarea>
					<div class="bar">
						<button type="button" class="icon-btn" aria-label={t.attach} title={t.attach} onclick={() => fileInput?.click()}><Icon name="image" size={16} /></button>
						<input bind:this={fileInput} type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple class="sr-only" tabindex="-1" onchange={(e) => (addFiles((e.target as HTMLInputElement).files ?? []), ((e.target as HTMLInputElement).value = ''))} />
						<label class="sr-only" for="chat-model">{t.model}</label>
						<select id="chat-model" class="input model" value={selected ? `${selected.provider}|${selected.model}` : ''} onchange={onModelChange} disabled={answering || busy}>
							{#each providers as p (p.name)}
								<optgroup label={p.label}>
									{#each models[p.name] ?? [] as mo (mo)}
										<option value="{p.name}|{mo}">{mo}</option>
									{/each}
								</optgroup>
							{/each}
						</select>
						{#if answering}
							<button type="button" class="btn secondary small send" onclick={stop}><Icon name="stop" size={14} />{t.stop}</button>
						{:else}
							<button type="submit" class="btn primary small send" disabled={busy || !selected || (!input.trim() && !images.length)} aria-label={t.send}><Icon name="send" size={14} /></button>
						{/if}
					</div>
				</div>
				{#if attachError}<p class="help error">{attachError}</p>{/if}
				<p class="hint">{t.hint}</p>
			</form>
		</section>
	</div>
{/if}

<style>
	.center {
		display: grid;
		place-items: center;
		padding: 32px 16px;
	}
	.h3 {
		font-size: 16px;
		line-height: 24px;
	}
	.chat {
		display: grid;
		grid-template-columns: 272px minmax(0, 1fr);
		height: calc(100dvh - 64px);
	}
	.list {
		border-right: 1px solid var(--bd);
		background: var(--sf);
		display: flex;
		flex-direction: column;
		min-height: 0;
	}
	.list-head {
		padding: 16px;
		display: flex;
		flex-direction: column;
		gap: 10px;
		border-bottom: 1px solid var(--bd);
	}
	.list ul {
		list-style: none;
		margin: 0;
		padding: 8px;
		overflow: auto;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}
	.list li {
		position: relative;
		border-radius: 8px;
	}
	.list li:hover,
	.list li.active {
		background: var(--sk);
	}
	.list li.active {
		background: var(--prt);
	}
	.list li a {
		display: flex;
		flex-direction: column;
		padding: 10px 64px 10px 10px;
		color: var(--tx);
		text-decoration: none;
		min-width: 0;
	}
	.list li b {
		font-size: 14px;
		line-height: 20px;
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.list li span {
		font-size: 12px;
		color: var(--mu);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.row-actions {
		position: absolute;
		right: 6px;
		top: 50%;
		transform: translateY(-50%);
		display: flex;
		gap: 2px;
		opacity: 0;
	}
	.list li:hover .row-actions,
	.list li:focus-within .row-actions {
		opacity: 1;
	}
	.icon-btn {
		width: 30px;
		height: 30px;
		border-radius: 8px;
		border: 0;
		background: transparent;
		color: var(--mu);
		display: inline-grid;
		place-items: center;
		cursor: pointer;
		flex: none;
	}
	.icon-btn:hover {
		background: var(--bd);
		color: var(--tx);
	}
	.rename,
	.confirm {
		padding: 6px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.confirm .sm {
		overflow-wrap: anywhere;
	}
	.conv {
		position: relative;
		display: flex;
		flex-direction: column;
		min-height: 0;
		min-width: 0;
	}
	.conv-head {
		display: flex;
		align-items: center;
		gap: 10px;
		padding: 12px 24px;
		border-bottom: 1px solid var(--bd);
		min-height: 57px;
	}
	.title {
		font-size: 16px;
		line-height: 24px;
		font-weight: 700;
		letter-spacing: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		min-width: 0;
	}
	.drop {
		position: absolute;
		inset: 12px;
		z-index: 30;
		border: 2px dashed var(--pr);
		border-radius: var(--r-card);
		background: color-mix(in srgb, var(--prt) 85%, transparent);
		color: var(--pr);
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 10px;
		font-weight: 700;
		pointer-events: none;
	}
	.list-toggle,
	.list-scrim {
		display: none;
	}
	.log {
		flex: 1;
		overflow-y: auto;
		padding: 24px;
		display: flex;
		flex-direction: column;
		gap: 18px;
		min-height: 0;
	}
	.hello {
		margin: auto;
		max-width: 560px;
		text-align: center;
		display: flex;
		flex-direction: column;
		gap: 10px;
		align-items: center;
	}
	.empty-icon {
		width: 48px;
		height: 48px;
		border-radius: 12px;
		background: var(--prt);
		color: var(--pr);
		display: grid;
		place-items: center;
	}
	.suggestions {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
		justify-content: center;
		margin-top: 6px;
	}
	.chip {
		height: 32px;
		padding: 0 12px;
		border-radius: 999px;
		border: 1px solid var(--bs);
		background: var(--sf);
		font-size: 13px;
		font-weight: 600;
		cursor: pointer;
	}
	.chip:hover {
		border-color: var(--pr);
		color: var(--pr);
	}
	.forget {
		max-width: 760px;
		width: 100%;
		margin: 0 auto;
		display: flex;
		align-items: center;
		gap: 10px;
		font-size: 12px;
		color: var(--mu);
		flex-wrap: wrap;
	}
	.forget::before,
	.forget::after {
		content: '';
		flex: 1;
		min-width: 24px;
		border-top: 1px dashed var(--bs);
	}
	.notice {
		max-width: 760px;
		width: 100%;
		margin: 0 auto;
	}
	.system {
		max-width: 760px;
		width: 100%;
		margin: 0 auto;
		font-size: 13px;
		color: var(--mu);
		text-align: center;
	}
	.note {
		text-align: left;
		font-family: var(--f-mono);
		font-size: 12.5px;
		white-space: pre-wrap;
		background: var(--sk);
		border-radius: 10px;
		padding: 10px 14px;
	}
	.lost {
		margin: 0 24px 8px;
		align-items: center;
	}
	.lost span {
		flex: 1;
	}
	.composer {
		padding: 12px 24px 16px;
		border-top: 1px solid var(--bd);
	}
	.box {
		max-width: 760px;
		margin: 0 auto;
		border: 1px solid var(--bs);
		border-radius: 12px;
		background: var(--sf);
		padding: 10px 10px 8px 14px;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}
	.box:focus-within {
		border-color: var(--pr);
		box-shadow: 0 0 0 2px var(--pr);
	}
	textarea {
		border: 0;
		background: none;
		resize: none;
		font-size: 15px;
		line-height: 22px;
		max-height: 200px;
		padding: 4px 0;
		outline: none;
	}
	.bar {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	.model {
		height: 32px;
		width: auto;
		max-width: 260px;
		font-family: var(--f-mono);
		font-size: 12.5px;
		padding: 0 8px;
	}
	.send {
		margin-left: auto;
	}
	.attachments {
		display: flex;
		gap: 8px;
		flex-wrap: wrap;
	}
	.attachment {
		position: relative;
	}
	.attachment img {
		width: 64px;
		height: 64px;
		object-fit: cover;
		border-radius: 8px;
		border: 1px solid var(--bd);
		display: block;
	}
	.attachment button {
		position: absolute;
		top: -6px;
		right: -6px;
		width: 22px;
		height: 22px;
		border-radius: 50%;
		border: 1px solid var(--bd);
		background: var(--sf);
		display: grid;
		place-items: center;
		cursor: pointer;
	}
	.composer .help,
	.hint {
		max-width: 760px;
		margin: 6px auto 0;
	}
	.hint {
		font-size: 12px;
		color: var(--mu);
	}
	@media (max-width: 1023px) {
		.chat {
			grid-template-columns: minmax(0, 1fr);
		}
		.list {
			display: none;
		}
		.show-list .list {
			display: flex;
			position: fixed;
			left: 0;
			top: 64px;
			bottom: 0;
			width: 290px;
			z-index: 55;
			box-shadow: var(--sh3);
		}
		.show-list .list-scrim {
			display: block;
			position: fixed;
			inset: 64px 0 0;
			z-index: 54;
			border: 0;
			background: var(--scrim);
		}
		.list-toggle {
			display: inline-grid;
		}
		.log,
		.conv-head,
		.composer {
			padding-left: 16px;
			padding-right: 16px;
		}
		.row-actions {
			opacity: 1;
		}
	}
</style>

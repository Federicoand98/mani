<script lang="ts">
	import DOMPurify from 'dompurify';
	import { renderMarkdown } from '../chat/markdown.ts';

	let { id, value, onchange }: { id: string; value: string; onchange: (value: string) => void } = $props();
	let preview = $state(false);
	let editor: HTMLTextAreaElement;
	const html = $derived(preview ? DOMPurify.sanitize(renderMarkdown(value), {
		FORBID_TAGS: ['img', 'video', 'audio', 'iframe', 'source', 'style', 'button'],
		FORBID_ATTR: ['style']
	}) : '');
	const actions = [
		['heading', 'H2', 'Heading'], ['bold', 'B', 'Bold'], ['italic', 'I', 'Italic'],
		['list', 'List', 'Bullet list'], ['link', 'Link', 'Insert link'], ['code', '</>', 'Inline code']
	] as const;
	type Action = typeof actions[number][0];

	function format(action: Action) {
		const start = editor.selectionStart, end = editor.selectionEnd;
		const selected = value.slice(start, end);
		if (action === 'heading' || action === 'list') {
			const from = value.slice(0, start).lastIndexOf('\n') + 1;
			const to = end > start ? (value[end - 1] === '\n' ? end - 1 : end) : (value.indexOf('\n', end) < 0 ? value.length : value.indexOf('\n', end));
			const prefix = action === 'heading' ? '## ' : '- ';
			editor.setRangeText(value.slice(from, to).split('\n').map((line) => prefix + line).join('\n'), from, to, 'select');
		} else {
			const [before, after, fallback] = {
				bold: ['**', '**', 'bold text'], italic: ['_', '_', 'italic text'],
				code: ['`', '`', 'code'], link: ['[', '](https://example.org)', 'link text']
			}[action];
			const text = selected || fallback;
			editor.setRangeText(before + text + after, start, end, 'end');
			editor.setSelectionRange(start + before.length, start + before.length + text.length);
		}
		onchange(editor.value);
		editor.focus();
	}

	function shortcut(e: KeyboardEvent) {
		if (!(e.ctrlKey || e.metaKey) || e.altKey) return;
		const action = ({ b: 'bold', i: 'italic', k: 'link' } as const)[e.key.toLowerCase() as 'b' | 'i' | 'k'];
		if (action) { e.preventDefault(); format(action); }
	}
</script>

<div class="markdown-editor">
	<div class="editor-head">
		<span class="cap">Markdown</span>
		<div class="row" role="group" aria-label="Instructions mode">
			<button type="button" class="btn ghost small" aria-pressed={!preview} onclick={() => (preview = false)}>Write</button>
			<button type="button" class="btn ghost small" aria-pressed={preview} onclick={() => (preview = true)}>Preview</button>
		</div>
	</div>
	<div class="toolbar" role="group" aria-label="Markdown formatting" hidden={preview}>
		{#each actions as [action, text, label] (action)}
			<button type="button" class="btn ghost small" aria-label={label} title={label} onclick={() => format(action)}>{text}</button>
		{/each}
	</div>
	<textarea {id} class="input mono" rows="9" data-path="identity.prompt" {value} bind:this={editor} spellcheck="false" hidden={preview} oninput={(e) => onchange(e.currentTarget.value)} onkeydown={shortcut}></textarea>
	{#if preview}
		<!-- eslint-disable-next-line svelte/no-at-html-tags -- sanitized by renderMarkdown and DOMPurify -->
		<div class="preview md" role="region" aria-label="Instructions preview">{@html html}</div>
	{/if}
</div>

<style>
	[hidden] { display: none; }
	.markdown-editor { border: 1px solid var(--bs); border-radius: var(--r-field); overflow: hidden; background: var(--sf); }
	.markdown-editor:focus-within { outline: 2px solid var(--pr); outline-offset: -1px; }
	.editor-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 8px 10px; background: var(--bg); border-bottom: 1px solid var(--bd); }
	.editor-head .row { gap: 4px; }
	.editor-head button[aria-pressed='true'] { background: var(--prt); color: var(--pr); }
	.toolbar { display: flex; gap: 2px; padding: 6px; flex-wrap: wrap; border-bottom: 1px solid var(--bd); }
	.toolbar button { min-width: 32px; padding: 0 10px; }
	textarea.input { height: auto; min-height: 200px; border: 0; border-radius: 0; padding: 12px; font: 13px/22px var(--f-mono); resize: vertical; tab-size: 4; }
	textarea.input:focus { box-shadow: none; }
	.preview { padding: 16px; min-height: 200px; max-height: 420px; overflow: auto; font-size: 14px; line-height: 22px; overflow-wrap: anywhere; }
	.preview :global(> * + *) { margin-top: 12px; }
	.preview :global(h1) { font-size: 22px; line-height: 30px; }
	.preview :global(h2) { font-size: 18px; line-height: 26px; }
	.preview :global(pre) { white-space: pre-wrap; padding: 12px; background: var(--sk); border-radius: 8px; }
	.preview :global(pre code) { padding: 0; background: none; }
	.preview :global(table) { border-collapse: collapse; display: block; overflow: auto; }
	.preview :global(th), .preview :global(td) { border: 1px solid var(--bd); padding: 8px 12px; text-align: left; }
</style>

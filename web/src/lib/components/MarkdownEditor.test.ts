import { afterEach, expect, it } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import MarkdownEditor from './MarkdownEditor.svelte';

let component: ReturnType<typeof mount> | undefined;
afterEach(async () => { if (component) await unmount(component); document.body.innerHTML = ''; });

it('preserves newlines, lists and indentation in the sanitized Markdown preview', () => {
	const value = '## Rules\nFirst line\nSecond line\n\n- One\n- Two\n\n```text\n  indented\n```\n\n<img src="https://example.invalid/tracker"><script>evil()</script><span style="background:url(https://example.invalid/bg)">Safe</span>';
	component = mount(MarkdownEditor, { target: document.body, props: { id: 'prompt', value, onchange: () => {} } });
	flushSync();
	const preview = [...document.querySelectorAll('button')].find((button) => button.textContent === 'Preview')!;
	preview.click(); flushSync();
	expect(document.querySelector('.preview h2')?.textContent).toBe('Rules');
	expect(document.querySelectorAll('.preview p br')).toHaveLength(1);
	expect(document.querySelectorAll('.preview li')).toHaveLength(2);
	expect(document.querySelector('.preview pre')?.textContent).toContain('  indented');
	expect(document.querySelectorAll('.preview img,.preview script,.preview [style]')).toHaveLength(0);
	expect(document.querySelector<HTMLTextAreaElement>('textarea')?.value).toBe(value);
});

it('formats selected text and supports the keyboard without flattening line breaks', () => {
	let changed = '';
	component = mount(MarkdownEditor, { target: document.body, props: { id: 'prompt', value: 'First\nSecond\n', onchange: (value) => { changed = value; } } });
	flushSync();
	const input = document.querySelector<HTMLTextAreaElement>('textarea')!;
	input.setSelectionRange(0, 5);
	input.dispatchEvent(new KeyboardEvent('keydown', { key: 'b', ctrlKey: true, bubbles: true, cancelable: true }));
	expect(changed).toBe('**First**\nSecond\n');
});

it('adds list prefixes without adding an unwanted item after a selected trailing newline', () => {
	let changed = '';
	component = mount(MarkdownEditor, { target: document.body, props: { id: 'prompt', value: 'First\nSecond\nThird', onchange: (value) => { changed = value; } } });
	flushSync();
	document.querySelector<HTMLTextAreaElement>('textarea')!.setSelectionRange(0, 13);
	document.querySelector<HTMLButtonElement>('[aria-label="Bullet list"]')!.click();
	expect(changed).toBe('- First\n- Second\nThird');
});

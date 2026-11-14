import { afterEach, expect, it, vi } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import { agentsApi } from '../api/client.ts';
import NativeFolderInput from './NativeFolderInput.svelte';

let component: ReturnType<typeof mount> | undefined;
afterEach(async () => { if (component) await unmount(component); component = undefined; vi.restoreAllMocks(); document.body.innerHTML = ''; });
const settle = async () => { await Promise.resolve(); await Promise.resolve(); flushSync(); };

it('uses the absolute path from the backend without guessing from a browser folder name', async () => {
	let changed = '';
	const picker = vi.spyOn(agentsApi, 'pickFolder').mockResolvedValue({ path: '/outside/workspace', cancelled: false });
	component = mount(NativeFolderInput, { target: document.body, props: { id: 'ws', value: '.', onchange: (path) => { changed = path; } } });
	flushSync(); document.querySelector<HTMLButtonElement>('button')!.click(); await settle();
	expect(picker).toHaveBeenCalledWith('.', expect.any(AbortSignal));
	expect(changed).toBe('/outside/workspace');
});

it('does not change the path after cancellation and surfaces unavailable native dialogs', async () => {
	let changed = false;
	const picker = vi.spyOn(agentsApi, 'pickFolder').mockResolvedValue({ path: '', cancelled: true });
	component = mount(NativeFolderInput, { target: document.body, props: { id: 'ws', value: '/existing', onchange: () => { changed = true; } } });
	flushSync(); document.querySelector<HTMLButtonElement>('button')!.click(); await settle();
	expect(changed).toBe(false);
	picker.mockRejectedValue(new Error('No desktop folder picker available'));
	document.querySelector<HTMLButtonElement>('button')!.click(); await settle();
	expect(document.querySelector('[role="alert"]')?.textContent).toContain('Enter a path manually');
	expect(document.querySelector<HTMLInputElement>('input')?.value).toBe('/existing');
});

it('cancels the HTTP request when the field is destroyed', async () => {
	let signal: AbortSignal | undefined;
	vi.spyOn(agentsApi, 'pickFolder').mockImplementation((_path, requestSignal) => {
		signal = requestSignal;
		return new Promise(() => {});
	});
	component = mount(NativeFolderInput, { target: document.body, props: { id: 'ws', value: '.', onchange: () => {} } });
	flushSync(); document.querySelector<HTMLButtonElement>('button')!.click(); await settle();
	await unmount(component); component = undefined;
	expect(signal?.aborted).toBe(true);
});

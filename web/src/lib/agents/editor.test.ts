import { afterEach, expect, it, vi } from 'vitest';
import { agentsApi } from '../api/client.ts';
import type { CheckResult } from '../api/types.ts';
import { ManifestState } from './editor.svelte.ts';

afterEach(() => vi.restoreAllMocks());

it('ignores validation responses for an older draft and exposes transport errors', async () => {
	let resolveOld: (result: CheckResult) => void = () => {};
	const check = vi.spyOn(agentsApi, 'check').mockImplementationOnce(() => new Promise((resolve) => { resolveOld = resolve; }))
		.mockResolvedValueOnce({ problems: [] });
	const state = new ManifestState('agent.yaml', 'identity: {name: first}\n');
	const old = state.check();
	state.setText('identity: {name: second}\n');
	await state.check();
	resolveOld({ problems: [{ path: 'identity.name', line: 1, message: 'obsolete' }] });
	await old;
	expect(state.problems).toEqual([]);
	expect(state.checking).toBe(false);
	check.mockRejectedValue(new Error('mani is offline'));
	await state.check();
	expect(state.checkError).toBe('mani is offline');
	state.dispose();
});

it('does not replace a fresh success with an older failure for identical text', async () => {
	let failOld: (error: Error) => void = () => {};
	vi.spyOn(agentsApi, 'check').mockImplementationOnce(() => new Promise((_resolve, reject) => { failOld = reject; }))
		.mockResolvedValueOnce({ problems: [] });
	const state = new ManifestState('agent.yaml', 'identity: {name: same}\n');
	const old = state.check();
	await state.check();
	failOld(new Error('old network failure'));
	await old;
	expect(state.checkError).toBe('');
	expect(state.checking).toBe(false);
	state.dispose();
});

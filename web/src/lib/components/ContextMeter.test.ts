import { afterEach, expect, it } from 'vitest';
import { flushSync, mount, unmount } from 'svelte';
import ContextMeter from './ContextMeter.svelte';

let component: ReturnType<typeof mount> | undefined;
afterEach(async () => { if (component) await unmount(component); document.body.innerHTML = ''; });

it('shows the effective input denominator, reserved output and last reported request separately', () => {
	component = mount(ContextMeter, { target: document.body, props: { context: {
		estimated_input: 1200, input_budget: 6000, output_limit: 2000, context_window: 8000,
		max_input_tokens: 7000, max_output_tokens: 3000, limit_source: 'mixed', last_input: 1100,
		state: 'ready', compacted: false
	} } });
	flushSync();
	expect(document.querySelector('summary')?.textContent).toContain('~1,200 / 6,000');
	expect(document.querySelector('summary')?.textContent).not.toContain('8,000');
	expect(document.body.textContent).toContain('Reserved output2,000');
	expect(document.body.textContent).toContain('Last reported input1,100');
	expect(document.body.textContent).toContain('Manual + endpoint');
	expect(document.body.textContent).toContain('exclude your unsent draft');
});

it('keeps an actionable unknown state', () => {
	let configured = false;
	component = mount(ContextMeter, { target: document.body, props: { onconfigure: () => { configured = true; } } });
	flushSync();
	expect(document.querySelector('summary')?.textContent).toContain('Context unknown');
	expect(document.querySelector('.context-meter > button')?.textContent).toBe('Configure limits');
	document.querySelector<HTMLButtonElement>('.context-meter > button')?.click();
	expect(configured).toBe(true);
});

it('shows compacting state without replacing the saved context details', () => {
	component = mount(ContextMeter, { target: document.body, props: { context: {
		estimated_input: 7500, input_budget: 6000, output_limit: 2000, context_window: 8000,
		max_input_tokens: 0, max_output_tokens: 0, limit_source: 'manual',
		state: 'compacting', compacted: false
	} } });
	flushSync();
	expect(document.querySelector('summary')?.textContent).toContain('Summarizing earlier messages…');
	expect(document.body.textContent).toContain('Effective input budget6,000');
});

it('shows passive agent input estimates even when the input ceiling is unknown', () => {
	component = mount(ContextMeter, { target: document.body, props: { context: {
		estimated_input: 150, input_budget: 0, output_limit: 4096, context_window: 0,
		max_input_tokens: 0, max_output_tokens: 0, limit_source: '',
		state: 'unknown', compacted: false
	} } });
	flushSync();
	expect(document.querySelector('summary')?.textContent).toContain('~150 / ?');
	expect(document.body.textContent).toContain('Effective input budgetContext unknown');
});

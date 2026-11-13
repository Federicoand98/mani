import { describe, expect, it } from 'vitest';
import type { ContextUsage } from '../api/types.ts';
import { contextSummarized, hasInputLimit, mergedLimits, reduceContext, validateLimits } from './context.ts';

export const usage: ContextUsage = {
	estimated_input: 1200, input_budget: 6000, output_limit: 2000, context_window: 8000,
	max_input_tokens: 7000, max_output_tokens: 3000, limit_source: 'mixed', last_input: 1100,
	state: 'ready', compacted: false
};

describe('context state', () => {
	it('keeps context across usage, done, and new turns without summing request input', () => {
		let current = reduceContext(null, { type: 'context', payload: usage });
		for (const type of ['usage', 'done', 'token', 'cancelled', 'error']) current = reduceContext(current, { type, payload: { input: 99999 } });
		expect(current).toEqual(usage);
		const next = { ...usage, estimated_input: 500, last_input: 450 };
		expect(reduceContext(current, { type: 'context', payload: next })).toEqual(next);
	});

	it('announces compaction once, not on session reload or repeated ready events', () => {
		const compacted = { ...usage, compacted: true };
		const event = { type: 'context', payload: compacted };
		expect(contextSummarized(usage, event)).toBe(true);
		expect(contextSummarized(compacted, event)).toBe(false);
		expect(contextSummarized({ ...compacted, state: 'compacting' }, event)).toBe(true);
		expect(contextSummarized(null, { type: 'done' })).toBe(false);
	});
});

describe('model limits', () => {
	it('requires an input ceiling, not only an output ceiling', () => {
		expect(hasInputLimit({ max_output_tokens: 4096 })).toBe(false);
		expect(hasInputLimit({ context_window: 8192 })).toBe(true);
		expect(hasInputLimit({ max_input_tokens: 8192 })).toBe(true);
	});
	it('rejects fractional, zero, negative, huge and incompatible manual values', () => {
		for (const n of [0, -1, 1.5, NaN, Infinity, 1_073_741_825]) expect(validateLimits({ context_window: n })).not.toBe('');
		expect(validateLimits({})).not.toBe('');
		expect(validateLimits({ context_window: 100, max_output_tokens: 100 })).not.toBe('');
		expect(validateLimits({ context_window: 100, max_input_tokens: 101 })).not.toBe('');
		expect(validateLimits({ context_window: 8000, max_input_tokens: 7000, max_output_tokens: 2000 })).toBe('');
	});
	it('uses manual overrides per field and keeps endpoint-only fields', () => {
		expect(mergedLimits({ context_window: 8000, max_output_tokens: 2000 }, { max_input_tokens: 6000 })).toEqual({
			context_window: 8000, max_input_tokens: 6000, max_output_tokens: 2000
		});
	});
});

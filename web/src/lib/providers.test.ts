import { describe, expect, it } from 'vitest';
import { modelSummary, validateBaseUrl, validateKey } from './providers.ts';

describe('validateBaseUrl', () => {
	it.each([
		['http://localhost:11434', null],
		['https://api.openai.com/v1', null],
		['', 'empty'],
		['   ', 'empty'],
		['localhost:11434', 'scheme'],
		['ftp://box', 'scheme'],
		['http://', 'invalid']
	])('%s → %s', (url, want) => expect(validateBaseUrl(url)).toBe(want));
});

describe('validateKey', () => {
	it.each([
		['sk-abc', null],
		['  sk-abc  ', null],
		['', 'empty'],
		['sk abc', 'spaces']
	])('%s → %s', (key, want) => expect(validateKey(key)).toBe(want));
});

describe('modelSummary', () => {
	it('shows the first two and counts the rest', () => {
		expect(modelSummary(['a', 'b', 'c', 'd'])).toEqual({ first: ['a', 'b'], more: 2 });
		expect(modelSummary(['a'])).toEqual({ first: ['a'], more: 0 });
	});
});

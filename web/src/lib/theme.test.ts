import { beforeEach, describe, expect, it } from 'vitest';
import { getReduceMotion, getTheme, setReduceMotion, setTheme } from './theme.ts';

beforeEach(() => {
	localStorage.clear();
	delete document.documentElement.dataset.theme;
	document.documentElement.classList.remove('reduce');
});

describe('theme', () => {
	it('defaults to the system theme', () => {
		expect(getTheme()).toBe('system');
	});

	it('sets data-theme for an explicit choice and remembers it', () => {
		setTheme('dark');
		expect(document.documentElement.dataset.theme).toBe('dark');
		expect(getTheme()).toBe('dark');
	});

	it('removes data-theme and the saved value for system', () => {
		setTheme('light');
		setTheme('system');
		expect(document.documentElement.dataset.theme).toBeUndefined();
		expect(localStorage.getItem('mani:theme')).toBeNull();
	});

	it('ignores a garbage saved value', () => {
		localStorage.setItem('mani:theme', 'purple');
		expect(getTheme()).toBe('system');
	});

	it('toggles reduced motion on the root', () => {
		setReduceMotion(true);
		expect(document.documentElement.classList.contains('reduce')).toBe(true);
		expect(getReduceMotion()).toBe(true);
		setReduceMotion(false);
		expect(getReduceMotion()).toBe(false);
	});
});

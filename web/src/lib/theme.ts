// Theme and motion preferences: kept in localStorage, applied on <html>.
// app.html applies the saved values before the first paint; this module keeps
// them in sync afterwards.

export type Theme = 'system' | 'light' | 'dark';

const THEME = 'mani:theme';
const MOTION = 'mani:reduce-motion';

function read(key: string): string | null {
	try {
		return localStorage.getItem(key);
	} catch {
		return null; // storage blocked: fall back to the defaults
	}
}

function write(key: string, value: string | null) {
	try {
		if (value === null) localStorage.removeItem(key);
		else localStorage.setItem(key, value);
	} catch {
		// the choice still applies to this page
	}
}

export function getTheme(): Theme {
	const t = read(THEME);
	return t === 'light' || t === 'dark' ? t : 'system';
}

export function setTheme(t: Theme, root: HTMLElement = document.documentElement) {
	if (t === 'system') delete root.dataset.theme;
	else root.dataset.theme = t;
	write(THEME, t === 'system' ? null : t);
}

export function getReduceMotion(): boolean {
	return read(MOTION) === '1';
}

export function setReduceMotion(on: boolean, root: HTMLElement = document.documentElement) {
	root.classList.toggle('reduce', on);
	write(MOTION, on ? '1' : null);
}

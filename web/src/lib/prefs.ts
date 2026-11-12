import { read, write } from './storage.ts';

// Chat preferences of this browser.

export const KEEP_DEFAULT = 20;
export const KEEP_MIN = 2;
export const KEEP_MAX = 500;

/** How many messages the model remembers (M2-12). */
export function getKeep(): number {
	const n = Number(read('mani:keep'));
	return Number.isInteger(n) && n >= KEEP_MIN && n <= KEEP_MAX ? n : KEEP_DEFAULT;
}

export function setKeep(n: number) {
	write('mani:keep', n === KEEP_DEFAULT ? null : String(n));
}

/** The model of the last conversation, which a new one starts from (M2-9). */
export function getLastModel(): { provider: string; model: string } | null {
	const v = read('mani:last-model');
	const i = v ? v.indexOf('|') : -1;
	return v && i > 0 ? { provider: v.slice(0, i), model: v.slice(i + 1) } : null;
}

export function setLastModel(provider: string, model: string) {
	write('mani:last-model', `${provider}|${model}`);
}

/** Whether reasoning blocks start open. */
export function getShowThinking(): boolean {
	return read('mani:show-thinking') === '1';
}

export function setShowThinking(on: boolean) {
	write('mani:show-thinking', on ? '1' : null);
}

import { api, ApiError } from './api/client.ts';
import type { Health, Provider } from './api/types.ts';

export type Connection = 'checking' | 'ok' | 'offline' | 'signed-out';

// One shared state for the shell: is mani there, who is it, which providers exist.
export const app = $state({
	connection: 'checking' as Connection,
	health: null as Health | null,
	providers: [] as Provider[],
	providersLoaded: false
});

export const enabledProviders = () => app.providers.filter((p) => p.enabled);

function classify(e: unknown): Connection {
	if (e instanceof ApiError && e.status === 401) return 'signed-out';
	return 'offline';
}

export async function checkHealth() {
	try {
		app.health = await api.health();
		app.connection = 'ok';
	} catch (e) {
		app.connection = classify(e);
	}
}

export async function loadProviders() {
	try {
		app.providers = (await api.providers()).providers;
		app.providersLoaded = true;
	} catch (e) {
		app.connection = classify(e);
	}
}

export type Toast = { id: number; text: string };
export const toasts = $state<Toast[]>([]);
let seq = 0;

export function toast(text: string) {
	const id = ++seq;
	toasts.push({ id, text });
	setTimeout(() => {
		const i = toasts.findIndex((t) => t.id === id);
		if (i >= 0) toasts.splice(i, 1);
	}, 3500);
}

// Checks on the Add provider form, before anything is sent to mani.

export function validateBaseUrl(url: string): string | null {
	const u = url.trim();
	if (!u) return 'empty';
	try {
		const parsed = new URL(u);
		if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return 'scheme';
		return null;
	} catch {
		return 'invalid';
	}
}

export function validateKey(key: string): string | null {
	const k = key.trim();
	if (!k) return 'empty';
	if (/\s/.test(k)) return 'spaces';
	return null;
}

/** The models to show in a table cell: the first few and how many more. */
export function modelSummary(models: string[], shown = 2): { first: string[]; more: number } {
	return { first: models.slice(0, shown), more: Math.max(0, models.length - shown) };
}

import { agentsApi } from '../api/client.ts';
import { Manifest } from './doc.ts';

// Duplicate and rename: the copy gets the new identity.name (edited in place,
// so the rest of the file stays as written) and a file named after it, in the
// same folder.

export const NAME_RE = /^[a-zA-Z0-9_-]{1,64}$/;

export function siblingPath(path: string, name: string): string {
	const slash = path.lastIndexOf('/');
	const ext = path.endsWith('.yml') ? '.yml' : '.yaml';
	return (slash >= 0 ? path.slice(0, slash + 1) : '') + name + ext;
}

export async function copyAgent(from: string, name: string, move: boolean): Promise<string> {
	const file = await agentsApi.get(from);
	const m = new Manifest(file.text);
	if (m.ok) m.set(['identity', 'name'], name);
	const to = siblingPath(from, name);
	await agentsApi.create(to, m.toString());
	if (move) await agentsApi.remove(from);
	return to;
}

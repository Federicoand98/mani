import type { Problem } from '../api/types.ts';
import { blockOf } from './blocks.ts';
import { Manifest } from './doc.ts';
import { NAME_RE } from './files.ts';
import { read, write } from '../storage.ts';

// The guided creation of an agent, minus the page: its steps, the checks that
// gate each one, where the file goes, and the draft kept in the browser (M4-6).

export const STEPS = ['Start from', 'Name and model', 'Tools', 'Permissions', 'Limits', 'Check', 'Try'] as const;

export type Draft = {
	template: string;
	text: string;
	step: number;
	folder: string;
	tried: boolean;
};

const KEY = 'mani:new-agent';

export function loadDraft(): Draft | null {
	try {
		const d = JSON.parse(read(KEY) ?? 'null') as Draft | null;
		return d && typeof d.text === 'string' && typeof d.step === 'number' ? d : null;
	} catch {
		return null;
	}
}

export function saveDraft(d: Draft | null) {
	write(KEY, d ? JSON.stringify(d) : null);
}

/** Where the file will be written: the chosen folder, named after the agent (M4-3). */
export function targetPath(folder: string, name: string): string {
	const dir = folder && folder !== '.' ? folder.replace(/^\.\//, '').replace(/\/$/, '') + '/' : '';
	return `${dir}${name}.yaml`;
}

/**
 * What keeps a step from moving on, in words; empty when it may.
 * existing holds the agent files already in the folder; providers the enabled ones.
 */
export function stepBlockers(step: number, text: string, folder: string, existing: string[], providers: string[]): string[] {
	const m = new Manifest(text);
	if (!m.ok) return ['The YAML does not parse: fix it before going on.'];
	const out: string[] = [];
	if (step === 1) {
		const name = m.str(['identity', 'name']);
		if (!NAME_RE.test(name)) out.push('Give it a name: letters, digits, - and _ (it also names the file).');
		else if (existing.includes(targetPath(folder, name))) out.push(`${targetPath(folder, name)} already exists: pick another name.`);
		if (!providers.includes(m.str(['identity', 'provider']))) out.push('Pick a model provider that is connected, or add one in Providers.');
		if (!m.str(['identity', 'model'])) out.push('Pick a model.');
		if (!m.str(['identity', 'prompt']) && !m.includeOf(['identity', 'prompt'])) out.push('Say in a sentence what it must do: it is all the model knows about its job.');
	}
	return out;
}

/** The step where a problem can be fixed, for "Fix" on the Check step. */
export function stepFor(p: Problem): number {
	switch (blockOf(p)) {
		case 'identity':
			return 1;
		case 'capabilities':
			return 2;
		case 'policy':
			return 3;
		case 'limits':
			return 4;
		default:
			return 5; // context, output, run, observability: not in the guided steps
	}
}

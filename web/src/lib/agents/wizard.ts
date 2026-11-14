import type { Problem } from '../api/types.ts';
import { blockOf } from './blocks.ts';
import { Manifest } from './doc.ts';
import { NAME_RE } from './files.ts';
import { read, write } from '../storage.ts';

export const MAIN_SECTIONS = [
	['agent', 'Agent'], ['tools', 'Tools'], ['policy', 'Policy'], ['context', 'Context']
] as const;
export const ADVANCED_SECTIONS = [
	['output', 'Output'], ['limits', 'Limits'], ['run', 'Run'], ['records', 'Records'], ['file', 'File']
] as const;
export type SetupSection = typeof MAIN_SECTIONS[number][0] | typeof ADVANCED_SECTIONS[number][0];

export type Draft = {
	template: string;
	text: string;
	step: number;
	folder: string;
	tried: boolean;
	section?: SetupSection;
	filename?: string;
};

const KEY = 'mani:new-agent';

export function loadDraft(): Draft | null {
	try {
		const d: unknown = JSON.parse(read(KEY) ?? 'null');
		if (!d || typeof d !== 'object' || !('text' in d) || typeof d.text !== 'string'
			|| !('template' in d) || typeof d.template !== 'string'
			|| !('folder' in d) || typeof d.folder !== 'string') return null;
		const step = 'step' in d && typeof d.step === 'number' && Number.isInteger(d.step) ? d.step : 1;
		const legacy: SetupSection[] = ['agent', 'agent', 'tools', 'policy', 'limits', 'file', 'agent'];
		const section = 'section' in d && [...MAIN_SECTIONS, ...ADVANCED_SECTIONS].some(([key]) => key === d.section)
			? d.section as SetupSection : legacy[step] ?? 'agent';
		return { template: d.template, text: d.text, folder: d.folder, step, tried: false, section,
			filename: 'filename' in d && typeof d.filename === 'string' ? d.filename : undefined };
	} catch {
		return null;
	}
}

export function saveDraft(d: Draft | null) {
	write(KEY, d ? JSON.stringify(d) : null);
}

/** Where the file will be written: the chosen folder, named after the agent (M4-3). */
export function targetPath(folder: string, name: string, filename?: string): string {
	const dir = folder && folder !== '.' ? folder.replace(/^\.\//, '').replace(/\/$/, '') + '/' : '';
	return `${dir}${filename ?? `${name}.yaml`}`;
}

/**
 * What prevents creation or a trial; empty when the local fields are ready.
 * existing holds the agent files already in the folder; providers the enabled ones.
 */
export function creationBlockers(text: string, folder: string, existing: string[], providers: string[], filename?: string): string[] {
	const m = new Manifest(text);
	if (!m.ok) return ['The YAML does not parse: fix it before going on.'];
	const out: string[] = [];
		const name = m.str(['identity', 'name']);
		if (!NAME_RE.test(name)) out.push('Name: use 1-64 letters, digits, hyphens or underscores.');
		if (filename !== undefined && !/^[a-zA-Z0-9_-]{1,64}\.ya?ml$/.test(filename)) out.push('File name: use letters, digits, hyphens or underscores, ending in .yaml or .yml.');
		if (existing.includes(targetPath(folder, name, filename))) out.push(`${targetPath(folder, name, filename)} already exists: pick another name.`);
		if (!providers.includes(m.str(['identity', 'provider']))) out.push('Pick a model provider that is connected, or add one in Providers.');
		if (!m.str(['identity', 'model'])) out.push('Pick a model.');
		if (!m.str(['identity', 'prompt']) && !m.includeOf(['identity', 'prompt'])) out.push('Instructions are required.');
	return out;
}

/** The workbench section that owns a validation problem. */
export function sectionFor(p: Problem): SetupSection {
	switch (blockOf(p)) {
		case 'identity':
			return 'agent';
		case 'capabilities':
			return p.path.startsWith('capabilities.workspace') ? 'agent' : 'tools';
		case 'policy':
			return 'policy';
		case 'limits':
			return 'limits';
		case 'context': return 'context';
		case 'output': return 'output';
		case 'run': return 'run';
		case 'observability': return 'records';
		default:
			return 'file';
	}
}

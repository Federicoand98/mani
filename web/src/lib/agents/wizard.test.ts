import { readFileSync } from 'node:fs';
import { beforeEach, describe, expect, it } from 'vitest';
import { Manifest } from './doc.ts';
import { loadDraft, saveDraft, stepBlockers, stepFor, targetPath } from './wizard.ts';

const docs = readFileSync('../internal/templates/docs.yaml', 'utf8');

describe('the guided creation', () => {
	beforeEach(() => localStorage.clear());

	it('keeps the template comments in the file it produces', () => {
		const m = new Manifest(docs);
		m.set(['identity', 'name'], 'handbook');
		m.set(['identity', 'model'], 'llama3.2:3b');
		m.setTool('write', false);
		const out = m.toString();
		expect(out).toContain('identity:                    # who thinks?');
		expect(out).toContain('  name: handbook');
		expect(out).toContain('  workspace: .               # file tools never reach outside this folder');
		expect(out).not.toContain('- name: write');
		expect(out).toContain('    write: ask               # writing waits for your yes');
	});

	it('names the file after the agent, in the chosen folder', () => {
		expect(targetPath('.', 'handbook')).toBe('handbook.yaml');
		expect(targetPath('agents', 'handbook')).toBe('agents/handbook.yaml');
		expect(targetPath('./team/', 'x')).toBe('team/x.yaml');
	});

	it('says what keeps the name step from going on', () => {
		const ok = stepBlockers(1, docs, 'agents', [], ['ollama']);
		expect(ok).toEqual([]);
		expect(stepBlockers(1, docs, 'agents', ['agents/docs-assistant.yaml'], ['ollama']).join()).toContain('already exists');
		expect(stepBlockers(1, docs, 'agents', [], ['openai']).join()).toContain('connected');
		const bad = docs.replace('name: docs-assistant', 'name: "has spaces"');
		expect(stepBlockers(1, bad, '.', [], ['ollama']).join()).toContain('Give it a name');
		expect(stepBlockers(2, 'identity: [oops', '.', [], []).join()).toContain('does not parse');
	});

	it('sends a problem back to the step that fixes it', () => {
		expect(stepFor({ path: 'identity.model', line: 3, message: '' })).toBe(1);
		expect(stepFor({ path: 'policy.rules[0].pattern', line: 3, message: '' })).toBe(3);
		expect(stepFor({ path: 'run.triggers[0].at', line: 3, message: '' })).toBe(5);
	});

	it('keeps the draft in the browser until it is thrown away', () => {
		expect(loadDraft()).toBeNull();
		saveDraft({ template: 'docs', text: docs, step: 3, folder: 'agents', tried: false });
		expect(loadDraft()).toMatchObject({ template: 'docs', step: 3, folder: 'agents' });
		saveDraft(null);
		expect(loadDraft()).toBeNull();
		localStorage.setItem('mani:new-agent', '{broken');
		expect(loadDraft()).toBeNull();
	});
});

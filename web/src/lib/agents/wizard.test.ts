import { readFileSync } from 'node:fs';
import { beforeEach, describe, expect, it } from 'vitest';
import { Manifest } from './doc.ts';
import { loadDraft, saveDraft, creationBlockers, sectionFor, targetPath } from './wizard.ts';

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
		const ok = creationBlockers(docs, 'agents', [], ['ollama']);
		expect(ok).toEqual([]);
		expect(creationBlockers(docs, 'agents', ['agents/docs-assistant.yaml'], ['ollama']).join()).toContain('already exists');
		expect(creationBlockers(docs, 'agents', [], ['openai']).join()).toContain('connected');
		const bad = docs.replace('name: docs-assistant', 'name: "has spaces"');
		expect(creationBlockers(bad, '.', [], ['ollama']).join()).toContain('Name:');
		expect(creationBlockers('identity: [oops', '.', [], []).join()).toContain('does not parse');
		expect(creationBlockers(docs, '.', [], ['ollama'], '../escape.yaml').join()).toContain('File name:');
	});

	it('sends a problem back to the step that fixes it', () => {
		expect(sectionFor({ path: 'identity.model', line: 3, message: '' })).toBe('agent');
		expect(sectionFor({ path: 'policy.rules[0].pattern', line: 3, message: '' })).toBe('policy');
		expect(sectionFor({ path: 'run.triggers[0].at', line: 3, message: '' })).toBe('run');
	});

	it('keeps the draft in the browser until it is thrown away', () => {
		expect(loadDraft()).toBeNull();
		saveDraft({ template: 'docs', text: docs, step: 3, folder: 'agents', tried: false });
		expect(loadDraft()).toMatchObject({ template: 'docs', step: 3, folder: 'agents' });
		expect(loadDraft()?.section).toBe('policy');
		saveDraft(null);
		expect(loadDraft()).toBeNull();
		localStorage.setItem('mani:new-agent', '{broken');
		expect(loadDraft()).toBeNull();
	});

	it('restores workbench sections and custom file names without claiming an old trial succeeded', () => {
		saveDraft({ template: 'docs', text: docs, step: 1, folder: '.', tried: true, section: 'output', filename: 'custom.yml' });
		expect(loadDraft()).toMatchObject({ section: 'output', filename: 'custom.yml', tried: false });
		expect(targetPath('agents', 'handbook', 'custom.yml')).toBe('agents/custom.yml');
		localStorage.setItem('mani:new-agent', '{"text":"","folder":42,"template":"docs"}');
		expect(loadDraft()).toBeNull();
	});
});

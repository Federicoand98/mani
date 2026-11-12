import { describe, expect, it } from 'vitest';
import { findModel, parseCommand } from './commands.ts';

describe('parseCommand', () => {
	it('leaves plain text alone', () => {
		expect(parseCommand('hello /help')).toBeNull();
	});
	it('reads a command and its argument', () => {
		expect(parseCommand('/model  llama3.2:3b ')).toEqual({ name: 'model', arg: 'llama3.2:3b' });
		expect(parseCommand('/HELP')).toEqual({ name: 'help', arg: '' });
	});
	it('names an unknown command', () => {
		expect(parseCommand('/login openai')).toEqual({ error: 'Unknown command /login. Type /help for the list.' });
	});
});

describe('findModel', () => {
	const models = { ollama: ['qwen3.5:9b', 'shared'], openai: ['gpt-4.1', 'shared'] };
	it('finds a model in any provider', () => {
		expect(findModel('gpt-4.1', models, 'ollama')).toEqual({ provider: 'openai', model: 'gpt-4.1' });
	});
	it('prefers the current provider when two offer the same name', () => {
		expect(findModel('shared', models, 'openai')).toEqual({ provider: 'openai', model: 'shared' });
	});
	it('returns null for a model nobody offers', () => {
		expect(findModel('nope', models, 'ollama')).toBeNull();
	});
});

// The slash commands of the chat: the ones from the TUI that make sense in a tab.

export type Command = { name: 'help' | 'clear' | 'model' | 'thinking'; arg: string };

export const commandHelp = [
	['/help', 'show this list'],
	['/clear', 'start a new conversation with the same model'],
	['/model <name>', 'switch the model of this conversation'],
	['/thinking', 'show or hide the reasoning of the model']
] as const;

/** null when the text is a plain message; an error string for an unknown command. */
export function parseCommand(text: string): Command | { error: string } | null {
	const t = text.trim();
	if (!t.startsWith('/')) return null;
	const [head, ...rest] = t.split(/\s+/);
	const name = head.slice(1).toLowerCase();
	if (name === 'help' || name === 'clear' || name === 'model' || name === 'thinking') {
		return { name, arg: rest.join(' ') };
	}
	return { error: `Unknown command ${head}. Type /help for the list.` };
}

/** Finds a model by name among the providers' lists, preferring the current provider. */
export function findModel(
	name: string,
	models: Record<string, string[]>,
	current: string
): { provider: string; model: string } | null {
	const order = [current, ...Object.keys(models).filter((p) => p !== current)];
	for (const p of order) {
		if (models[p]?.includes(name)) return { provider: p, model: name };
	}
	return null;
}

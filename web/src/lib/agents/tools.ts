// What each built-in tool does and how risky it is, as the editor shows them.
export const TOOL_INFO: Record<string, { about: string; risk: 'none' | 'network' | 'write' | 'execute' }> = {
	read: { about: 'Reads a file in the workspace', risk: 'none' },
	glob: { about: 'Finds files by name pattern', risk: 'none' },
	grep: { about: 'Searches text in files', risk: 'none' },
	planning: { about: 'Keeps a list of steps', risk: 'none' },
	write: { about: 'Creates or overwrites a file', risk: 'write' },
	edit: { about: 'Changes part of a file', risk: 'write' },
	delete: { about: 'Deletes a file', risk: 'write' },
	fetch: { about: 'Downloads a web page (allowed hosts only)', risk: 'network' },
	bash: { about: 'Runs a command: it can do anything you can', risk: 'execute' },
	delegate: { about: 'Hands a task to a subagent', risk: 'none' }
};

export const RISK_LABEL = { none: 'None', network: 'Network', write: 'Write', execute: 'Execute' } as const;
export const RISK_CLASS = { none: 'muted', network: 'warn', write: 'warn', execute: 'error' } as const;

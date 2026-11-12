// Every string the console shows. One catalog per language: adding a language
// is adding a file with the same shape, then a selector in Settings.

export const en = {
	app: {
		skip: 'Skip to content',
		menu: 'Open the menu',
		workspace: 'Working directory'
	},
	nav: {
		home: 'Home',
		groupBuild: 'Build',
		chat: 'Chat',
		groupSystem: 'System',
		providers: 'Providers',
		settings: 'Settings'
	},
	status: {
		connected: 'Local · connected',
		offline: 'Not reachable',
		checking: 'Checking…'
	},
	offline: {
		title: "Can't reach mani.",
		body: 'The local server is not answering. Nothing is lost: your agents and settings are files on disk.',
		retry: 'Try again',
		command: 'mani web'
	},
	signedOut: {
		title: 'Sign in from the terminal',
		body: 'For your safety, the console only opens from the link that mani web prints when it starts. Open that link in this browser, or restart mani web to get a new one.'
	},
	home: {
		title: 'Home',
		subtitle: (dir: string) => `Working on ${dir}`,
		getStarted: 'Get started',
		connect: 'Connect a model',
		connectDone: (n: number) => `${n} provider${n === 1 ? '' : 's'} connected`,
		connectTodo: 'Agents need a model to think. Ollama runs on this machine and costs nothing.',
		chat: 'Chat with a model',
		chatTodo: 'Ask something, paste a screenshot. Conversations are saved in this folder.',
		chatDone: (n: number) => `${n} conversation${n === 1 ? '' : 's'}`,
		next: 'Coming next in the console',
		nextItems: ['Agents: list, edit and test', 'Runs and approvals', 'Flows'],
		done: 'Done'
	},
	providers: {
		title: 'Providers',
		subtitle: 'The services that make your agents think. Keys are stored in auth.json on this machine, never in agent files.',
		add: 'Add provider',
		emptyTitle: 'No provider connected',
		emptyBody:
			'A provider is the service that makes your agents think. With Ollama everything stays on this machine and costs nothing; OpenAI and Anthropic need an API key.',
		colProvider: 'Provider',
		colWhere: 'Runs',
		colModels: 'Models',
		local: 'On this machine',
		cloud: 'In the cloud',
		test: 'Test',
		testing: 'Testing…',
		remove: 'Remove',
		confirmRemove: 'Confirm',
		cancel: 'Cancel',
		testOk: (n: number) => `Connected · ${n} model${n === 1 ? '' : 's'}`,
		models: (n: number) => `${n} model${n === 1 ? '' : 's'}`,
		more: (n: number) => `+${n}`,
		added: (label: string) => `${label} added`,
		removed: (label: string) => `${label} removed. Its key was deleted from auth.json.`
	},
	addProvider: {
		title: 'Add a provider',
		pickBody: 'Choose where the model runs. You can add more later.',
		alreadyAdded: 'Already added',
		recommended: 'Recommended to start',
		about: {
			ollama: 'Runs models on this machine. No cost, no data leaves it.',
			openai: 'GPT models, with an API key from platform.openai.com.',
			anthropic: 'Claude models, with an API key from console.anthropic.com.',
			openrouter: 'Many models behind one key from openrouter.ai.',
			copilot: 'The models of your GitHub Copilot subscription, through your GitHub account.'
		} as Record<string, string>,
		back: 'Back',
		cancel: 'Cancel',
		url: 'Address',
		urlHelp: 'Where Ollama listens. The default is http://localhost:11434.',
		find: 'Find Ollama',
		finding: 'Looking…',
		found: (n: number) => `Found. ${n} model${n === 1 ? '' : 's'} already downloaded:`,
		noModels: 'Ollama answers, but has no models yet. Download one with',
		install: "Ollama isn't installed? Get it from ollama.com, then download a model with",
		key: 'API key',
		keyHelp: 'Stored in auth.json on this machine, readable only by you.',
		endpoint: 'Custom endpoint',
		endpointHelp: 'Leave it as it is unless you use a proxy or a compatible service.',
		testAndAdd: 'Test and add',
		add: 'Add',
		working: 'Testing…',
		refused: (label: string) =>
			`${label} refused the connection. Nothing was saved, and no other provider is tried in its place.`,
		deviceIntro: 'Open the GitHub page below and type this code:',
		deviceWaiting: 'Waiting for you to authorize on GitHub…',
		deviceExpires: (min: number) => `The code expires in ${min} minutes.`,
		copy: 'Copy',
		copied: 'Copied',
		openGitHub: 'Open GitHub'
	},
	chat: {
		title: 'Chat',
		newChat: 'New conversation',
		listLabel: 'Conversations',
		listHelp: 'Saved in .mani/sessions of this folder.',
		untitled: 'New conversation',
		rename: 'Rename',
		delete: 'Delete',
		confirmDelete: 'Delete',
		cancel: 'Cancel',
		renameLabel: 'Conversation title',
		noProviderTitle: 'Connect a model to start',
		noProviderBody: 'The chat talks to a model through a provider: Ollama on this machine, or a service like OpenAI or Anthropic.',
		goProviders: 'Go to Providers',
		helloTitle: (model: string) => `Talk with ${model}`,
		helloBody:
			'Just the model: it cannot read your files or run commands. Agents with tools and permissions come later in the console.',
		suggestions: ['Explain what a webhook is, in two paragraphs', 'Write a polite reminder for an unpaid invoice', 'Summarize the pros and cons of SQLite'],
		placeholder: 'Write a message…',
		send: 'Send',
		stop: 'Stop',
		attach: 'Attach images',
		dropImages: 'Drop images to attach them to your message',
		removeImage: (name: string) => `Remove ${name}`,
		model: 'Model',
		hint: 'Enter to send · Shift+Enter for a new line · /help for commands',
		you: 'You',
		thinking: 'Reasoning',
		thinkingFor: (s: string) => `Reasoning · ${s}`,
		thinkingNow: 'Reasoning…',
		busyTitle: 'Still answering.',
		busyBody: 'The model is writing the reply in the background. It appears here when it is done.',
		lostTitle: 'Connection lost.',
		lostBody: 'A reply in progress keeps going and is saved.',
		reconnect: 'Reconnect',
		forgotten: 'The model does not remember the messages above this line.',
		forgottenHelp: 'Change how many it keeps in Settings.',
		stopped: 'Stopped. The reply so far is saved.',
		usage: (inp: number, out: number, secs: string) => `${inp.toLocaleString('en')} in · ${out.toLocaleString('en')} out · ${secs}`,
		modelChanged: (model: string) => `From here, ${model} answers.`,
		help: 'Commands',
		noSuchModel: (name: string) => `No connected provider offers ${name}. Nothing changed.`,
		modelNow: (model: string, list: string) => `Model: ${model}. Available: ${list}`,
		thinkingOn: 'Reasoning blocks now start open.',
		thinkingOff: 'Reasoning blocks now start closed.',
		deleted: 'Conversation deleted',
		copy: 'Copy',
		copied: 'Copied'
	},
	settings: {
		title: 'Settings',
		subtitle: 'Preferences of this console. They are kept in this browser.',
		appearance: 'Appearance',
		theme: 'Theme',
		themes: { system: 'System', light: 'Light', dark: 'Dark' },
		reduceMotion: 'Reduce motion',
		chat: 'Chat',
		keep: 'Conversation memory',
		keepHelp: (min: number, max: number) =>
			`How many recent messages the model remembers, from ${min} to ${max}. Older messages stay on screen.`,
		language: 'Language',
		languageHelp: 'More languages will come later.'
	},
	errors: {
		unreachable: "mani isn't answering. Is mani web still running?",
		required: (what: string) => `Enter the ${what}.`,
		badUrl: 'Enter an address like http://localhost:11434.'
	}
};

export type Messages = typeof en;
export const m: Messages = en;

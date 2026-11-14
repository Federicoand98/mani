import type {
	AgentFile,
	AgentList,
	AgentWrite,
	APIError,
	CheckResult,
	Draft,
	TemplateList,
	FolderList,
	NativeFolderResult,
	Conversation,
	ConversationInput,
	ConversationList,
	DeviceStart,
	DeviceStatus,
	Health,
	ModelList,
	ModelLimits,
	Settings,
	Provider,
	ProviderInput,
	ProviderList,
	RunDetail,
	RunList,
	TestResult
} from './types.ts';

/** status is 0 when mani did not answer at all. */
export class ApiError extends Error {
	constructor(
		readonly status: number,
		message: string
	) {
		super(message);
		this.name = 'ApiError';
	}
}

export async function call<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
	let res: Response;
	try {
		res = await fetch('/api' + path, {
			method,
			signal,
			credentials: 'same-origin',
			headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
			body: body === undefined ? undefined : JSON.stringify(body)
		});
	} catch (e) {
		if (signal?.aborted) throw e;
		throw new ApiError(0, "mani isn't answering");
	}
	let data: unknown = null;
	try {
		data = await res.json();
	} catch {
		// an empty or non-JSON body: the status says enough
	}
	if (!res.ok) {
		const msg = (data as APIError | null)?.error ?? `${res.status} ${res.statusText}`;
		throw new ApiError(res.status, msg);
	}
	return data as T;
}

const p = (name: string) => '/providers/' + encodeURIComponent(name);

export const api = {
	health: () => call<Health>('GET', '/health'),
	settings: () => call<Settings>('GET', '/settings'),
	saveSettings: (input: Settings) => call<Settings>('PUT', '/settings', input),
	providers: () => call<ProviderList>('GET', '/providers'),
	saveProvider: (name: string, input: ProviderInput) => call<Provider>('PUT', p(name), input),
	removeProvider: (name: string) => call<Provider>('DELETE', p(name)),
	testProvider: (name: string, input: ProviderInput) => call<TestResult>('POST', p(name) + '/test', input),
	models: (name: string) => call<ModelList>('GET', p(name) + '/models'),
	modelLimits: (name: string, model: string) => call<ModelLimits>('POST', p(name) + '/model-limits', { model }),
	deviceStart: () => call<DeviceStart>('POST', '/providers/copilot/device'),
	deviceStatus: () => call<DeviceStatus>('GET', '/providers/copilot/device'),
	conversations: () => call<ConversationList>('GET', '/conversations'),
	conversation: (id: string) => call<Conversation>('GET', c(id)),
	createConversation: (input: ConversationInput) => call<Conversation>('POST', '/conversations', input),
	updateConversation: (id: string, input: ConversationInput) => call<Conversation>('PATCH', c(id), input),
	deleteConversation: (id: string) => call<null>('DELETE', c(id))
};

const a = (path: string) => '/agents/file/' + path.split('/').map(encodeURIComponent).join('/');

export const agentsApi = {
	list: () => call<AgentList>('GET', '/agents'),
	get: (path: string) => call<AgentFile>('GET', a(path)),
	save: (path: string, body: AgentWrite) => call<AgentFile>('PUT', a(path), body),
	create: (path: string, text: string) => call<AgentFile>('POST', a(path), { text }),
	remove: (path: string) => call<null>('DELETE', a(path)),
	check: (text: string, path: string) => call<CheckResult>('POST', '/agents/check', { text, path }),
	folders: (path: string) => call<FolderList>('GET', '/folders?path=' + encodeURIComponent(path)),
	pickFolder: (path: string, signal?: AbortSignal) => call<NativeFolderResult>('POST', '/folders/pick', { path }, signal),
	templates: () => call<TemplateList>('GET', '/templates'),
	draft: (text: string, folder: string) => call<Draft>('POST', '/drafts', { text, folder }),
	endDraft: (id: string) => call<null>('DELETE', '/drafts/' + encodeURIComponent(id))
};

const c = (id: string) => '/conversations/' + encodeURIComponent(id);

export const runsApi = {
	/** query comes from apiQuery in #lib/runs/runs.ts. */
	list: (query: string, signal?: AbortSignal) => call<RunList>('GET', '/runs' + query, undefined, signal),
	get: (id: string, after = -1, signal?: AbortSignal) =>
		call<RunDetail>('GET', '/runs/' + encodeURIComponent(id) + (after >= 0 ? '?after=' + after : ''), undefined, signal)
};

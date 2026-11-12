import type {
	APIError,
	Conversation,
	ConversationInput,
	ConversationList,
	DeviceStart,
	DeviceStatus,
	Health,
	ModelList,
	Provider,
	ProviderInput,
	ProviderList,
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

export async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
	let res: Response;
	try {
		res = await fetch('/api' + path, {
			method,
			credentials: 'same-origin',
			headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
			body: body === undefined ? undefined : JSON.stringify(body)
		});
	} catch {
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
	providers: () => call<ProviderList>('GET', '/providers'),
	saveProvider: (name: string, input: ProviderInput) => call<Provider>('PUT', p(name), input),
	removeProvider: (name: string) => call<Provider>('DELETE', p(name)),
	testProvider: (name: string, input: ProviderInput) => call<TestResult>('POST', p(name) + '/test', input),
	models: (name: string) => call<ModelList>('GET', p(name) + '/models'),
	deviceStart: () => call<DeviceStart>('POST', '/providers/copilot/device'),
	deviceStatus: () => call<DeviceStatus>('GET', '/providers/copilot/device'),
	conversations: () => call<ConversationList>('GET', '/conversations'),
	conversation: (id: string) => call<Conversation>('GET', c(id)),
	createConversation: (input: ConversationInput) => call<Conversation>('POST', '/conversations', input),
	updateConversation: (id: string, input: ConversationInput) => call<Conversation>('PATCH', c(id), input),
	deleteConversation: (id: string) => call<null>('DELETE', c(id))
};

const c = (id: string) => '/conversations/' + encodeURIComponent(id);

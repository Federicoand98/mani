import type { ClientMsg, ServerMsg } from '../api/wire.ts';

// One WebSocket per open conversation. The reply does not depend on it (M2-4):
// if it drops, the page reconnects and mani says whether a reply is still busy.

export type SocketState = 'connecting' | 'open' | 'lost';

export class ChatSocket {
	private ws: WebSocket | null = null;
	private closed = false;
	private retries = 0;
	private opened: Promise<void> = Promise.resolve();

	constructor(
		private id: string,
		private onMessage: (m: ServerMsg) => void,
		private onState: (s: SocketState) => void
	) {
		this.connect();
	}

	private connect() {
		const url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/api/conversations/${encodeURIComponent(this.id)}/ws`;
		const ws = new WebSocket(url);
		this.ws = ws;
		this.onState('connecting');
		this.opened = new Promise((resolve, reject) => {
			ws.onopen = () => {
				this.retries = 0;
				this.onState('open');
				resolve();
			};
			ws.onerror = () => reject(new Error('connection failed'));
		});
		this.opened.catch(() => {}); // a failed open is handled by onclose
		ws.onmessage = (e) => {
			try {
				this.onMessage(JSON.parse(e.data));
			} catch {
				// a malformed frame is ignored; the next one may be fine
			}
		};
		ws.onclose = () => {
			if (this.closed) return;
			if (this.retries < 3) {
				const wait = 500 * 2 ** this.retries++;
				setTimeout(() => !this.closed && this.connect(), wait);
			} else {
				this.onState('lost');
			}
		};
	}

	/** Reconnect after the page gave up (the "Reconnect" button). */
	retry() {
		this.retries = 0;
		this.connect();
	}

	async send(m: ClientMsg) {
		await this.opened;
		this.ws?.send(JSON.stringify(m));
	}

	close() {
		this.closed = true;
		this.ws?.close();
	}
}

import { agentsApi, ApiError } from '../api/client.ts';
import type { Problem } from '../api/types.ts';
import { Manifest, type Path } from './doc.ts';

// A manifest being edited: its text, the problems mani finds in it, and reads
// that touch `rev`, so the fields re-render when the document changes. Writes
// go through edit(). The editor of a saved file (AgentEditor) and the guided
// creation of a new one share it.

export class ManifestState {
	/** Where the file is, or will be: problems are checked against its folder. */
	path: string;
	text = $state('');
	problems = $state<Problem[]>([]);
	rev = $state(0);
	m = new Manifest('');

	private checkTimer: ReturnType<typeof setTimeout> | undefined;

	constructor(path: string, text = '') {
		this.path = path;
		if (text) {
			this.text = text;
			this.m = new Manifest(text);
		}
	}

	get parses() {
		void this.rev;
		return this.m.ok;
	}

	// --- reactive reads ---
	str = (p: Path) => (void this.rev, this.m.str(p));
	num = (p: Path) => (void this.rev, this.m.num(p));
	bool = (p: Path, fallback = false) => (void this.rev, this.m.bool(p, fallback));
	has = (p: Path) => (void this.rev, this.m.has(p));
	list = (p: Path) => (void this.rev, this.m.list(p));
	strings = (p: Path) => (void this.rev, this.m.strings(p));
	map = (p: Path) => (void this.rev, this.m.map(p));
	includeOf = (p: Path) => (void this.rev, this.m.includeOf(p));
	toolNames = () => (void this.rev, this.m.toolNames());
	customTools = () => (void this.rev, this.m.customTools());

	/** Applies a change from a field to the document and the text. */
	edit(fn: (m: Manifest) => void) {
		if (!this.m.ok) return;
		fn(this.m);
		this.text = this.m.toString();
		this.rev++;
		this.scheduleCheck();
	}

	set(p: Path, v: unknown) {
		this.edit((m) => m.set(p, v));
	}

	/** The text changed in the YAML panel. */
	setText(text: string) {
		if (text === this.text) return;
		this.text = text;
		this.m = new Manifest(text);
		this.rev++;
		this.scheduleCheck();
	}

	scheduleCheck() {
		clearTimeout(this.checkTimer);
		this.checkTimer = setTimeout(() => this.check(), 350);
	}

	async check() {
		try {
			this.problems = (await agentsApi.check(this.text, this.path)).problems;
		} catch {
			// the next edit checks again
		}
	}
}

// The editor of a manifest saved on disk: load, save with a version, and watch
// the file for changes made elsewhere (M3-6).
export class AgentEditor extends ManifestState {
	saved = $state('');
	version = $state('');
	loading = $state(true);
	saving = $state(false);
	error = $state('');
	/** The file changed on disk while there were local edits (M3-6). */
	external = $state<{ text: string; version: string } | null>(null);
	/** A save was refused because the file changed since it was loaded. */
	conflict = $state(false);

	private pollTimer: ReturnType<typeof setInterval> | undefined;

	get dirty() {
		return this.text !== this.saved;
	}

	private adopt(text: string, version: string, problems: Problem[]) {
		this.text = this.saved = text;
		this.version = version;
		this.problems = problems;
		this.m = new Manifest(text);
		this.rev++;
	}

	async load() {
		this.loading = true;
		try {
			const f = await agentsApi.get(this.path);
			this.adopt(f.text, f.version, f.problems);
			this.error = '';
		} catch (e) {
			this.error = (e as Error).message;
		} finally {
			this.loading = false;
		}
	}

	/** Saves; returns false when the file changed on disk since it was loaded. */
	async save(force = false): Promise<boolean> {
		this.saving = true;
		try {
			const f = await agentsApi.save(this.path, { text: this.text, version: this.version, force });
			this.adopt(f.text, f.version, f.problems);
			this.conflict = false;
			this.external = null;
			return true;
		} catch (e) {
			if (e instanceof ApiError && e.status === 409) {
				this.conflict = true;
				return false;
			}
			throw e;
		} finally {
			this.saving = false;
		}
	}

	/** Throws away local edits and takes the file as it is on disk. */
	async reload() {
		this.conflict = false;
		this.external = null;
		await this.load();
	}

	/** Watches the file: reloads when clean, warns when there are edits (M3-6). */
	watch() {
		this.pollTimer = setInterval(async () => {
			if (document.visibilityState !== 'visible' || this.saving) return;
			try {
				const f = await agentsApi.get(this.path);
				if (f.version === this.version) return;
				if (!this.dirty) this.adopt(f.text, f.version, f.problems);
				else this.external = { text: f.text, version: f.version };
			} catch {
				// deleted or unreachable: the next save reports it
			}
		}, 3000);
		return () => clearInterval(this.pollTimer);
	}
}

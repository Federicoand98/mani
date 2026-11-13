import { yaml } from '@codemirror/lang-yaml';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { lintGutter, setDiagnostics, type Diagnostic } from '@codemirror/lint';
import { EditorState } from '@codemirror/state';
import { tags } from '@lezer/highlight';
import { basicSetup, EditorView } from 'codemirror';
import type { Problem } from '../api/types.ts';

// The YAML panel. Loaded with a dynamic import, so CodeMirror reaches the
// browser only when an agent is opened.

const highlight = HighlightStyle.define([
	{ tag: [tags.propertyName, tags.definition(tags.propertyName)], color: '#9A97FF' },
	{ tag: [tags.string, tags.special(tags.string)], color: '#5CC8D2' },
	{ tag: [tags.number, tags.bool, tags.null], color: '#F2B14C' },
	{ tag: [tags.comment, tags.lineComment], color: '#8C92AE', fontStyle: 'italic' },
	{ tag: [tags.typeName, tags.tagName, tags.labelName, tags.meta], color: '#FF8A80' },
	{ tag: [tags.punctuation, tags.separator, tags.squareBracket, tags.brace], color: '#A3A9C2' }
]);

const theme = EditorView.theme(
	{
		'&': { backgroundColor: 'var(--code-bg)', color: 'var(--code-tx)', height: '100%', fontSize: '12.5px' },
		'.cm-scroller': { fontFamily: 'var(--f-mono)', lineHeight: '20px' },
		'.cm-gutters': { backgroundColor: 'var(--code-bg)', color: 'var(--code-mu)', border: 'none' },
		'.cm-activeLine, .cm-activeLineGutter': { backgroundColor: 'rgba(255,255,255,0.05)' },
		'.cm-cursor': { borderLeftColor: '#ECEEF8' },
		'&.cm-focused .cm-selectionBackground, .cm-selectionBackground': { backgroundColor: '#2E3347 !important' },
		'&.cm-focused': { outline: 'none' }
	},
	{ dark: true }
);

export type YamlView = {
	setText(text: string): void;
	setProblems(ps: Problem[]): void;
	gotoLine(line: number): void;
	destroy(): void;
};

export function createYamlView(parent: HTMLElement, text: string, onChange: (text: string) => void): YamlView {
	let applying = false;
	const view = new EditorView({
		parent,
		state: EditorState.create({
			doc: text,
			extensions: [
				basicSetup,
				yaml(),
				theme,
				syntaxHighlighting(highlight),
				lintGutter(),
				EditorView.lineWrapping,
				EditorView.contentAttributes.of({ 'aria-label': 'Manifest YAML' }),
				EditorView.updateListener.of((u) => {
					if (u.docChanged && !applying) onChange(u.state.doc.toString());
				})
			]
		})
	});

	return {
		setText(next) {
			const current = view.state.doc.toString();
			if (next === current) return;
			// Replace only the changed span, so the cursor and scroll stay put.
			let start = 0;
			while (start < current.length && start < next.length && current[start] === next[start]) start++;
			let endA = current.length;
			let endB = next.length;
			while (endA > start && endB > start && current[endA - 1] === next[endB - 1]) {
				endA--;
				endB--;
			}
			applying = true;
			view.dispatch({ changes: { from: start, to: endA, insert: next.slice(start, endB) } });
			applying = false;
		},
		setProblems(ps) {
			const doc = view.state.doc;
			const diags: Diagnostic[] = ps.map((p) => {
				const line = doc.line(Math.min(Math.max(p.line || 1, 1), doc.lines));
				return { from: line.from, to: line.to, severity: 'error', message: p.message };
			});
			view.dispatch(setDiagnostics(view.state, diags));
		},
		gotoLine(n) {
			const line = view.state.doc.line(Math.min(Math.max(n, 1), view.state.doc.lines));
			view.dispatch({ selection: { anchor: line.from }, effects: EditorView.scrollIntoView(line.from, { y: 'center' }) });
			view.focus();
		},
		destroy() {
			view.destroy();
		}
	};
}

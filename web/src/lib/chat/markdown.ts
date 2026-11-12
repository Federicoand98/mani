import DOMPurify from 'dompurify';
import { Marked } from 'marked';

// Replies come from a model, so they are untrusted: Markdown becomes HTML here
// and DOMPurify removes anything that could run (scripts, event handlers,
// javascript: links) before it reaches the page.

const escape = (s: string) =>
	s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!);

const marked = new Marked({
	gfm: true,
	breaks: true,
	renderer: {
		// A code block gets a Copy button; the click is handled by the page.
		code({ text, lang }) {
			const label = `<span class="code-lang">${lang ? escape(lang) : ''}</span>`;
			return `<div class="code-block"><div class="code-head">${label}<button type="button" class="code-copy" data-copy>Copy</button></div><pre><code>${escape(text)}</code></pre></div>`;
		}
	}
});

export function renderMarkdown(src: string): string {
	const html = marked.parse(src, { async: false }) as string;
	return DOMPurify.sanitize(html, {
		ADD_ATTR: ['data-copy', 'target'],
		FORBID_TAGS: ['style', 'form', 'input'],
	});
}

// Links open in a new tab and never hand the page to the target.
DOMPurify.addHook('afterSanitizeAttributes', (node) => {
	if (node.tagName === 'A' && node.getAttribute('href')) {
		node.setAttribute('target', '_blank');
		node.setAttribute('rel', 'noopener noreferrer');
	}
});

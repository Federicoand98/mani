import { describe, expect, it } from 'vitest';
import { renderMarkdown } from './markdown.ts';

// Replies come from a model: nothing in them may run in the console.
describe('renderMarkdown', () => {
	it('renders the usual Markdown', () => {
		const html = renderMarkdown('# Title\n\n- **one**\n- two\n\n| a | b |\n|---|---|\n| 1 | 2 |');
		expect(html).toContain('<h1>Title</h1>');
		expect(html).toContain('<strong>one</strong>');
		expect(html).toContain('<table>');
	});

	it.each([
		['a script tag', '<script>alert(1)</script>hello', '<script'],
		['an event handler', '<img src=x onerror="alert(1)">', 'onerror'],
		['a javascript: link', '[click](javascript:alert(1))', 'javascript:'],
		['an iframe', '<iframe src="https://evil.example"></iframe>', '<iframe'],
		['a form', '<form action="https://evil.example"><input name=x></form>', '<form']
	])('removes %s', (_, src, bad) => {
		expect(renderMarkdown(src).toLowerCase()).not.toContain(bad);
	});

	it('puts code in an escaped block with a Copy button', () => {
		const html = renderMarkdown('```go\nfmt.Println("<b>")\n```');
		expect(html).toContain('data-copy');
		expect(html).toContain('&lt;b&gt;');
		expect(html).toContain('class="code-lang">go<');
	});

	it('opens links in a new tab without handing over the page', () => {
		const html = renderMarkdown('[docs](https://example.com)');
		expect(html).toContain('target="_blank"');
		expect(html).toContain('rel="noopener noreferrer"');
	});
});

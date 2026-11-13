import { readFileSync, writeFileSync, existsSync, mkdirSync } from 'node:fs';
import { join } from 'node:path';
import { expect, test } from './fixtures.ts';

const AGENT = `# Reviews pull requests. This comment must survive every save.
identity:
  name: reviewer      # kept short on purpose
  provider: ollama
  model: qwen3.5:9b
  prompt: "Review the diff."
capabilities:
  tools: [read, bash]
policy:
  tools:
    read: allow
`;

function seed(dir: string, rel = 'agents/reviewer.yaml', body = AGENT) {
	const p = join(dir, rel);
	mkdirSync(join(p, '..'), { recursive: true });
	writeFileSync(p, body);
	return p;
}

test('change a permission in the form: only that line changes on disk', async ({ page, mani }) => {
	const file = seed(mani.home);
	await page.goto(mani.link);
	await page.getByRole('navigation', { name: 'Main' }).getByRole('link', { name: 'Agents' }).click();
	await page.getByRole('link', { name: 'reviewer' }).click();
	await expect(page.getByRole('heading', { name: 'reviewer', level: 1 })).toBeVisible();

	await page.getByRole('button', { name: /Permissions/ }).click();
	await page.getByRole('group', { name: 'Decision for bash' }).getByRole('button', { name: 'Deny' }).click();
	await expect(page.getByText('Unsaved changes')).toBeVisible();
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Saved', { exact: true })).toBeVisible();

	const disk = readFileSync(file, 'utf8');
	expect(disk).toBe(AGENT.replace('    read: allow\n', '    read: allow\n    bash: deny\n'));
});

test('a problem is listed with its line and leads to its field', async ({ page, mani }) => {
	seed(mani.home);
	await page.goto(mani.link);
	await page.goto(mani.url + '/agents/agents/reviewer.yaml');
	await page.getByRole('button', { name: /Limits/ }).click();
	await page.getByLabel(/Longest run/).fill('2 minutes');
	await expect(page.getByText(/1 problem/)).toBeVisible();
	await expect(page.getByText(/Line \d+\..*max_duration/)).toBeVisible();

	await page.getByRole('button', { name: /Name and model/ }).click();
	await page.getByRole('button', { name: 'Go to field' }).click();
	await expect(page.getByLabel(/Longest run/)).toBeFocused();
});

test('Edit as YAML switches views, and both show the same file', async ({ page, mani }) => {
	seed(mani.home);
	await page.goto(mani.link);
	await page.goto(mani.url + '/agents/agents/reviewer.yaml');
	await page.locator('#f-name').fill('critic');

	await page.getByRole('button', { name: 'Edit as YAML' }).click();
	const yaml = page.locator('.cm-content');
	await expect(yaml).toContainText('name: critic      # kept short on purpose');
	await expect(page.locator('#f-name')).toHaveCount(0);

	await yaml.click();
	await page.keyboard.press('ControlOrMeta+End');
	await page.keyboard.type('limits:\n  max_tool_calls: 7\n');
	await page.getByRole('button', { name: 'Edit as form' }).click();
	await page.getByRole('button', { name: /Limits/ }).click();
	await expect(page.getByLabel(/Tool calls per run/)).toHaveValue('7');

	// The choice is remembered for the next agent opened.
	await page.getByRole('button', { name: 'Edit as YAML' }).click();
	await page.reload();
	await expect(page.locator('.cm-content')).toBeVisible();
});

test('a change on disk: reloads when clean, warns and refuses a stale save when not', async ({ page, mani }) => {
	const file = seed(mani.home);
	await page.goto(mani.link);
	await page.goto(mani.url + '/agents/agents/reviewer.yaml');
	await expect(page.locator('#f-name')).toHaveValue('reviewer');

	writeFileSync(file, AGENT.replace('Review the diff.', 'Review it twice.'));
	await expect(page.locator('#f-prompt')).toHaveValue('Review it twice.', { timeout: 8000 });

	await page.locator('#f-desc').fill('local edit');
	writeFileSync(file, AGENT.replace('Review the diff.', 'Someone else.'));
	await expect(page.getByText('This file changed on disk.')).toBeVisible({ timeout: 8000 });
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText(/Not saved: the file changed on disk/)).toBeVisible();
	await page.getByRole('button', { name: 'Overwrite with mine' }).click();
	await expect(page.getByText('Saved', { exact: true })).toBeVisible();
	expect(readFileSync(file, 'utf8')).toContain('description: local edit');
});

test('duplicate, rename and delete from the list', async ({ page, mani }) => {
	seed(mani.home);
	await page.goto(mani.link);
	await page.goto(mani.url + '/agents');

	// A row by the file it shows: names alone overlap (reviewer, reviewer-strict).
	const row = (name: string) => page.getByRole('row').filter({ has: page.getByText(`agents/${name}.yaml`, { exact: true }) });
	await row('reviewer').getByRole('button', { name: 'Duplicate reviewer' }).click();
	await page.getByLabel('New name').fill('reviewer-strict');
	await row('reviewer').getByRole('button', { name: 'Duplicate', exact: true }).click();
	await expect(page.getByText('Created agents/reviewer-strict.yaml')).toBeVisible();
	const copy = readFileSync(join(mani.home, 'agents/reviewer-strict.yaml'), 'utf8');
	expect(copy).toBe(AGENT.replace('name: reviewer      #', 'name: reviewer-strict      #'));

	await row('reviewer-strict').getByRole('button', { name: 'Rename reviewer-strict' }).click();
	await page.getByLabel('New name').fill('strict');
	await row('reviewer-strict').getByRole('button', { name: 'Rename', exact: true }).click();
	await expect(page.getByText('Renamed to agents/strict.yaml')).toBeVisible();
	expect(existsSync(join(mani.home, 'agents/reviewer-strict.yaml'))).toBe(false);

	await row('strict').getByRole('button', { name: 'Delete strict' }).click();
	await row('strict').getByRole('button', { name: 'Delete', exact: true }).last().click();
	await expect(page.getByText('agents/strict.yaml deleted')).toBeVisible();
	expect(existsSync(join(mani.home, 'agents/strict.yaml'))).toBe(false);
});

import { expect, test } from '@playwright/test';
import { login } from './helpers/document.js';

async function request(page, path, body) {
  return page.evaluate(async ({ path, body }) => {
    const response = await fetch(path, body === undefined ? {} : { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    if (!response.ok) throw new Error(`${response.status}: ${await response.text()}`);
    return response.json();
  }, { path, body });
}
async function open(page, content) {
  await login(page);
  const path = `document-closure-${crypto.randomUUID()}.md`;
  await request(page, '/api/files/create', { path, content });
  await page.reload(); await page.getByTestId('layout-3').click();
  await page.getByTestId(`file-row-${path}`).click();
  await expect(page.getByTestId('rich-text-editor')).toBeVisible();
  return path;
}
const read = (page, path) => request(page, `/api/files/read?path=${encodeURIComponent(path)}`);

test('rich editor saves local edits while preserving original source, Unicode and protected nodes', async ({ page }) => {
  const errors = []; page.on('pageerror', error => errors.push(error.message));
  const prefix = '\uFEFF# 原始标题\r\n\r\n<!-- 保留  -->\r\n\r\n';
  const path = await open(page, prefix + '正文😀\r\n');
  const editor = page.getByTestId('rich-text-editor');
  await editor.getByText('正文😀', { exact: true }).click();
  await page.keyboard.press('End'); await page.keyboard.insertText('继续写');
  await expect.poll(async () => (await read(page, path)).content).toBe(prefix + '正文😀继续写\r\n');
  await page.keyboard.press('Control+z');
  await expect.poll(async () => (await read(page, path)).content).toBe(prefix + '正文😀\r\n');
  expect(errors).toEqual([]);
});

test('native footnotes, formulas, callouts, references and metadata remain editable', async ({ page }) => {
  const content = '---\ntitle: 文稿\n---\n\n正文 $x^2$ 和 [资料][r]，脚注[^n]\n\n> [!NOTE]\n> 提示内容\n\n[^n]: 注释内容\n\n[r]: https://example.com';
  const path = await open(page, content);
  await expect(page.getByTestId('document-inlineMath')).toBeVisible();
  await expect(page.getByTestId('document-footnoteReference')).toBeVisible();
  await expect(page.locator('aside[data-callout="NOTE"]')).toBeVisible();
  await expect(page.getByTestId('rich-text-editor').getByRole('link', { name: '资料' })).toBeVisible();
  await page.getByRole('button', { name: '查看属性', exact: true }).click();
  await page.getByRole('textbox', { name: 'title', exact: true }).fill('修改标题');
  await page.getByRole('button', { name: '应用', exact: true }).click();
  await expect.poll(async () => (await read(page, path)).content).toContain('title: 修改标题');
  const saved = (await read(page, path)).content;
  expect(saved).toContain('[资料][r]'); expect(saved).toContain('[^n]: 注释内容');
  expect(saved).toContain('> [!NOTE]\n> 提示内容');
});

test('table native line breaks and column alignment survive save and reload', async ({ page }) => {
  const path = await open(page, '| A | B |\n| --- | --- |\n| 内容 | 其他 |');
  const editor = page.getByTestId('rich-text-editor'), cell = editor.locator('td').first();
  await cell.getByText('内容', { exact: true }).click();
  await expect(page.getByTestId('table-toolbar')).toBeVisible();
  await expect.poll(() => cell.evaluate(element => element.contains(window.getSelection()?.anchorNode))).toBe(true);
  await editor.press('End'); await editor.press('Enter');
  await expect(cell.locator('br:not(.ProseMirror-trailingBreak)')).toHaveCount(1);
  await page.keyboard.insertText('下一行');
  await page.getByRole('button', { name: '当前列居中', exact: true }).click();
  await expect.poll(async () => (await read(page, path)).content).toContain('内容<br>下一行');
  await expect(page.getByRole('alert')).toHaveCount(0);
  await page.reload(); await page.getByTestId(`file-row-${path}`).click();
  await expect(page.getByTestId('rich-text-editor').locator('td').first()).toHaveCSS('text-align', 'center');
});

test('version restore keeps a recoverable current version and rejects stale hashes', async ({ page }) => {
  const path = await open(page, '第一版');
  const first = await read(page, path);
  const second = await request(page, '/api/files/write', { path, content: '第二版', expected_hash: first.hash });
  const versions = await request(page, `/api/documents/versions?path=${encodeURIComponent(path)}`);
  expect(versions.items).toHaveLength(1);
  await request(page, '/api/documents/restore', { path, version_id: versions.items[0].id, expected_hash: second.hash });
  expect((await read(page, path)).content).toBe('第一版');
  expect((await request(page, `/api/documents/versions?path=${encodeURIComponent(path)}`)).items).toHaveLength(2);
});

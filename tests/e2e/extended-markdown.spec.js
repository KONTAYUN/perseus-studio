import { expect, test } from '@playwright/test';
import { api, open, rich, read, append, paste } from './helpers/document.js';

test('native extensions and inert special payloads survive adjacent prose edits and reload', async ({ page }) => {
  const raw = '<details>摘要原文</details>', content = `---\ntitle: 原稿\n---\n\n${raw}\n\n正文 $x^2$ 脚注[^n] [[资料]]\n\n[^n]: **注释**\n\n尾段`;
  const path = await open(page, content);
  await expect(page.getByTestId('document-documentMetadata')).toBeVisible();
  await expect(page.getByTestId('document-inlineMath')).toBeVisible();
  await append(page, rich(page).getByText('尾段', { exact: true }), '继续写');
  await expect.poll(async () => (await read(page, path)).content).toBe(content + '继续写');
  await page.reload(); await page.getByTestId(`file-row-${path}`).click();
  await expect(page.getByTestId('preserved-block')).toHaveCount(1);
  await expect(page.getByTestId('preserved-inline')).toHaveText('[[资料]]');
});

test('special blocks offer continue/delete/undo and assistant confirmation without a source editor', async ({ page }) => {
  const original = '<details>唯一内容</details>', path = await open(page, original);
  const block = page.getByTestId('preserved-block');
  await block.getByRole('button', { name: '在后面写', exact: true }).click();
  await expect(rich(page)).toBeFocused(); await page.keyboard.insertText('尾段');
  await expect(rich(page).locator(':scope > p').last()).toHaveText('尾段');
  await block.getByRole('button', { name: '在前面写', exact: true }).click();
  await expect(rich(page)).toBeFocused(); await page.keyboard.insertText('前段');
  await expect.poll(async () => (await read(page, path)).content).toBe(`前段\n\n${original}\n\n尾段`);
  await block.getByRole('button', { name: '让助手调整', exact: true }).click();
  await expect(page.getByRole('dialog', { name: '让助手调整特殊内容' })).toBeVisible();
  await expect(page.getByRole('textbox', { name: '特殊内容', exact: true })).toHaveCount(0);
  await page.getByRole('button', { name: '取消', exact: true }).click();
  await block.getByRole('button', { name: '删除（可撤销）', exact: true }).click();
  await expect(block).toHaveCount(0);
  await rich(page).press('Control+z'); await expect(block).toBeVisible();
});

test('ordinary delimiters and extension-like typing remain prose without false format warnings', async ({ page }) => {
  const path = await open(page, '正文');
  const handle = await rich(page).elementHandle();
  await append(page, rich(page).locator('p'), ' * ** ` $10 [[资料]]');
  await expect.poll(async () => (await read(page, path)).content).toContain('资料');
  await expect(page.getByTestId('markdown-format-warning')).toHaveCount(0);
  expect(await handle.evaluate(element => element.isConnected)).toBe(true);
  await rich(page).press('Control+z'); await expect(rich(page)).toHaveText('正文');
});

test('formatted footnote dialog retains multiple paragraphs and marks', async ({ page }) => {
  const content = '正文[^n]\n\n[^n]: **重点** 与 [链接](https://example.com)\n\n    第二段';
  const path = await open(page, content);
  await page.getByTestId('document-footnoteReference').getByRole('button').click();
  const dialog = page.getByRole('dialog', { name: '脚注', exact: true });
  await expect(dialog.locator('strong')).toHaveText('重点');
  await expect(dialog.locator('a')).toHaveAttribute('href', 'https://example.com');
  await append(page, dialog.locator('.tiptap p').last(), '修改');
  await expect(dialog.locator('.tiptap p').last()).toHaveText('第二段修改');
  await dialog.getByRole('button', { name: '应用', exact: true }).click();
  await expect.poll(async () => (await read(page, path)).content, { timeout: 15_000 }).toContain('第二段修改');
  expect((await read(page, path)).content).toContain('**重点**');
});

test('Word and spreadsheet paste produces supported structures with safe table line breaks', async ({ page }) => {
  const path = await open(page, '正文');
  await rich(page).click(); await rich(page).press('Control+End'); await rich(page).press('Enter');
  await paste(page, { html: '<h2>粘贴标题</h2><p><strong>重点</strong> 与 <a href="https://example.com">链接</a></p><table><tr><td>列一</td><td>列二</td></tr><tr><td><p>甲</p><p>乙</p></td><td>丙</td></tr></table>' });
  await expect(rich(page).locator('h2')).toHaveText('粘贴标题');
  await expect(rich(page).locator('td').first().locator('br')).toHaveCount(1);
  await expect.poll(async () => (await read(page, path)).content).toContain('甲<br>乙');
  await page.reload(); await page.getByTestId(`file-row-${path}`).click();
  await expect(rich(page).locator('th')).toHaveCount(2);
});

test('merged tables and script payloads are preserved locally and never execute', async ({ page }) => {
  const path = await open(page, '正文');
  const raw = '<script>window.pasteXSS=1</script><table><tr><td colspan="2">不能丢失</td></tr></table>';
  await rich(page).click(); await rich(page).press('End'); await rich(page).press('Enter');
  await paste(page, { html: raw });
  await expect(page.getByTestId('preserved-block')).toBeVisible();
  await expect.poll(async () => (await read(page, path)).content).toContain(raw);
  expect(await page.evaluate(() => window.pasteXSS)).toBeUndefined();
  await expect(rich(page).locator('script')).toHaveCount(0);
  await page.getByTestId('preserved-block').getByRole('button', { name: '在后面写', exact: true }).click();
  await page.keyboard.insertText('继续写作');
  await expect.poll(async () => (await read(page, path)).content).toContain('继续写作');
});

test('paste in code blocks preserves literal text rather than interpreting Markdown', async ({ page }) => {
  const path = await open(page, '```txt\n原文\n```');
  await rich(page).locator('pre code').click(); await page.keyboard.press('End');
  await paste(page, { text: '\n# 原样标题\n**文字**' });
  await expect.poll(async () => (await read(page, path)).content).toContain('原文\n# 原样标题\n**文字**');
  await expect(rich(page).locator('h1,strong')).toHaveCount(0);
});

test('failed saves recover durable drafts after refresh without overwriting the original', async ({ page }) => {
  const path = await open(page, '原稿');
  await page.route('**/api/files/write', route => route.fulfill({ status: 503, json: { error: '隔离故障' } }));
  await append(page, rich(page).locator('p'), '本地草稿');
  await expect(page.getByTestId('editor-panel')).toContainText('隔离故障');
  expect((await read(page, path)).content).toBe('原稿');
  await page.reload(); await page.getByTestId(`file-row-${path}`).click();
  await page.getByRole('button', { name: '恢复草稿', exact: true }).click();
  await expect(rich(page)).toContainText('原稿本地草稿');
  await expect(page.getByRole('combobox', { name: '浏览器历史草稿' })).toBeVisible();
  expect((await read(page, path)).content).toBe('原稿');
  await page.unroute('**/api/files/write'); await rich(page).press('Control+s');
  await expect.poll(async () => (await read(page, path)).content).toBe('原稿本地草稿');
});

test('composition transactions defer conversion and saving until compositionend', async ({ page }) => {
  const path = await open(page, '正文');
  await rich(page).click(); await rich(page).press('End');
  await rich(page).dispatchEvent('compositionstart', { data: '' });
  await page.keyboard.insertText('中文😀');
  await page.waitForTimeout(400);
  expect((await read(page, path)).content).toBe('正文');
  await rich(page).press('Control+s');
  await rich(page).dispatchEvent('compositionend', { data: '中文😀' });
  await expect.poll(async () => (await read(page, path)).content).toBe('正文中文😀');
});

test('worker loading failure offers retry, preserves disk and never opens source editing', async ({ page }) => {
  await page.route(/\/assets\/session\.worker-[^/]+\.js$/, route => route.abort('failed'));
  const path = await open(page, '# 原稿');
  await expect(page.getByRole('alert')).toContainText('文稿处理组件');
  expect((await read(page, path)).content).toBe('# 原稿');
  await expect(page.getByTestId('editor-textarea')).toHaveCount(0);
  await page.unroute(/\/assets\/session\.worker-[^/]+\.js$/);
  await page.getByRole('button', { name: '重试', exact: true }).click();
  await expect(rich(page).locator('h1')).toHaveText('原稿');
});

test('paged documents retain global reference scope and undo across page navigation', async ({ page }) => {
  const source = '[全局链接][r]\n\n' + Array.from({ length: 260 }, (_, i) => `段落${i}`).join('\n\n') + '\n\n[r]: https://example.com';
  const path = await open(page, source), nav = page.getByRole('group', { name: '长文档分段编辑' });
  await expect(rich(page).locator('a')).toHaveAttribute('href', 'https://example.com');
  await append(page, rich(page).getByText('段落0', { exact: true }), '修改');
  await nav.getByRole('button', { name: '下一段', exact: true }).click();
  await rich(page).locator('p').first().click(); await rich(page).press('Control+z');
  await expect(rich(page).getByText('段落0', { exact: true })).toBeVisible();
  await expect.poll(async () => (await read(page, path)).content).toBe(source);
});

import { expect, test } from '@playwright/test';
import { paste } from './helpers/document.js';

async function api(page, path, body) {
  return page.evaluate(async ({ path, body }) => {
    const r = await fetch(path, body === undefined ? {} : {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
    });
    if (!r.ok) throw new Error(`API ${r.status}`);
    return r.json();
  }, { path, body });
}
async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
}
const rich = page => page.getByTestId('rich-text-editor');
const tools = page => page.getByTestId('table-toolbar');
async function open(page, content) {
  await login(page);
  const path = `table-editing-${Date.now()}.md`;
  await api(page, '/api/files/create', { path, content });
  await page.reload();
  await page.getByTestId('layout-3').click();
  await page.getByTestId(`file-row-${path}`).click();
  await expect(rich(page)).toBeVisible();
  return path;
}
async function action(page, group, label) {
  await tools(page).getByRole('button', { name: `调整表格${group}`, exact: true }).click();
  await page.getByTestId('table-action-menu').getByRole('menuitem', { name: label, exact: true }).click();
}
async function reload(page, path) {
  const saved = page.waitForResponse(response => response.url().includes('/api/files/write') && response.request().method() === 'POST', { timeout: 1500 }).catch(() => null);
  await page.keyboard.press('Control+s');
  const response = await saved;
  if (response) expect(response.ok()).toBeTruthy();
  await expect(page.getByTestId(`editor-tab-${path}`)).not.toContainText('●');
  await page.reload();
  await page.getByTestId(`file-row-${path}`).click();
  await expect(rich(page)).toBeVisible();
}
const source = '| 字段 | 内容 |\n| --- | --- |\n| A1 | B1 |\n| A2 | B2 |\n';

test('table controls do not move the clicked cell before pointer release', async ({ page }) => {
  await open(page, source);
  const cell = rich(page).locator('td').first();
  const before = await cell.boundingBox();
  await page.mouse.move(before.x + before.width / 2, before.y + before.height / 2);
  await page.mouse.down();
  try {
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
    await expect(tools(page)).toHaveCount(0);
    expect((await cell.boundingBox()).y).toBeCloseTo(before.y, 1);
  } finally { await page.mouse.up(); }
  await expect(tools(page)).toBeVisible();
  await rich(page).press('End');
  await page.keyboard.insertText('正确单元格');
  await expect(cell).toContainText('A1正确单元格');
  await expect(rich(page).locator('th').first()).toHaveText('字段');
});

test('Enter and Shift+Enter in a table remain editable through autosave and reload', async ({ page }) => {
  const errors = []; page.on('pageerror', e => errors.push(e.message));
  const path = await open(page, source);
  const cell = rich(page).locator('td').first();
  await cell.click();
  await expect(rich(page)).toBeFocused();
  await rich(page).press('End');
  await rich(page).press('Enter');
  await expect(cell.locator('br:not(.ProseMirror-trailingBreak)')).toHaveCount(1);
  await page.keyboard.insertText('第二行');
  await expect(cell).toContainText('第二行');
  await rich(page).press('Shift+Enter');
  await page.keyboard.insertText('第三行');
  await expect(rich(page)).toBeVisible();
  await expect(cell.locator('br')).toHaveCount(2);
  await expect(tools(page)).toContainText('3 行 × 2 列');
  await page.keyboard.press('Control+s');
  await expect.poll(async () => (await api(page, `/api/files/read?path=${path}`)).content).toContain('A1<br>第二行<br>第三行');
  await expect(page.getByTestId('editor-mode-toggle')).toHaveCount(0);
  await expect(rich(page).locator('td').first().locator('br')).toHaveCount(2);
  await reload(page, path);
  await expect(rich(page).locator('td').first()).toContainText('第三行');
  await expect(rich(page).locator('td').first().locator('br')).toHaveCount(2);
  expect(errors).toEqual([]);
});

test('empty table insertion and Enter do not lock the editor, and existing break tables recover immediately', async ({ page }) => {
  const path = await open(page, '');
  await rich(page).click();
  await page.getByTestId('block-format-handle').click();
  await page.getByTestId('block-format-menu').getByRole('menuitem', { name: '插入表格', exact: true }).click();
  await rich(page).locator('th').first().click();
  await page.keyboard.press('Enter');
  await page.keyboard.insertText('可以继续输入');
  await expect(tools(page)).toBeVisible();
  await action(page, '行', '在下方插入行');
  await expect(rich(page).locator('tr')).toHaveCount(4);
  await reload(page, path);
  await expect(rich(page).locator('th').first()).toContainText('可以继续输入');
  await expect(rich(page).locator('th').first().locator('br')).toHaveCount(1);
});

test('add/remove rows and columns keeps data, promotes the header, and survives undo/redo and reload', async ({ page }) => {
  const path = await open(page, source);
  await rich(page).locator('td').first().click();
  await action(page, '行', '在上方插入行');
  await expect(rich(page).locator('tr')).toHaveCount(4);
  await rich(page).locator('tr').nth(1).locator('td').first().click();
  await action(page, '行', '删除当前行');
  await expect(rich(page).locator('tr')).toHaveCount(3);
  await rich(page).locator('th').first().click();
  await action(page, '行', '删除当前行');
  await expect(rich(page).locator('tr')).toHaveCount(2);
  await expect(rich(page).locator('th').first()).toHaveText('A1');
  await page.keyboard.press('Control+z');
  await expect(rich(page).locator('tr')).toHaveCount(3);
  await page.keyboard.press('Control+Shift+z');
  await expect(rich(page).locator('tr')).toHaveCount(2);
  await rich(page).locator('th').first().click();
  await action(page, '列', '在左侧插入列');
  await expect(rich(page).locator('tr').first().locator('th')).toHaveCount(3);
  await rich(page).locator('th').first().click();
  await action(page, '列', '删除当前列');
  await expect(rich(page).locator('tr').first().locator('th')).toHaveCount(2);
  await action(page, '列', '在右侧插入列');
  await expect(rich(page).locator('tr').first().locator('th')).toHaveCount(3);
  await reload(page, path);
  await expect(rich(page).locator('tr')).toHaveCount(2);
  await expect(rich(page).locator('th')).toHaveCount(3);
  await expect(rich(page).locator('th').first()).toHaveText('A1');
  await expect(rich(page)).toContainText('B2');
});

test('one remaining row/column is protected; delete table is undoable and prose remains writable', async ({ page }) => {
  await open(page, '| 唯一单元格 |\n| --- |\n');
  await rich(page).locator('th').click();
  for (const [group, label] of [['行', '删除当前行'], ['列', '删除当前列']]) {
    await tools(page).getByRole('button', { name: `调整表格${group}` }).click();
    await expect(page.getByTestId('table-action-menu').getByRole('menuitem', { name: label })).toBeDisabled();
    await page.keyboard.press('Escape');
  }
  await tools(page).getByRole('button', { name: '删除表格（可撤销）', exact: true }).click();
  await expect(rich(page).locator('table')).toHaveCount(0);
  await rich(page).press('Control+z');
  await expect(rich(page).locator('th')).toHaveText('唯一单元格');
  await rich(page).locator('th').click();
  await tools(page).getByRole('button', { name: '继续写', exact: true }).click();
  await page.keyboard.insertText('表格后的正文');
  await expect(rich(page).locator(':scope > p').last()).toHaveText('表格后的正文');
  await expect(tools(page)).toHaveCount(0);
});

test('Tab appends a final row, Shift+Tab moves back, and Mod+Enter escapes the table', async ({ page }) => {
  await open(page, source);
  await rich(page).locator('td').last().click();
  await page.keyboard.press('Tab');
  await page.keyboard.insertText('新增末行');
  await expect(rich(page).locator('tr')).toHaveCount(4);
  await expect(rich(page).locator('tr').last().locator('td').first()).toHaveText('新增末行');
  await page.keyboard.press('Shift+Tab');
  await page.keyboard.press('End');
  await page.keyboard.insertText('返回上一格');
  await expect(rich(page).locator('tr').nth(2).locator('td').last()).toContainText('返回上一格');
  const modifier = await page.evaluate(() => /Mac|iPhone|iPad/.test(navigator.platform) ? 'Meta' : 'Control');
  await page.keyboard.press(`${modifier}+Enter`);
  await page.keyboard.insertText('跳出表格继续写');
  await expect(rich(page).locator(':scope > p').last()).toHaveText('跳出表格继续写');
});

test('table block menu offers safe structural actions and can insert prose before or after', async ({ page }) => {
  await open(page, source);
  await rich(page).locator('td').first().click();
  await page.getByTestId('block-format-handle').click();
  const menu = page.getByTestId('block-format-menu');
  await expect(menu.getByRole('menuitem', { name: '插入表格', exact: true })).toHaveCount(0);
  await expect(menu.getByRole('menuitem', { name: '代码块', exact: true })).toHaveCount(0);
  await menu.getByRole('menuitem', { name: '在表格前插入段落' }).click();
  await page.keyboard.insertText('表格前的正文');
  await expect(rich(page).locator(':scope > p').first()).toHaveText('表格前的正文');
  await rich(page).locator('td').first().click();
  await page.getByTestId('block-format-handle').click();
  await menu.getByRole('menuitem', { name: '在下方插入段落', exact: true }).click();
  await page.keyboard.insertText('表格后继续');
  await expect(rich(page).locator(':scope > p').last()).toHaveText('表格后继续');
});

test('multiline plain-text paste in a cell preserves line breaks and remains editable after reload', async ({ page }) => {
  const path = await open(page, source);
  await rich(page).locator('td').first().click();
  await page.keyboard.press('End');
  await paste(page, { text: '粘贴第一行\n粘贴第二行\n粘贴第三行' });
  await expect(rich(page).locator('td').first()).toContainText('粘贴第三行');
  await reload(page, path);
  await expect(rich(page).locator('td').first()).toContainText('粘贴第一行');
  await expect(rich(page).locator('td').first()).toContainText('粘贴第三行');
  await expect(rich(page).locator('td').first().locator('br')).toHaveCount(2);
});

test('narrow editor keeps table actions reachable and large tables horizontally scroll', async ({ page }) => {
  await page.setViewportSize({ width: 760, height: 800 });
  const cols = Array.from({ length: 9 }, (_, i) => `字段${i}`);
  await open(page, `| ${cols.join(' | ')} |\n| ${cols.map(() => '---').join(' | ')} |\n| ${cols.join(' | ')} |\n`);
  await rich(page).locator('th').first().click();
  await expect(tools(page)).toContainText('2 行 × 9 列');
  const rect = await tools(page).boundingBox();
  expect(rect.x).toBeGreaterThanOrEqual(0);
  expect(rect.x + rect.width).toBeLessThanOrEqual(760);
  const overflow = await rich(page).locator('.tableWrapper').evaluate(el => el.scrollWidth > el.clientWidth);
  expect(overflow).toBe(true);
  await action(page, '列', '在右侧插入列');
  await expect(tools(page)).toContainText('2 行 × 10 列');
});

test('extended documents keep tables editable while unsafe HTML remains inert', async ({ page }) => {
  await login(page);
  const path = `table-preview-${Date.now()}.md`;
  const content = `---\ntitle: 扩展文档\n---\n\n${source.replace('A1', '第一行<br>第二行')}\n<br onclick="window.tableXSS=1">`;
  await api(page, '/api/files/create', { path, content });
  await page.reload();
  await page.getByTestId('layout-3').click();
  await page.getByTestId(`file-row-${path}`).click();
  const preview = rich(page);
  await expect(preview.locator('td').first().locator('br')).toHaveCount(1);
  expect(await page.evaluate(() => window.tableXSS)).toBeUndefined();
  await expect(page.getByTestId('editor-mode-toggle')).toHaveCount(0);
  expect((await api(page, `/api/files/read?path=${path}`)).content).toBe(content);
});

test('spreadsheet clipboard paste keeps a rectangular editable table and a stable header on reload', async ({ page }) => {
  const path = await open(page, source);
  await rich(page).locator('th').first().click();
  await paste(page, { html: '<table><tr><td>粘贴表头一</td><td>粘贴表头二</td></tr><tr><td>单元格一</td><td>单元格二</td></tr></table>', text: '粘贴表头一\t粘贴表头二\n单元格一\t单元格二' });
  await expect(rich(page)).toContainText('单元格二');
  await expect(rich(page).locator('table')).toHaveCount(2);
  await expect(rich(page).locator('table').last().locator('th').first()).toHaveText('粘贴表头一');
  await reload(page, path);
  await expect(rich(page).locator('table')).toHaveCount(2);
  await expect(rich(page).locator('table').last().locator('th').first()).toHaveText('粘贴表头一');
  await expect(rich(page)).toContainText('单元格二');
});

test('inserting above the header keeps one header and never adds a phantom row on reload', async ({ page }) => {
  const path = await open(page, source);
  await rich(page).locator('th').first().click();
  await action(page, '行', '在上方插入行');
  await expect(rich(page).locator('tr')).toHaveCount(4);
  await expect(rich(page).locator('th')).toHaveCount(2);
  await rich(page).locator('th').first().click();
  await page.keyboard.insertText('新表头');
  await reload(page, path);
  await expect(rich(page).locator('tr')).toHaveCount(4);
  await expect(rich(page).locator('th')).toHaveCount(2);
  await expect(rich(page).locator('th').first()).toHaveText('新表头');
  await expect(rich(page).locator('tr').nth(1)).toContainText('字段');
});

import { expect, test } from '@playwright/test';

async function api(page, path, body, method = 'POST') {
  return page.evaluate(async ({ path, body, method }) => {
    const response = await fetch(path, { method, headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
    if (!response.ok) throw new Error(`API status ${response.status}`);
    return response.json();
  }, { path, body, method });
}
async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
}
async function openDocument(page, path, content) {
  await api(page, '/api/files/create', { path, content });
  await page.reload();
  await page.getByTestId('layout-3').click();
  await page.getByTestId(`file-row-${path}`).click();
}
async function selectText(page, text) {
  await page.getByTestId('rich-text-editor').evaluate((element, text) => {
    element.focus();
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    let node;
    while ((node = walker.nextNode())) {
      const start = node.textContent.indexOf(text);
      if (start < 0) continue;
      const range = document.createRange();
      range.setStart(node, start); range.setEnd(node, start + text.length);
      window.getSelection().removeAllRanges(); window.getSelection().addRange(range);
      document.dispatchEvent(new Event('selectionchange'));
      return;
    }
    throw new Error('text to select was not found');
  }, text);
  await expect(page.getByTestId('selection-toolbar')).toBeVisible();
}
async function dragSelectText(page, text) {
  const rect = await page.getByTestId('rich-text-editor').evaluate((element, text) => {
    const walker = document.createTreeWalker(element, NodeFilter.SHOW_TEXT);
    let node;
    while ((node = walker.nextNode())) {
      const start = node.textContent.indexOf(text);
      if (start < 0) continue;
      const range = document.createRange();
      range.setStart(node, start); range.setEnd(node, start + text.length);
      return range.getBoundingClientRect().toJSON();
    }
    throw new Error('drag target missing');
  }, text);
  await page.mouse.move(rect.left + 1, rect.top + rect.height / 2);
  await page.mouse.down();
  await page.mouse.move(rect.right - 1, rect.top + rect.height / 2, { steps: 12 });
  await page.mouse.up();
  await expect.poll(() => page.evaluate(() => window.getSelection()?.toString())).toBe(text);
  await expect(page.getByTestId('selection-toolbar')).toBeVisible();
}
const sourceButton = page => page.getByRole('button', { name: '切换为纯文本（当前：富文本）', exact: true });
const richButton = page => page.getByRole('button', { name: '切换为富文本（当前：纯文本）', exact: true });

test('assistant Markdown renders safely and command copy keeps exact text', async ({ page, context }) => {
  await context.grantPermissions(['clipboard-read', 'clipboard-write']);
  const content = '# 部署步骤\n\n**登录服务器**，执行 `ssh`：\n\n```bash\nssh root@YOUR_SERVER_IP\nprintf "hello"\n```\n\n- 第一步\n- 第二步\n\n| 名称 | 状态 |\n| --- | --- |\n| API | 就绪 |\n\n[危险链接](javascript:alert(1))\n\n<img src=x onerror="window.markdownXSS=1">';
  await page.route(/\/api\/conversations$/, route => route.fulfill({ json: { items: [{ id: 'md-fixture', title: 'Markdown 显示测试' }] } }));
  await page.route('**/api/conversations/md-fixture/messages', route => route.fulfill({ json: { items: [{ id: 'md-answer', role: 'assistant', content, attachments: [] }] } }));
  await page.route('**/api/conversations/md-fixture/context-usage', route => route.fulfill({ json: { usage: null } }));
  await login(page);
  await page.getByTestId('conversation-history-button').click();
  await page.getByTestId('conversation-history-popover').getByText('Markdown 显示测试', { exact: true }).click();
  const message = page.locator('.message-bubble-assistant');
  await expect(message.getByRole('heading', { name: '部署步骤' })).toBeVisible();
  await expect(message.locator('strong')).toHaveText('登录服务器');
  await expect(message.locator('table')).toBeVisible();
  await expect(message.locator('a[href^="javascript"]')).toHaveCount(0);
  await expect(message.locator('img')).toHaveCount(0);
  await message.getByRole('button', { name: '复制代码', exact: true }).click();
  expect((await page.evaluate(() => navigator.clipboard.readText())).replace(/\r\n/g, '\n')).toBe('ssh root@YOUR_SERVER_IP\nprintf "hello"\n');
  expect(await page.evaluate(() => window.markdownXSS)).toBeUndefined();
  await expect(message.getByText('已复制', { exact: true })).toBeVisible();
});

test('rich document formats selection and paragraphs then persists Markdown without writes on open', async ({ page }) => {
  const path = `rich-document-${Date.now()}.md`;
  const content = '# 文档规则\n\n选择这段文字调整格式。\n\n普通段落\n\n- 记录范围\n- 容器关系\n\n```bash\necho hello\n```\n';
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await login(page);
  let writes = 0;
  page.on('request', request => { if (request.method() === 'POST' && request.url().endsWith('/api/files/write')) writes++; });
  try {
    await openDocument(page, path, content);
    const editor = page.getByTestId('rich-text-editor');
    await expect(editor.getByRole('heading', { name: '文档规则' })).toBeVisible();
    await sourceButton(page).click();
    await expect(page.getByTestId('editor-textarea')).toHaveValue(content);
    expect(writes).toBe(0);
    await richButton(page).click();
    await dragSelectText(page, '这段文字');
    await page.getByTestId('selection-toolbar').getByRole('button', { name: '粗体', exact: true }).click();
    await expect(editor.locator('strong')).toHaveText('这段文字');
    await selectText(page, '普通段落');
    await page.getByTestId('block-format-handle').click();
    await page.getByTestId('block-format-menu').getByRole('menuitem', { name: '二级标题', exact: true }).click();
    await expect(editor.locator('h2')).toHaveText('普通段落');
    await sourceButton(page).click();
    await expect(page.getByTestId('editor-textarea')).toHaveValue(/选择\*\*这段文字\*\*调整格式/);
    const read = () => api(page, `/api/files/read?path=${encodeURIComponent(path)}`, undefined, 'GET');
    await expect.poll(async () => (await read()).content).toContain('## 普通段落');
    expect((await read()).content).toContain('**这段文字**');
    await richButton(page).click();
    await editor.getByText('容器关系', { exact: true }).click();
    await expect(page.getByTestId('block-format-handle')).toBeVisible();
    await page.screenshot({ path: '_local_artifacts/markdown-document-light.png', fullPage: true });
    await page.getByRole('button', { name: '切换深色模式', exact: true }).click();
    await expect(page.locator('.app-shell')).toHaveClass(/dark/);
    await expect(page.getByTestId('editor-panel')).toHaveCSS('background-color', 'rgb(30, 30, 30)');
    await selectText(page, '记录范围');
    await page.screenshot({ path: '_local_artifacts/markdown-document-dark.png', fullPage: true });
    expect(errors).toEqual([]);
  } finally { await api(page, '/api/files/delete', { path }).catch(() => {}); }
});

test('extended Markdown is previewed without rewriting and remains editable as source', async ({ page }) => {
  const path = `rich-preserve-${Date.now()}.md`;
  const content = '---\ntitle: 原始配置\n---\n\n# 标题\n\n<details>不丢失原始结构</details>\n';
  await login(page);
  try {
    await openDocument(page, path, content);
    await expect(page.getByText('此文档包含扩展语法', { exact: false })).toBeVisible();
    await expect(page.getByTestId('rich-text-editor')).toHaveCount(0);
    await sourceButton(page).click();
    await expect(page.getByTestId('editor-textarea')).toHaveValue(content);
    expect((await api(page, `/api/files/read?path=${encodeURIComponent(path)}`, undefined, 'GET')).content).toBe(content);
  } finally { await api(page, '/api/files/delete', { path }).catch(() => {}); }
});

test('inline HTML spans remain editable and preserve inert attributes on save', async ({ page }) => {
  const path = `rich-inline-span-${Date.now()}.md`;
  const opening = '<span style="background:red;color:white" onclick="window.spanXSS=1" title="a > b">';
  const content = `# 行内格式\n\n- 地址：${opening}\`example\`</span>\n\n尾段文字\n`;
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await login(page);
  let writes = 0;
  page.on('request', request => { if (request.method() === 'POST' && request.url().endsWith('/api/files/write')) writes++; });
  try {
    await openDocument(page, path, content);
    const editor = page.getByTestId('rich-text-editor');
    const span = editor.locator('span[data-preserved-span]');
    await expect(span.locator('code')).toHaveText('example');
    await expect(span).not.toHaveAttribute('onclick');
    await expect(span).not.toHaveAttribute('style');
    await sourceButton(page).click();
    await expect(page.getByTestId('editor-textarea')).toHaveValue(content);
    expect(writes).toBe(0);
    await richButton(page).click();
    await dragSelectText(page, 'example');
    await page.keyboard.press('ArrowRight');
    await expect.poll(() => page.evaluate(() => window.getSelection()?.isCollapsed)).toBe(true);
    await page.keyboard.insertText('-edited');
    await selectText(page, '尾段文字');
    await page.getByTestId('selection-toolbar').getByRole('button', { name: '粗体', exact: true }).click();
    const read = () => api(page, `/api/files/read?path=${encodeURIComponent(path)}`, undefined, 'GET');
    await expect.poll(async () => (await read()).content).toContain(`${opening}\`example-edited\`</span>`);
    await expect.poll(async () => (await read()).content).toContain('**尾段文字**');
    await sourceButton(page).click();
    await expect(page.getByTestId('editor-textarea')).toHaveValue(new RegExp('example-edited'));
    await richButton(page).click();
    await expect(span.locator('code')).toHaveText('example-edited');
    expect(await page.evaluate(() => window.spanXSS)).toBeUndefined();
    expect(errors).toEqual([]);
  } finally { await api(page, '/api/files/delete', { path }).catch(() => {}); }
});

test('rich editor supports links and undo and preserves both sides of an external save conflict', async ({ page }) => {
  const path = `rich-conflict-${Date.now()}.md`;
  await login(page);
  try {
    await openDocument(page, path, '原始链接和段落\n');
    const editor = page.getByTestId('rich-text-editor');
    await selectText(page, '链接');
    await page.getByTestId('selection-toolbar').getByRole('button', { name: '编辑链接', exact: true }).click();
    await page.getByRole('textbox', { name: '链接地址' }).fill('https://example.com/docs');
    await page.getByRole('button', { name: '应用', exact: true }).click();
    await expect(editor.locator('a')).toHaveAttribute('href', 'https://example.com/docs');
    await editor.press('Control+z');
    await expect(editor.locator('a')).toHaveCount(0);
    await api(page, '/api/files/write', { path, content: '外部 AI 新版本\n' });
    await editor.click();
    await editor.press('Control+End');
    await editor.pressSequentially(' 本地保留');
    await expect(page.getByRole('button', { name: '保留本地修改', exact: true })).toBeVisible();
    const read = () => api(page, `/api/files/read?path=${encodeURIComponent(path)}`, undefined, 'GET');
    expect((await read()).content).toBe('外部 AI 新版本\n');
    await expect(editor).toContainText('本地保留');
    await page.getByRole('button', { name: '重新加载文件', exact: true }).click();
    await expect(editor).toContainText('外部 AI 新版本');
    await editor.press('Control+z');
    await expect(editor).toContainText('外部 AI 新版本');
    await expect(editor).not.toContainText('本地保留');
  } finally { await api(page, '/api/files/delete', { path }).catch(() => {}); }
});

test('document modes track file identity and plain text edits return to rich view', async ({ page }) => {
  const one = `rich-first-${Date.now()}.md`;
  const two = `rich-second-${Date.now()}.md`;
  await login(page);
  try {
    await api(page, '/api/files/create', { path: one, content: '# 第一份\n' });
    await openDocument(page, two, '# 第二份\n');
    await expect(page.getByTestId('rich-text-editor').locator('h1')).toHaveText('第二份');
    await sourceButton(page).click();
    await page.getByTestId('editor-textarea').fill('# 第二份更新\n\n**保留内容**\n');
    await page.getByTestId('layout-1').click();
    await page.getByTestId('layout-3').click();
    await expect(page.getByTestId('editor-textarea')).toHaveValue('# 第二份更新\n\n**保留内容**\n');
    await page.getByTestId(`file-row-${one}`).click();
    await expect(page.getByTestId('rich-text-editor').locator('h1')).toHaveText('第一份');
    await page.getByTestId(`editor-tab-${two}`).click();
    await expect(page.getByTestId('editor-textarea')).toHaveValue('# 第二份更新\n\n**保留内容**\n');
    await richButton(page).click();
    await expect(page.getByTestId('rich-text-editor').locator('strong')).toHaveText('保留内容');
    expect((await api(page, `/api/files/read?path=${encodeURIComponent(two)}`, undefined, 'GET')).content).toBe('# 第二份更新\n\n**保留内容**\n');
    await page.setViewportSize({ width: 760, height: 720 });
    await selectText(page, '保留内容');
    const toolbar = await page.getByTestId('selection-toolbar').boundingBox();
    expect(toolbar.x).toBeGreaterThanOrEqual(0);
    expect(toolbar.x + toolbar.width).toBeLessThanOrEqual(760);
    await page.screenshot({ path: '_local_artifacts/markdown-document-narrow.png', fullPage: true });
  } finally {
    await api(page, '/api/files/delete', { path: one }).catch(() => {});
    await api(page, '/api/files/delete', { path: two }).catch(() => {});
  }
});

test('empty rich document accepts typing, task checkboxes, tables, and immediate keyboard save', async ({ page }) => {
  const path = `rich-structures-${Date.now()}.md`;
  const errors = [];
  page.on('pageerror', error => errors.push(error.message));
  await login(page);
  try {
    await openDocument(page, path, '');
    const editor = page.getByTestId('rich-text-editor');
    await editor.click();
    await editor.pressSequentially('待完成事项');
    await page.getByTestId('block-format-handle').click();
    await page.getByTestId('block-format-menu').getByRole('menuitem', { name: '任务列表', exact: true }).click();
    await editor.getByRole('checkbox').check();
    await expect(editor.getByRole('checkbox')).toBeChecked();
    const checkbox = await editor.getByRole('checkbox').boundingBox();
    const taskText = await editor.getByText('待完成事项', { exact: true }).boundingBox();
    expect(Math.abs(checkbox.y + checkbox.height / 2 - taskText.y - taskText.height / 2)).toBeLessThan(8);
    expect(taskText.x).toBeGreaterThan(checkbox.x + checkbox.width);
    await editor.getByText('待完成事项', { exact: true }).click();
    await page.getByTestId('block-format-handle').click();
    await page.getByTestId('block-format-menu').getByRole('menuitem', { name: '在下方插入段落', exact: true }).click();
    await page.getByTestId('block-format-handle').click();
    await page.getByTestId('block-format-menu').getByRole('menuitem', { name: '插入表格', exact: true }).click();
    await expect(editor.locator('table')).toBeVisible();
    await editor.locator('th').first().click();
    await page.keyboard.insertText('字段');
    await page.getByTestId('block-format-handle').click();
    await page.getByTestId('block-format-menu').getByRole('menuitem', { name: '在右侧插入列', exact: true }).click();
    await expect(editor.locator('tr').first().locator('th')).toHaveCount(4);
    await editor.locator('th').first().click();
    const saved = page.waitForRequest(request => request.method() === 'POST' && request.url().endsWith('/api/files/write') && request.postDataJSON().content.includes('快捷保存'));
    await page.keyboard.insertText('快捷保存');
    await page.keyboard.press('Control+s');
    expect((await saved).postDataJSON().content).toContain('- [x] 待完成事项');
    await sourceButton(page).click();
    await expect(page.getByTestId('editor-textarea')).toHaveValue(/\|.*字段.*快捷保存/);
    await richButton(page).click();
    await expect(editor.getByRole('checkbox')).toBeChecked();
    await expect(editor.locator('th').first()).toContainText('快捷保存');
    await editor.locator('th').first().click();
    await page.getByTestId('block-format-handle').click();
    await expect(page.getByTestId('block-format-menu')).toHaveCSS('opacity', '1');
    await page.screenshot({ path: '_local_artifacts/markdown-block-menu.png', fullPage: true });
    expect(errors).toEqual([]);
  } finally { await api(page, '/api/files/delete', { path }).catch(() => {}); }
});

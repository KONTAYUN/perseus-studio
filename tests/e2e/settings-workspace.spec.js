import { expect, test } from '@playwright/test';
import { createServer } from 'node:http';

async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByTestId('settings-button')).toBeVisible();
}
async function api(page, path, body, method = 'POST') {
  const response = await page.request.fetch(path, { method, data: body });
  expect(response.ok(), `${method} ${path}: ${response.status()}`).toBeTruthy();
  return response.json();
}
async function open(page) { await page.getByTestId('settings-button').click(); await expect(page.getByTestId('settings-workspace')).toBeVisible(); }
async function model(page, name = `settings-${Date.now()}`) {
  return api(page, '/api/model-providers', { name, model_id: 'ui-model', protocol: 'openai', base_url: 'http://127.0.0.1:19991/v1', api_key: 'settings-fixture-key', enabled: true });
}
async function fillModel(page) {
  await page.getByTestId('model-add-button').click();
  await page.getByTestId('model-base-url-input').fill('http://127.0.0.1:19991/v1');
  await page.getByTestId('model-api-key-input').fill('settings-fixture-key');
  await page.getByTestId('model-id-input').fill(`manual-${Date.now()}`);
}
async function discard(page) { await page.getByRole('button', { name: '放弃修改', exact: true }).click(); }

test('settings categories preserve live workbench drafts and return keyboard focus', async ({ page }) => {
  const errors = []; page.on('pageerror', error => errors.push(error.message));
  await login(page);
  const path = `settings-draft-${Date.now()}.txt`;
  await api(page, '/api/files/create', { path, content: 'server text' });
  try {
    await page.reload(); await page.getByTestId('layout-3').click();
    await page.getByTestId(`file-row-${path}`).click();
    const editor = page.getByTestId('editor-textarea');
    await editor.fill('local editor draft');
    await page.getByTestId('chat-input').fill('unsent chat draft');
    await open(page);
    await expect(page.getByRole('heading', { name: '模型与连接', exact: true })).toBeFocused();
    await expect(page.getByTestId('model-name-input')).toHaveCount(0);
    for (const [id, title] of [['search', '联网搜索'], ['skills', '工具扩展'], ['users', '用户与访问'], ['usage', '用量与费用'], ['diagnostics', '诊断日志']]) {
      await page.getByRole('navigation', { name: '设置分类' }).getByText(title, { exact: true }).click();
      await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible();
      expect(await page.getByTestId('settings-workspace').evaluate(root => root.contains(document.activeElement))).toBe(true);
    }
    await page.getByTestId('settings-return').click();
    await expect(editor).toHaveValue('local editor draft');
    await expect(page.getByTestId('chat-input')).toHaveValue('unsent chat draft');
    await expect(page.getByTestId('settings-button')).toBeFocused();
    expect(errors).toEqual([]);
  } finally { await api(page, '/api/files/delete', { path }); }
});

test('unsaved model navigation uses a modal focus loop, cancel and explicit discard', async ({ page }) => {
  await login(page); await open(page); await fillModel(page);
  const value = await page.getByTestId('model-id-input').inputValue();
  await page.getByTestId('settings-nav-search').click();
  const dialog = page.getByRole('dialog', { name: '放弃未保存的修改？' });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole('button', { name: '继续编辑' })).toBeFocused();
  await page.keyboard.press('Shift+Tab');
  expect(await dialog.evaluate(root => root.contains(document.activeElement))).toBe(true);
  for (let i = 0; i < 8; i++) { await page.keyboard.press('Tab'); expect(await dialog.evaluate(root => root.contains(document.activeElement))).toBe(true); }
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(page.getByTestId('model-id-input')).toHaveValue(value);
  await page.getByTestId('settings-return').click(); await discard(page);
  await expect(page.getByTestId('settings-workspace')).toHaveCount(0);
  expect(await page.getByTestId('settings-button').evaluate(node => Boolean(node.closest('[inert]')))).toBe(false);
  await open(page); await page.getByTestId('model-add-button').click();
  await expect(page.getByTestId('model-api-key-input')).toHaveValue('');
  await page.getByRole('button', { name: '取消', exact: true }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
});

test('failed saves retain inputs; double submit is blocked and a failed refresh cannot cause a second create', async ({ page }) => {
  await login(page); await open(page); await fillModel(page);
  const id = await page.getByTestId('model-id-input').inputValue();
  let creates = 0, fail = true, failRefresh = false;
  await page.route('**/api/model-providers', async route => {
    if (route.request().method() === 'POST') {
      creates++;
      if (fail) { fail = false; await route.fulfill({ status: 503, json: { error: '保存故障，请重试' } }); return; }
      await new Promise(resolve => setTimeout(resolve, 350)); failRefresh = true;
    } else if (failRefresh) { await route.fulfill({ status: 503, json: { error: '刷新故障' } }); return; }
    await route.continue();
  });
  await page.getByTestId('settings-form-save').click();
  await expect(page.getByRole('alert')).toContainText('保存故障');
  await expect(page.getByTestId('model-id-input')).toHaveValue(id);
  await expect(page.getByTestId('model-api-key-input')).toHaveValue('settings-fixture-key');
  await page.getByTestId('settings-form-save').evaluate(button => { button.click(); button.click(); });
  await page.getByTestId('settings-return').click();
  await expect(page.getByText('操作正在提交，请等待结果后再离开。')).toBeVisible();
  await expect(page.getByRole('status').filter({ hasText: '配置已保存，但列表刷新失败' })).toBeVisible();
  expect(creates).toBe(2);
  await page.unroute('**/api/model-providers');
  const all = await api(page, '/api/model-providers', undefined, 'GET');
  const items = all.items.filter(item => item.model_id === id);
  expect(items).toHaveLength(1); expect(items[0].name).toBe(id);
  await page.getByRole('button', { name: '刷新列表', exact: true }).click();
  await expect(page.getByTestId(`provider-row-${items[0].id}`)).toBeVisible();
  await api(page, `/api/model-providers/${items[0].id}`, undefined, 'DELETE');
});

test('model test results are discarded when Model ID changes during the request', async ({ page }) => {
  await login(page); const source = await model(page);
  try {
    await page.reload(); await open(page);
    await page.getByTestId(`provider-row-${source.id}`).getByRole('button', { name: '编辑', exact: true }).click();
    await expect(page.getByTestId('model-api-key-input')).toHaveValue('');
    let release; const gate = new Promise(resolve => { release = resolve; });
    await page.route('**/api/model-providers/test', async route => { await gate; await route.fulfill({ json: { ok: true, summary: 'OLD_MODEL_REPORT' } }).catch(() => {}); });
    const requested = page.waitForRequest('**/api/model-providers/test');
    await page.getByRole('button', { name: '测试模型调用', exact: true }).click(); await requested;
    await page.getByTestId('model-id-input').fill('changed-model'); release();
    await expect(page.getByRole('button', { name: '测试模型调用', exact: true })).toBeEnabled();
    await expect(page.getByText('OLD_MODEL_REPORT')).toHaveCount(0);
  } finally { await api(page, `/api/model-providers/${source.id}`, undefined, 'DELETE'); }
});

test('model deletion requires confirmation and cancelling preserves the configuration', async ({ page }) => {
  await login(page); const source = await model(page);
  await page.reload(); await open(page);
  const row = page.getByTestId(`provider-row-${source.id}`);
  let deletes = 0; page.on('request', request => { if (request.method() === 'DELETE' && request.url().endsWith(`/model-providers/${source.id}`)) deletes++; });
  await row.getByLabel(`更多操作 ${source.name}`).click();
  await row.getByRole('button', { name: '删除模型', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: '取消', exact: true }).click();
  expect(deletes).toBe(0); await expect(row).toBeVisible();
  await row.getByLabel(`更多操作 ${source.name}`).click(); await row.getByRole('button', { name: '删除模型', exact: true }).click();
  await page.getByRole('dialog').getByRole('button', { name: '删除模型', exact: true }).click();
  await expect(row).toHaveCount(0); expect(deletes).toBe(1);
});

test('search configuration and user creation protect unsaved values across category changes', async ({ page }) => {
  await login(page); await open(page); await page.getByTestId('settings-nav-search').click();
  await page.getByTestId('search-add-button').click();
  await page.getByTestId('search-api-key-input').fill('search-fixture-key');
  await page.getByTestId('search-priority-input').fill('7');
  await page.getByTestId('settings-nav-users').click();
  await page.getByRole('button', { name: '继续编辑', exact: true }).click();
  await expect(page.getByTestId('search-priority-input')).toHaveValue('7');
  await page.getByTestId('settings-nav-users').click(); await discard(page);
  await page.getByRole('button', { name: '创建用户', exact: true }).click();
  await page.getByPlaceholder('username', { exact: true }).fill('unsaved-user');
  await page.getByTestId('settings-nav-models').click();
  await page.getByRole('button', { name: '继续编辑', exact: true }).click();
  await expect(page.getByPlaceholder('username', { exact: true })).toHaveValue('unsaved-user');
  await page.getByTestId('settings-return').click(); await discard(page);
  const users = await api(page, '/api/admin/users', undefined, 'GET');
  expect(users.items.some(item => item.username === 'unsaved-user')).toBe(false);
});

test('price configuration is an inline page with guarded drafts and persists actual amounts', async ({ page }) => {
  await login(page); const source = await model(page, `price-source-${Date.now()}`);
  try {
    await page.reload(); await open(page); await page.getByTestId('settings-nav-usage').click();
    await page.getByRole('button', { name: '价格配置', exact: true }).click();
    await expect(page.getByTestId('price-settings')).toBeVisible(); await expect(page.getByRole('dialog')).toHaveCount(0);
    await page.getByRole('button', { name: '添加价格', exact: true }).click();
    // Select the actual saved provider through the shared accessible picker.
    await page.locator('label').filter({ hasText: '模型配置' }).getByRole('button').click();
    await page.locator('body > .ui-popover-surface').getByRole('button').filter({ hasText: source.name }).click();
    await page.getByLabel('输入 / 百万 tokens', { exact: true }).fill('2.5');
    await page.getByTestId('settings-nav-models').click();
    await page.getByRole('button', { name: '继续编辑', exact: true }).click();
    await expect(page.getByLabel('输入 / 百万 tokens', { exact: true })).toHaveValue('2.5');
    await page.getByTestId('settings-form-save').click();
    await expect(page.getByRole('form', { name: '模型价格配置' })).toHaveCount(0);
    const prices = await api(page, '/api/admin/usage/prices', undefined, 'GET');
    const saved = prices.items.find(item => item.model_provider_id === source.id);
    expect(saved.input_price_micro).toBe(2500000);
    await api(page, `/api/admin/usage/prices/${saved.id}`, undefined, 'DELETE');
  } finally { await api(page, `/api/model-providers/${source.id}`, undefined, 'DELETE'); }
});

test('settings remain usable at narrow widths, large text layout and both themes', async ({ page }) => {
  await login(page); const source = await model(page, '长名称模型用于验证设置页面自动换行与窄屏布局');
  try {
    await page.reload(); await open(page);
    for (const width of [1440, 1024, 760, 720, 390]) {
      await page.setViewportSize({ width, height: 900 });
      await expect(page.getByTestId('model-add-button')).toBeInViewport();
      expect(await page.getByTestId('settings-workspace').evaluate(root => root.scrollWidth <= root.clientWidth)).toBe(true);
      if (width <= 760) await expect(page.getByRole('combobox', { name: '设置分类', exact: true })).toBeVisible();
    }
    await page.screenshot({ path: '_local_artifacts/settings-refactor/mobile-light.png', fullPage: true });
    await page.getByTestId('model-add-button').click(); await page.getByTestId('model-id-input').fill('narrow-draft');
    expect(await page.locator('.settings-discovery-heading').evaluate(node => node.getBoundingClientRect().width)).toBeGreaterThan(250);
    await page.getByTestId('settings-form-save').scrollIntoViewIfNeeded(); await expect(page.getByTestId('settings-form-save')).toBeInViewport();
    await page.screenshot({ path: '_local_artifacts/settings-refactor/mobile-form.png', fullPage: true });
    await page.getByTestId('settings-return').click(); await discard(page);
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.getByRole('button', { name: '切换深色模式' }).click(); await open(page);
    await page.screenshot({ path: '_local_artifacts/settings-refactor/desktop-dark.png', fullPage: true });
    await page.getByTestId('settings-return').click();
    await page.getByRole('button', { name: '切换浅色模式' }).click(); await open(page);
    await page.screenshot({ path: '_local_artifacts/settings-refactor/desktop-light.png', fullPage: true });
  } finally { await api(page, `/api/model-providers/${source.id}`, undefined, 'DELETE'); }
});

test('long configuration lists wrap identifiers and keep bottom-row actions inside the viewport', async ({ page }) => {
  const items = Array.from({ length: 24 }, (_, index) => ({ id: `list-${index}`, name: `模型 ${index}`,
    model_id: `long-model-${'identifier'.repeat(12)}`, protocol: 'openai', enabled: true,
    visibility: 'private', base_url: 'https://example.invalid/v1' }));
  await page.route('**/api/model-providers', route => route.fulfill({ json: { items } }));
  await login(page); await open(page);
  for (const width of [1440, 390]) {
    await page.setViewportSize({ width, height: 720 });
    const row = page.getByTestId('provider-row-list-23');
    await row.scrollIntoViewIfNeeded();
    const trigger = row.getByLabel('更多操作 模型 23', { exact: true });
    await trigger.click();
    await expect(row.getByRole('button', { name: '删除模型', exact: true })).toBeInViewport();
    expect(await page.getByTestId('settings-workspace').evaluate(root => root.scrollWidth <= root.clientWidth)).toBe(true);
    await page.keyboard.press('Escape'); await expect(trigger).toBeFocused();
    await expect(row.getByRole('button', { name: '删除模型', exact: true })).toBeHidden();
  }
});

test('prompt selects the original name, traps focus and restores it after cancelling', async ({ page }) => {
  await page.route('**/api/skills', route => route.fulfill({ json: { items: [
    { id: 'prompt-fixture', name: '原始名称', enabled: true, visibility: 'private' },
  ] } }));
  await login(page); await open(page); await page.getByTestId('settings-nav-skills').click();
  const trigger = page.getByRole('button', { name: '重命名', exact: true });
  await trigger.click();
  const dialog = page.getByRole('dialog', { name: '重命名 Skill' });
  const input = dialog.getByRole('textbox');
  await expect(input).toBeFocused(); await expect(input).toHaveValue('原始名称');
  expect(await input.evaluate(node => node.value.substring(node.selectionStart, node.selectionEnd))).toBe('原始名称');
  await page.keyboard.insertText('替换名称'); await expect(input).toHaveValue('替换名称');
  await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0); await expect(trigger).toBeFocused();
  await expect(page.getByText('原始名称', { exact: true })).toBeVisible();
});

test('an active Agent continues receiving its result while settings are open', async ({ page }) => {
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  const mock = createServer(async (request, response) => {
    const chunks = []; for await (const chunk of request) chunks.push(chunk);
    const input = JSON.parse(Buffer.concat(chunks).toString());
    response.setHeader('Content-Type', 'application/json');
    if (!input.tools?.length) { response.end(JSON.stringify({ choices: [{ message: { role: 'assistant', content: '设置运行测试' } }] })); return; }
    await gate;
    response.end(JSON.stringify({ choices: [{ message: { role: 'assistant', tool_calls: [{
      id: 'settings-completion', type: 'function', function: { name: 'attempt_completion',
        arguments: JSON.stringify({ result: '设置切换期间任务已完成。', changed_paths: [] }) },
    }] }, finish_reason: 'tool_calls' }] }));
  });
  await new Promise(resolve => mock.listen(0, '127.0.0.1', resolve));
  let source, accepted;
  try {
    await login(page);
    source = await api(page, '/api/model-providers', { name: 'settings-active-run', protocol: 'openai',
      model_id: 'test-model', base_url: `http://127.0.0.1:${mock.address().port}/v1`, api_key: 'fixture-key', enabled: true, is_default: true });
    await page.reload();
    const response = page.waitForResponse(item => item.request().method() === 'POST' && /\/conversations\/[^/]+\/messages$/.test(item.url()));
    await page.getByTestId('chat-input').fill('只回复一句话，不要修改文件。'); await page.getByTestId('chat-send-button').click();
    accepted = await (await response).json();
    await expect.poll(async () => (await api(page, `/api/agent-runs/${accepted.run.id}`, undefined, 'GET')).status).toBe('running');
    await open(page); await page.getByTestId('settings-nav-search').click(); release();
    await expect.poll(async () => (await api(page, `/api/agent-runs/${accepted.run.id}`, undefined, 'GET')).status).toBe('succeeded');
    await page.getByTestId('settings-return').click();
    await expect(page.locator('.message-bubble-assistant').last()).toContainText('设置切换期间任务已完成。');
    await expect(page.getByTestId('chat-send-button')).toHaveAttribute('title', '发送');
  } finally {
    release();
    if (accepted?.run?.id) await api(page, `/api/agent-runs/${accepted.run.id}/cancel`, {}).catch(() => {});
    if (source) await api(page, `/api/model-providers/${source.id}`, undefined, 'DELETE').catch(() => {});
    mock.closeAllConnections(); await new Promise(resolve => mock.close(resolve));
  }
});

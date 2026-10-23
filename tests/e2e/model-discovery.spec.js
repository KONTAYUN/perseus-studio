import { createServer } from 'node:http';
import { expect, test } from '@playwright/test';

async function api(page, path, body, method = 'POST') {
  return page.evaluate(async ({ path, body, method }) => {
    const response = await fetch(path, { method, credentials: 'include', headers: { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body) });
    if (!response.ok) throw new Error(`API failed: ${response.status}`);
    return response.json();
  }, { path, body, method });
}

async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByTestId('settings-button')).toBeVisible();
}

test('one discovery supports multiple direct adds, retries and saved credentials', async ({ page }) => {
  let discoveries = 0;
  const mock = createServer((req, res) => {
    discoveries++;
    if (req.headers.authorization !== 'Bearer discovery-test-key') { res.writeHead(401).end(); return; }
    res.setHeader('Content-Type', 'application/json');
    res.end(JSON.stringify({ data: ['alpha', 'beta', 'gamma', ...Array.from({ length: 20 }, (_, i) => `extra-${i}`)].map(id => ({ id })) }));
  });
  await new Promise(resolve => mock.listen(0, '127.0.0.1', resolve));
  const baseURL = `http://127.0.0.1:${mock.address().port}/v1`;
  let oldDefault;
  try {
    await login(page);
    const before = await api(page, '/api/model-providers', undefined, 'GET');
    oldDefault = (before.items || []).find(item => item.is_default)?.id;
    const source = await api(page, '/api/model-providers', { name: 'discovery source', model_id: 'anchor', protocol: 'openai',
      base_url: baseURL, api_key: 'discovery-test-key', enabled: true, is_default: true });
    await page.reload();
    await page.getByTestId('settings-button').click();
    await page.getByTestId('model-add-button').click();
    await page.getByTestId('model-base-url-input').fill(baseURL);
    await page.getByTestId('model-api-key-input').fill('discovery-test-key');
    const form = page.locator('form').filter({ has: page.getByTestId('model-name-input') });
    await form.getByRole('button', { name: '获取模型列表', exact: true }).click();
    const list = page.getByTestId('discovered-model-list');
    await expect(list.getByRole('button', { name: '添加模型 alpha', exact: true })).toBeVisible();
    let creates = 0;
    let failBeta = true;
    await page.route('**/api/model-providers', async route => {
      if (route.request().method() === 'POST') {
        creates++;
        const input = route.request().postDataJSON();
        if (input.model_id === 'beta' && failBeta) {
          failBeta = false;
          await route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: '测试添加失败，请重试' }) });
          return;
        }
        await new Promise(resolve => setTimeout(resolve, 180));
      }
      await route.continue();
    });
    const addAlpha = list.getByRole('button', { name: '添加模型 alpha', exact: true });
    await addAlpha.evaluate(button => { button.click(); button.click(); });
    await expect(list.getByRole('button', { name: '已添加 alpha', exact: true })).toBeDisabled();
    expect(creates).toBe(1);
    await expect(page.getByTestId('model-base-url-input')).toHaveValue(baseURL);
    await expect(page.getByTestId('model-api-key-input')).toHaveValue('discovery-test-key');
    // Direct add must bypass unrelated required name/model-ID form fields.
    await expect(page.getByTestId('model-name-input')).toHaveValue('');
    await expect(page.getByTestId('model-id-input')).toHaveValue('');
    await list.getByRole('button', { name: '添加模型 beta', exact: true }).click();
    await expect(page.getByRole('alert')).toContainText('beta 添加失败');
    await expect(list.getByRole('button', { name: '添加模型 beta', exact: true })).toBeEnabled();
    await list.getByRole('button', { name: '添加模型 beta', exact: true }).click();
    await expect(list.getByRole('button', { name: '已添加 beta', exact: true })).toBeDisabled();
    expect(discoveries).toBe(1);
    const added = await api(page, '/api/model-providers', undefined, 'GET');
    expect(added.items.filter(item => item.base_url === baseURL).map(item => item.model_id).sort()).toEqual(['alpha', 'anchor', 'beta']);
    expect(added.items.find(item => item.is_default)?.id).toBe(source.id);
    await page.getByTestId('model-discovery-panel').screenshot({ path: '_local_artifacts/model-discovery-light.png' });
    // Editing uses the saved secret; adding gamma must create a new config, not overwrite the source.
    await form.getByRole('button', { name: '取消', exact: true }).click();
    await page.getByRole('button', { name: '放弃修改', exact: true }).click();
    const sourceCard = page.getByTestId(`provider-row-${source.id}`);
    await sourceCard.getByRole('button', { name: '编辑', exact: true }).click();
    await expect(page.getByTestId('model-api-key-input')).toHaveValue('');
    await form.getByRole('button', { name: '获取模型列表', exact: true }).click();
    await expect(list.getByRole('button', { name: '已添加 alpha', exact: true })).toBeDisabled();
    await list.getByRole('button', { name: '添加模型 gamma', exact: true }).click();
    await expect(list.getByRole('button', { name: '已添加 gamma', exact: true })).toBeDisabled();
    expect(discoveries).toBe(2);
    const final = await api(page, '/api/model-providers', undefined, 'GET');
    expect(final.items.find(item => item.id === source.id)?.model_id).toBe('anchor');
    expect(final.items.find(item => item.is_default)?.id).toBe(source.id);
    expect(final.items.filter(item => item.base_url === baseURL)).toHaveLength(4);
    await page.evaluate(() => document.documentElement.classList.add('dark'));
    await page.getByTestId('model-discovery-panel').screenshot({ path: '_local_artifacts/model-discovery-dark.png' });
    await page.getByTestId('model-base-url-input').fill(`${baseURL}/changed`);
    await expect(list).toHaveCount(0);
  } finally {
    await page.unroute('**/api/model-providers');
    try {
      const all = await api(page, '/api/model-providers', undefined, 'GET');
      for (const item of (all.items || []).filter(item => item.base_url === baseURL)) await api(page, `/api/model-providers/${item.id}`, undefined, 'DELETE');
      if (oldDefault) await api(page, `/api/model-providers/${oldDefault}/set-default`, {});
    } finally { await new Promise(resolve => mock.close(resolve)); }
  }
});

test('changing the connection discards an in-flight discovery result', async ({ page }) => {
  await login(page);
  await page.getByTestId('settings-button').click();
  await page.getByTestId('model-add-button').click();
  await page.getByTestId('model-base-url-input').fill('http://127.0.0.1:19991/v1');
  await page.getByTestId('model-api-key-input').fill('discovery-test-key');
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  let finished;
  const handled = new Promise(resolve => { finished = resolve; });
  await page.route('**/api/model-providers/discover', async route => {
    await gate;
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ models: [{ id: 'old-connection' }] }) }).catch(() => {});
    finished();
  });
  const request = page.waitForRequest('**/api/model-providers/discover');
  await page.getByRole('button', { name: '获取模型列表', exact: true }).click();
  await request;
  await page.getByTestId('model-base-url-input').fill('http://127.0.0.1:19992/v1');
  release();
  await handled;
  await expect(page.getByTestId('discovered-model-list')).toHaveCount(0);
  await expect(page.getByRole('button', { name: '获取模型列表', exact: true })).toBeEnabled();
});

import { createServer } from 'node:http';
import { expect, test } from '@playwright/test';

async function api(page, path, body, method = 'POST') {
  return page.evaluate(async ({ path, body, method }) => {
    const response = await fetch(path, {
      method, credentials: 'include', headers: { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!response.ok) throw new Error(`${response.status}: ${await response.text()}`);
    return response.json();
  }, { path, body, method });
}

for (const protocol of ['openai', 'anthropic']) {
  test(`${protocol}: chat tool call edits disk and refreshes the open editor`, async ({ page }) => {
    const path = `agent-edit-${protocol}-${Date.now()}.md`;
    let round = 0;
    let patchVisible = false;
    const systemCounts = [];
    const mock = createServer(async (request, response) => {
      const chunks = [];
      for await (const chunk of request) chunks.push(chunk);
      const payload = JSON.parse(Buffer.concat(chunks).toString());
      const names = (payload.tools || []).map((tool) => tool.name || tool.function?.name);
      systemCounts.push((payload.messages || []).filter((message) => message.role === 'system').length);
      patchVisible ||= names.includes('apply_workspace_patch');
      if (!names.includes('read_workspace_text')) {
        response.setHeader('Content-Type', 'application/json');
        response.end(JSON.stringify(protocol === 'openai'
          ? { choices: [{ message: { role: 'assistant', content: '编辑测试' } }] }
          : { content: [{ type: 'text', text: '编辑测试' }], stop_reason: 'end_turn' }));
        return;
      }
      const step = round++;
      const name = ['read_workspace_text', 'apply_workspace_patch', 'attempt_completion'][Math.min(step, 2)];
      const args = step === 0 ? { path } : step === 1
        ? { patch: `*** Begin Patch\n*** Update File: ${path}\n@@\n-before\n+after\n*** End Patch` }
        : { result: `已修改 ${path}。`, changed_paths: [path] };
      if (protocol === 'openai') {
        // Exercise real provider streaming, backend SSE, and frontend tool refresh.
        response.setHeader('Content-Type', 'text/event-stream');
        response.write(`data: ${JSON.stringify({ id: 'chatcmpl-test', choices: [{ index: 0, delta: {
          tool_calls: [{ index: 0, id: `call_${step}`, type: 'function', function: { name, arguments: JSON.stringify(args) } }],
        }, finish_reason: null }] })}\n\n`);
        response.end(`data: ${JSON.stringify({ choices: [{ index: 0, delta: {}, finish_reason: 'tool_calls' }] })}\n\ndata: [DONE]\n\n`);
      } else {
        // Also cover providers returning ordinary JSON to a streaming request.
        response.setHeader('Content-Type', 'application/json');
        response.end(JSON.stringify({ id: 'msg_test', type: 'message', role: 'assistant', stop_reason: 'tool_use',
          content: [{ type: 'tool_use', id: `call_${step}`, name, input: args }] }));
      }
    });
    await new Promise((resolve) => mock.listen(0, '127.0.0.1', resolve));
    let provider;
    try {
      await page.goto('/');
      await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
      await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
      await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
      await expect(page.getByTestId('chat-send-button')).toBeVisible();
      provider = await api(page, '/api/model-providers', {
        name: `E2E ${protocol}`, protocol, base_url: `http://127.0.0.1:${mock.address().port}/v1`,
        api_key: 'local-test', model_id: protocol === 'openai' ? 'grok-test-model' : 'test-model', enabled: true, is_default: true,
      });
      await api(page, '/api/files/create', { path, content: 'before\n' });
      await page.reload();
      await page.getByTestId('layout-3').click();
      await page.getByTestId(`file-row-${path}`).click();
      await page.getByRole('button', { name: '切换为纯文本（当前：富文本）', exact: true }).click();
      await expect(page.getByTestId('editor-textarea')).toHaveValue('before\n');
      let acceptedRun;
      let recoveredRequests = 0;
      if (protocol === 'openai') {
        await page.route('**/api/agent-runs/by-request/*', async (route) => {
          recoveredRequests++;
          await route.continue();
        });
        await page.route('**/api/conversations/*/messages', async (route) => {
          if (route.request().method() !== 'POST' || acceptedRun) return route.continue();
          const accepted = await route.fetch();
          expect(accepted.status()).toBe(202);
          acceptedRun = await accepted.json();
          // The server committed the input; the browser loses only the response.
          await route.abort('failed');
        });
      }
      await page.getByTestId('chat-input').fill(`把 ${path} 中的 before 改为 after，并保存文件。`);
      await page.getByTestId('chat-send-button').click();
      await expect(page.getByTestId('editor-textarea')).toHaveValue('after\n', { timeout: 20_000 });
      const file = await api(page, `/api/files/read?path=${encodeURIComponent(path)}`, undefined, 'GET');
      expect(file.content).toBe('after\n');
      expect(patchVisible).toBe(true);
      expect(round).toBeGreaterThanOrEqual(2);
      if (protocol === 'openai') {
        expect(recoveredRequests).toBe(1);
        const history = await api(page, `/api/conversations/${acceptedRun.conversation.id}/messages`, undefined, 'GET');
        expect(history.items.filter((item) => item.role === 'user')).toHaveLength(1);
      }
      const review = page.getByRole('button', { name: /查看变更/ }).last();
      await expect(review).toBeVisible();
      await review.click();
      await expect(page.getByText('修改前', { exact: true })).toBeVisible();
      await expect(page.locator('.workspace-change-content').first()).toContainText('before');
      await expect(page.locator('.workspace-change-content').last()).toContainText('after');
      await page.screenshot({ path: `_local_artifacts/change-review-${protocol}.png`, fullPage: true });
      if (protocol === 'anthropic') {
        await page.evaluate(() => document.documentElement.classList.add('dark'));
        await page.setViewportSize({ width: 600, height: 850 });
        await page.getByTestId('layout-1').click();
        await expect.poll(() => page.locator('.panel-wrapper').evaluateAll((nodes) => nodes.reduce((n, node) => n + node.getAnimations().length, 0))).toBe(0);
        await expect(page.locator('.workspace-change-columns')).toHaveCSS('grid-template-columns', /^\d+(\.\d+)?px$/);
        const bounds = await page.locator('.workspace-change-preview').boundingBox();
        expect(bounds.x).toBeGreaterThanOrEqual(0);
        expect(bounds.x + bounds.width).toBeLessThanOrEqual(600);
        await page.screenshot({ path: '_local_artifacts/change-review-dark-narrow.png', fullPage: true });
        await page.setViewportSize({ width: 1280, height: 720 });
        await page.getByTestId('layout-3').click();
      }
      await api(page, '/api/files/write', { path, content: 'new user content' });
      await page.getByRole('button', { name: '撤销这一处', exact: true }).click();
      await page.getByRole('button', { name: '确认撤销', exact: true }).click();
      await expect(page.getByRole('alert')).toContainText('文件已被其他操作修改');
      expect((await api(page, `/api/files/read?path=${encodeURIComponent(path)}`, undefined, 'GET')).content).toBe('new user content');
      await api(page, '/api/files/write', { path, content: 'after\n' });
      await page.getByRole('button', { name: '刷新状态', exact: true }).click();
      await page.getByRole('button', { name: '撤销这一处', exact: true }).click();
      await page.getByRole('button', { name: '确认撤销', exact: true }).click();
      await expect(page.getByText('已撤销此处变更。', { exact: true })).toBeVisible();
      await expect(page.getByTestId('editor-textarea')).toHaveValue('before\n');

      if (protocol === 'openai') {
        expect(systemCounts.every((count) => count === 1)).toBe(true);
        round = 0; // Regeneration performs a fresh read/edit/completion cycle.
        await expect(page.getByTitle('重新生成', { exact: true })).toBeVisible();
        const regeneration = page.waitForResponse((response) => response.url().endsWith('/regenerate-last'));
        await page.getByTitle('重新生成', { exact: true }).click();
        await page.getByRole('button', { name: '继续重新生成', exact: true }).click();
        const accepted = await regeneration;
        expect(accepted.status()).toBe(202);
        const regenerated = await accepted.json();
        expect(regenerated.run.id).not.toBe(acceptedRun.run.id);
        await expect.poll(async () => (await api(page, `/api/agent-runs/${regenerated.run.id}`, undefined, 'GET')).status).toBe('succeeded');
        const history = await api(page, `/api/conversations/${acceptedRun.conversation.id}/messages`, undefined, 'GET');
        expect(history.items.filter((item) => item.role === 'user')).toHaveLength(1);
      }
    } finally {
      await api(page, '/api/files/delete', { path }).catch(() => {});
      if (provider) await api(page, `/api/model-providers/${provider.id}`, undefined, 'DELETE').catch(() => {});
      mock.closeAllConnections();
      await new Promise((resolve) => mock.close(resolve));
    }
  });
}

test('autosave preserves external changes until the user explicitly saves the local version', async ({ page }) => {
  const path = `save-conflict-${Date.now()}.md`;
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
  try {
    await api(page, '/api/files/create', { path, content: 'original' });
    await page.reload();
    await page.getByTestId('layout-3').click();
    await page.getByTestId(`file-row-${path}`).click();
    await page.getByRole('button', { name: '切换为纯文本（当前：富文本）', exact: true }).click();
    const editor = page.getByTestId('editor-textarea');
    await expect(editor).toHaveValue('original');
    await api(page, '/api/files/write', { path, content: 'external AI version' });
    await editor.fill('local unsaved version');
    await expect(page.getByRole('button', { name: '保留本地修改', exact: true })).toBeVisible();
    await expect(editor).toHaveValue('local unsaved version');
    const read = () => api(page, `/api/files/read?path=${encodeURIComponent(path)}`, undefined, 'GET');
    expect((await read()).content).toBe('external AI version');
    await page.getByRole('button', { name: '保留本地修改', exact: true }).click();
    expect((await read()).content).toBe('external AI version');
    await page.getByRole('button', { name: '保存本地版本', exact: true }).click();
    await expect.poll(async () => (await read()).content).toBe('local unsaved version');
    await expect(page.getByRole('button', { name: '保存本地版本', exact: true })).toBeHidden();
  } finally { await api(page, '/api/files/delete', { path }).catch(() => {}); }
});

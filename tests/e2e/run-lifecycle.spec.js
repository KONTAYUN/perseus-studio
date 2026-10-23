import { createServer } from 'node:http';
import { expect, test } from '@playwright/test';

test('stop during acceptance cancels the committed run after its identity arrives', async ({ page }) => {
  const mock = createServer(async (request, response) => {
    const chunks = [];
    for await (const chunk of request) chunks.push(chunk);
    const payload = JSON.parse(Buffer.concat(chunks).toString());
    if (payload.tools?.length) return; // Keep inference active until cancellation.
    response.setHeader('Content-Type', 'application/json');
    response.end(JSON.stringify({ choices: [{ message: { role: 'assistant', content: '停止测试' } }] }));
  });
  await new Promise((resolve) => mock.listen(0, '127.0.0.1', resolve));
  const api = async (path, body, method = 'POST') => page.evaluate(async ({ path, body, method }) => {
    const response = await fetch(path, { method, headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
    if (!response.ok) throw new Error(`${response.status}: ${await response.text()}`);
    return response.json();
  }, { path, body, method });
  let provider, accepted, release;
  const heldResponse = new Promise((resolve) => { release = resolve; });
  try {
    await page.goto('/');
    await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
    await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
    await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    provider = await api('/api/model-providers', {
      name: 'E2E acceptance cancel', protocol: 'openai', base_url: `http://127.0.0.1:${mock.address().port}/v1`,
      api_key: 'local-test', model_id: 'test-model', enabled: true, is_default: true,
    });
    await page.reload();
    await page.route('**/api/conversations/*/messages', async (route) => {
      if (route.request().method() !== 'POST') return route.continue();
      const response = await route.fetch();
      expect(response.status()).toBe(202);
      accepted = await response.json();
      await heldResponse;
      await route.fulfill({ response });
    });
    await page.getByTestId('chat-input').fill('只回复完成。');
    await page.getByTestId('chat-send-button').click();
    await expect.poll(() => Boolean(accepted?.run?.id)).toBe(true);
    await expect(page.getByTestId('chat-send-button')).toHaveAttribute('title', '停止生成');
    await page.getByTestId('chat-send-button').click();
    release();
    await expect.poll(async () => (await api(`/api/agent-runs/${accepted.run.id}`, undefined, 'GET')).status).toBe('canceled');
    await expect(page.getByTestId('chat-send-button')).toHaveAttribute('title', '发送');
    const history = await api(`/api/conversations/${accepted.conversation.id}/messages`, undefined, 'GET');
    expect(history.items.filter((item) => item.role === 'user')).toHaveLength(1);
    expect(history.items.filter((item) => item.role === 'assistant')).toHaveLength(0);
  } finally {
    release();
    if (accepted?.run?.id) await api(`/api/agent-runs/${accepted.run.id}/cancel`, {}).catch(() => {});
    if (provider) await api(`/api/model-providers/${provider.id}`, undefined, 'DELETE').catch(() => {});
    mock.closeAllConnections();
    await new Promise((resolve) => mock.close(resolve));
  }
});

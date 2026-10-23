import { createServer } from 'node:http';
import { expect, test } from '@playwright/test';

const ownedProviders = new WeakMap();
test.afterEach(async ({ page }) => {
  const id = ownedProviders.get(page);
  if (id) await api(page, `/api/model-providers/${id}`, undefined, 'DELETE');
});

async function api(page, path, body, method = body === undefined ? 'GET' : 'POST') {
  return page.evaluate(async ({ path, body, method }) => {
    const r = await fetch(path, { method, headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
    if (!r.ok) throw Error(`${r.status} ${await r.text()}`);
    return r.json();
  }, { path, body, method });
}
async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
}
async function select(page, conversation) {
  await page.getByTestId('conversation-history-button').click();
  await page.locator('.conversation-item-row').filter({ has: page.getByText(conversation.title, { exact: true }) }).click();
}
async function newConversation(page, title) {
  const providers = (await api(page, '/api/model-providers')).items || [];
  let provider = providers[0];
  if (!provider) {
    provider = await api(page, '/api/model-providers', { name: 'UI regression', protocol: 'openai', base_url: 'http://127.0.0.1:9/v1', api_key: 'local-test', model_id: 'test-model', enabled: true, is_default: true });
    ownedProviders.set(page, provider.id);
  }
  return api(page, '/api/conversations', { title, selected_model_provider_id: provider.id });
}

test('queued follow-up survives reload and is committed exactly once', async ({ page }) => {
  test.setTimeout(60000);
  let release;
  const held = new Promise(resolve => { release = resolve; });
  let heldOnce = false;
  const mock = createServer(async (request, response) => {
    const chunks = []; for await (const c of request) chunks.push(c);
    const payload = JSON.parse(Buffer.concat(chunks).toString());
    if (payload.tools?.length && !heldOnce) { heldOnce = true; await held; }
    response.setHeader('Content-Type', 'application/json');
    response.end(JSON.stringify({ choices: [{ message: payload.tools?.length
      ? { role: 'assistant', tool_calls: [{ id: 'done', type: 'function', function: { name: 'attempt_completion', arguments: JSON.stringify({ result: '已完成验收回复。' }) } }] }
      : { role: 'assistant', content: '验收会话' } }] }));
  });
  await new Promise(resolve => mock.listen(0, '127.0.0.1', resolve));
  let provider, run;
  try {
    await login(page);
    provider = await api(page, '/api/model-providers', { name: 'Queue regression', protocol: 'openai', base_url: `http://127.0.0.1:${mock.address().port}/v1`, api_key: 'local-test', model_id: 'test-model', enabled: true, is_default: true });
    const conversation = await api(page, '/api/conversations', { title: `queue-${Date.now()}`, selected_model_provider_id: provider.id });
    await page.reload(); await select(page, conversation);
    const accepted = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/messages'));
    await page.getByTestId('chat-input').fill('请只回复完成，不要写文件。'); await page.getByTestId('chat-send-button').click();
    run = (await (await accepted).json()).run;
    await expect.poll(() => heldOnce).toBe(true);
    const queued = `FOLLOW_UP_${Date.now()}`;
    await page.getByTestId('chat-input').fill(queued); await page.getByTestId('chat-send-button').click();
    await expect(page.locator('.pending-run-message-panel')).toContainText(queued);
    await page.route('**/api/pending-messages', route => route.request().method() === 'POST'
      ? route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ error: 'injected save failure' }) }) : route.continue());
    await page.getByTestId('chat-input').fill('KEEP_DRAFT_ON_QUEUE_FAILURE');
    await page.getByTestId('chat-send-button').click();
    await expect(page.getByText('排队消息未能保存，原输入已保留，请检查连接后重试。', { exact: true })).toBeVisible();
    await expect(page.getByTestId('chat-input')).toHaveValue('KEEP_DRAFT_ON_QUEUE_FAILURE');
    await expect(page.locator('.pending-run-message-panel')).toContainText(queued);
    await page.unroute('**/api/pending-messages');
    await page.getByTestId('chat-input').fill('REPLACEMENT_TO_DISCARD'); await page.getByTestId('chat-send-button').click();
    await expect(page.locator('.pending-run-message-panel')).toContainText('REPLACEMENT_TO_DISCARD');
    await page.getByRole('button', { name: '删除挂起消息', exact: true }).click();
    await expect(page.locator('.pending-run-message-panel')).toHaveCount(0);
    await page.reload(); await select(page, conversation);
    expect((await api(page, '/api/pending-messages')).items).toHaveLength(0);
    await page.getByTestId('chat-input').fill(queued);
    await expect(page.getByTestId('chat-send-button')).toHaveAttribute('title', '挂起发送');
    await page.getByTestId('chat-send-button').click();
    await expect(page.locator('.pending-run-message-panel')).toContainText(queued);
    await page.reload(); await select(page, conversation);
    await expect(page.locator('.pending-run-message-panel')).toContainText(queued);
    let lost = false, recovered = 0;
    await page.route('**/api/pending-messages/*/send', async route => {
      if (lost) return route.continue();
      const response = await route.fetch();
      if (response.status() !== 202) return route.fulfill({ response });
      lost = true; await route.abort('failed');
    });
    await page.route('**/api/agent-runs/by-request/*', route => { recovered++; return route.continue(); });
    release();
    await expect.poll(async () => (await api(page, `/api/conversations/${conversation.id}/messages`)).items.filter(m => m.role === 'user' && m.content === queued).length, { timeout: 20000 }).toBe(1);
    await expect(page.locator('.pending-run-message-panel')).toHaveCount(0);
    await expect.poll(() => recovered).toBe(1);
    expect(lost).toBe(true);
    await page.reload(); await select(page, conversation);
    expect((await api(page, `/api/conversations/${conversation.id}/messages`)).items.filter(m => m.role === 'user' && m.content === queued)).toHaveLength(1);
  } finally {
    release();
    if (run) await api(page, `/api/agent-runs/${run.id}/cancel`, {}).catch(() => {});
    if (provider) await api(page, `/api/model-providers/${provider.id}`, undefined, 'DELETE').catch(() => {});
    mock.closeAllConnections(); await new Promise(resolve => mock.close(resolve));
  }
});

test('compaction preserves the draft for success, no-op and failure', async ({ page }) => {
  await login(page);
  const conversation = await newConversation(page, `compact-${Date.now()}`);
  await page.reload(); await select(page, conversation);
  for (const mode of ['success', 'no-op', 'failure']) {
    await page.route('**/api/conversations/*/compact', route => route.fulfill({ status: mode === 'failure' ? 500 : 200, contentType: 'application/json', body: JSON.stringify(mode === 'failure' ? { error: 'injected failure' } : { updated: mode === 'success', compacted_message_count: mode === 'success' ? 5 : 0 }) }));
    const draft = `UNSENT_DRAFT_${mode}`;
    await page.getByTestId('chat-input').fill(draft);
    await page.getByTestId('context-usage-button').click();
    const response = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/compact'));
    await page.getByRole('button', { name: '压缩上下文', exact: true }).click(); await response;
    await expect(page.getByTestId('chat-input')).toHaveValue(draft);
    expect(((await api(page, `/api/conversations/${conversation.id}/messages`)).items || []).filter(m => m.role === 'user')).toHaveLength(0);
    await page.unroute('**/api/conversations/*/compact');
  }
});

test('a reloaded active run can be stopped and its pending input remains paused', async ({ page }) => {
  const mock = createServer(async (request, response) => {
    const chunks = []; for await (const c of request) chunks.push(c);
    const payload = JSON.parse(Buffer.concat(chunks).toString());
    if (payload.tools?.length) return;
    response.setHeader('Content-Type', 'application/json');
    response.end(JSON.stringify({ choices: [{ message: { role: 'assistant', content: '停止回归' } }] }));
  });
  await new Promise(resolve => mock.listen(0, '127.0.0.1', resolve));
  let provider, run;
  try {
    await login(page);
    provider = await api(page, '/api/model-providers', { name: 'Recovered stop regression', protocol: 'openai', base_url: `http://127.0.0.1:${mock.address().port}/v1`, api_key: 'local-test', model_id: 'test-model', enabled: true, is_default: true });
    const conversation = await api(page, '/api/conversations', { title: `reload-stop-${Date.now()}`, selected_model_provider_id: provider.id });
    await page.reload(); await select(page, conversation);
    const accepted = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/messages'));
    await page.getByTestId('chat-input').fill('等待停止。'); await page.getByTestId('chat-send-button').click();
    run = (await (await accepted).json()).run;
    await page.getByTestId('chat-input').fill('PAUSED_FOLLOW_UP'); await page.getByTestId('chat-send-button').click();
    await expect(page.locator('.pending-run-message-panel')).toContainText('PAUSED_FOLLOW_UP');
    await page.reload(); await select(page, conversation);
    await expect(page.getByTestId('chat-send-button')).toHaveAttribute('title', '停止生成');
    await page.getByTestId('chat-send-button').click();
    await expect.poll(async () => (await api(page, `/api/agent-runs/${run.id}`)).status).toBe('canceled');
    await expect(page.locator('.pending-run-message-panel')).toContainText('上一轮已暂停');
    expect((await api(page, '/api/pending-messages')).items.some(item => item.conversationId === conversation.id && item.content === 'PAUSED_FOLLOW_UP')).toBe(true);
    await page.getByRole('button', { name: '删除挂起消息', exact: true }).click();
    await expect(page.locator('.pending-run-message-panel')).toHaveCount(0);
  } finally {
    if (run) await api(page, `/api/agent-runs/${run.id}/cancel`, {}).catch(() => {});
    if (provider) await api(page, `/api/model-providers/${provider.id}`, undefined, 'DELETE').catch(() => {});
    mock.closeAllConnections(); await new Promise(resolve => mock.close(resolve));
  }
});

test('rename closes history and its save button receives ordinary clicks', async ({ page }) => {
  await login(page);
  const conversation = await newConversation(page, `rename-${Date.now()}`);
  await page.reload(); await select(page, conversation);
  await page.getByTestId('conversation-history-button').click();
  const row = page.locator('.conversation-item-row').filter({ has: page.getByText(conversation.title, { exact: true }) });
  await row.hover(); await row.getByRole('button', { name: '重命名会话', exact: true }).click();
  await page.getByPlaceholder('会话名称').fill(`${conversation.title}-changed`);
  await page.getByRole('button', { name: '保存', exact: true }).click({ timeout: 3000 });
  await expect(page.getByTestId('conversation-history-button')).toContainText(`${conversation.title}-changed`);
  await expect(page.getByTestId('conversation-history-button')).toBeFocused();
  await page.getByTestId('conversation-history-button').click();
  const renamed = page.locator('.conversation-item-row').filter({ has: page.getByText(`${conversation.title}-changed`, { exact: true }) });
  await renamed.hover(); await renamed.getByRole('button', { name: '重命名会话', exact: true }).click();
  const dialog = page.getByRole('dialog', { name: '重命名会话' });
  await expect(dialog).toBeVisible();
  await page.getByPlaceholder('会话名称').press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(page.getByTestId('conversation-history-button')).toBeFocused();
});

test('deleting the active document activates the remaining tab without losing its text', async ({ page }) => {
  await login(page); const stamp = Date.now();
  const kept = `remaining-${stamp}.md`, removed = `removed-${stamp}.md`;
  await api(page, '/api/files/create', { path: kept, content: '# Kept\n\nKEEP_REMAINING_TEXT\n' });
  await api(page, '/api/files/create', { path: removed, content: '# Deleted\n' });
  await page.reload(); await page.getByTestId('layout-3').click();
  await page.getByTestId(`file-row-${kept}`).click(); await page.getByTestId(`file-row-${removed}`).click();
  await page.getByTestId(`file-row-${removed}`).hover();
  await page.getByTestId(`file-row-${removed}`).getByRole('button', { name: '删除', exact: true }).click();
  await page.getByTestId('file-dialog-submit').click();
  await expect(page.getByTestId('rich-text-editor')).toContainText('KEEP_REMAINING_TEXT');
  expect((await api(page, `/api/files/read?path=${kept}`)).content).toBe('# Kept\n\nKEEP_REMAINING_TEXT\n');
});

import { createServer } from 'node:http';
import { expect, test } from '@playwright/test';

async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
}
async function api(page, path, body, method = body === undefined ? 'GET' : 'POST') {
  return page.evaluate(async ({ path, body, method }) => {
    const r = await fetch(path, { method, headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
    if (!r.ok) throw new Error(`HTTP ${r.status}: ${path}`);
    return r.json();
  }, { path, body, method });
}

test('uploaded text and image survive reload, next turn and regeneration; failure has one terminal', async ({ page }) => {
  const calls = [];
  let fail = false;
  const mock = createServer(async (request, response) => {
    const chunks = [];
    for await (const chunk of request) chunks.push(chunk);
    const payload = JSON.parse(Buffer.concat(chunks).toString());
    if (payload.tools?.length) calls.push(payload);
    if (fail === 'unauthorized' && payload.tools?.length) { response.writeHead(401); response.end('{"error":{"message":"fixture unauthorized"}}'); return; }
    response.setHeader('Content-Type', 'application/json');
    response.end(JSON.stringify({ choices: [{ message: { role: 'assistant', content: fail === 'empty' ? '' : '附件回归已完成。' }, finish_reason: 'stop' }] }));
  });
  await new Promise(resolve => mock.listen(0, '127.0.0.1', resolve));
  let provider;
  try {
    await login(page);
    provider = await api(page, '/api/model-providers', { name: `upload-fix-${Date.now()}`, protocol: 'openai',
      base_url: `http://127.0.0.1:${mock.address().port}/v1`, api_key: 'fixture', model_id: 'mock', enabled: true, is_default: true });
    await page.reload();
    await page.getByTestId('conversation-history-button').click();
    await page.getByTestId('conversation-menu-create-button').click();
    const image = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==', 'base64');
    await page.getByTestId('chat-attachment-input').setInputFiles([
      { name: 'fact.txt', mimeType: 'text/plain', buffer: Buffer.from('PERSISTED_FACT_8127') },
      { name: 'pixel.png', mimeType: 'image/png', buffer: image },
    ]);
    async function send(content) {
      const accepted = page.waitForResponse(r => r.request().method() === 'POST' && /\/api\/conversations\/[^/]+\/messages$/.test(new URL(r.url()).pathname));
      await page.getByTestId('chat-input').fill(content);
      await page.getByTestId('chat-send-button').click();
      return (await (await accepted).json()).run;
    }
    const first = await send('阅读附件，不要修改文件。');
    await expect.poll(async () => (await api(page, `/api/agent-runs/${first.id}`)).status).toBe('succeeded');
    const title = `附件持久化-${first.id}`;
    await api(page, `/api/conversations/${first.conversation_id}`, { title }, 'PATCH');
    await page.reload();
    await page.getByTestId('conversation-history-button').click();
    await page.getByTestId('conversation-history-popover').getByText(title, { exact: true }).click();
    const second = await send('请继续参考上轮附件。');
    expect(second.conversation_id).toBe(first.conversation_id);
    await expect.poll(async () => (await api(page, `/api/agent-runs/${second.id}`)).status).toBe('succeeded');
    expect(calls.length).toBeGreaterThanOrEqual(2);
    const lastWire = JSON.stringify(calls.at(-1).messages);
    expect(lastWire).toContain('PERSISTED_FACT_8127');
    expect(lastWire).toContain(image.toString('base64'));
    const history = (await api(page, `/api/conversations/${first.conversation_id}/messages`)).items;
    expect(JSON.stringify(history)).not.toContain('PERSISTED_FACT_8127');
    expect(JSON.stringify(history)).not.toContain(image.toString('base64'));
    const regen = await api(page, `/api/conversations/${first.conversation_id}/regenerate-last`, { message_id: history.at(-1).id });
    await expect.poll(async () => (await api(page, `/api/agent-runs/${regen.run.id}`)).status).toBe('succeeded');
    expect(JSON.stringify(calls.at(-1).messages)).toContain('PERSISTED_FACT_8127');
    fail = 'unauthorized';
    await page.reload();
    await page.getByTestId('conversation-history-button').click();
    await page.getByTestId('conversation-history-popover').getByText(title, { exact: true }).click();
    const failed = await send('触发可控供应商故障。');
    await expect.poll(async () => (await api(page, `/api/agent-runs/${failed.id}`)).status).toBe('failed');
    const events = (await api(page, `/api/agent-runs/${failed.id}/events`)).items;
    expect(events.filter(e => e.type === 'run_failed')).toHaveLength(1);
    fail = 'empty';
    const empty = await send('触发空回复故障。');
    await expect.poll(async () => (await api(page, `/api/agent-runs/${empty.id}`)).status).toBe('failed');
    const emptyEvents = (await api(page, `/api/agent-runs/${empty.id}/events`)).items;
    expect(emptyEvents.filter(e => e.type === 'run_failed')).toHaveLength(1);
    expect(emptyEvents.filter(e => e.type === 'agent_execution_failed')).toHaveLength(1);
  } finally {
    if (provider) await api(page, `/api/model-providers/${provider.id}`, undefined, 'DELETE').catch(() => {});
    mock.closeAllConnections();
    await new Promise(resolve => mock.close(resolve));
  }
});

test('chat cards refresh without task panel; old and deleted tasks never show stale pending', async ({ page }) => {
  let status = 'pending';
  let absent = false;
  await page.route(/\/api\/conversations$/, route => route.fulfill({ json: { items: [{ id: 'card-fixture', title: '实时卡片' }] } }));
  await page.route('**/api/conversations/card-fixture/context-usage', route => route.fulfill({ json: { usage: null } }));
  await page.route('**/api/conversations/card-fixture/messages', route => route.fulfill({ json: { items: [{ id: 'card', role: 'assistant', content: 'https://example.com/guide，不证明事实。', attachments: [{ type: 'task', task_id: 'fixture-task', title: '历史任务', status: 'pending' }] }] } }));
  await page.route(/\/api\/tasks$/, route => route.fulfill({ json: { items: absent ? [] : [{ id: 'fixture-task', title: '历史任务', status }] } }));
  await page.route('**/api/tasks/fixture-task', route => absent ? route.fulfill({ status: 404, json: { error: 'missing' } }) : route.fulfill({ json: { id: 'fixture-task', status } }));
  await login(page);
  await page.getByTestId('conversation-history-button').click();
  await page.getByTestId('conversation-history-popover').getByText('实时卡片', { exact: true }).click();
  await expect(page.getByText('状态：等待执行', { exact: true })).toBeVisible();
  status = 'done';
  await expect(page.getByText('状态：已完成', { exact: true })).toBeVisible();
  const link = page.locator('.message-bubble-assistant a');
  await expect(link).toHaveAttribute('href', 'https://example.com/guide');
  await expect(page.locator('.message-bubble-assistant')).toContainText('，不证明事实。');
  absent = true;
  await expect(page.getByText('状态：任务已删除或不可访问', { exact: true })).toBeVisible();
});


test('binary file and mixed folder downloads retain every byte', async ({ page }) => {
  await login(page);
  const folder = 'binary-download-' + Date.now();
  const path = folder + '/中文.pdf';
  const bytes = [0, 255, 128, 13, 10, 65, 0];
  await api(page, '/api/files/mkdir', { path: folder });
  await page.evaluate(async ({ path, bytes }) => {
    const form = new FormData();
    form.append('path', path);
    form.append('file', new File([new Uint8Array(bytes)], '中文.pdf', { type: 'application/pdf' }));
    const r = await fetch('/api/files/upload', { method: 'POST', body: form });
    if (!r.ok) throw new Error('upload ' + r.status);
  }, { path, bytes });
  await api(page, '/api/files/create', { path: folder + '/draft.md', content: '# 标题\r\n' });
  await page.reload();
  await page.getByTestId('layout-3').click();
  await page.getByTestId('toggle-folder-' + folder).click();
  async function download(target) {
    await page.getByTestId('file-row-' + target).hover();
    const pending = page.waitForEvent('download');
    await page.getByTestId('download-' + target).click();
    const item = await pending;
    const chunks = [];
    for await (const chunk of await item.createReadStream()) chunks.push(chunk);
    return Buffer.concat(chunks);
  }
  expect(await download(path)).toEqual(Buffer.from(bytes));
  const zip = await download(folder);
  const entries = new Map();
  let offset = 0;
  while (zip.readUInt32LE(offset) === 0x04034b50) {
    const size = zip.readUInt32LE(offset + 18);
    const nameLength = zip.readUInt16LE(offset + 26);
    const extra = zip.readUInt16LE(offset + 28);
    const name = zip.subarray(offset + 30, offset + 30 + nameLength).toString('utf8');
    const start = offset + 30 + nameLength + extra;
    entries.set(name, zip.subarray(start, start + size));
    offset = start + size;
  }
  expect(entries.size).toBe(2);
  expect(entries.get(path)).toEqual(Buffer.from(bytes));
  expect(entries.get(folder + '/draft.md').toString('utf8')).toBe('# 标题\r\n');
});


test('file reorder uses drop coordinates even without committed hover state', async ({ page }) => {
  await login(page);
  const folder = 'drop-state-' + Date.now();
  await api(page, '/api/files/mkdir', { path: folder });
  for (const name of ['a.txt', 'b.txt', 'c.txt']) await api(page, '/api/files/create', { path: folder + '/' + name, content: name });
  try {
    await page.reload();
    await page.getByTestId('layout-3').click();
    await page.getByTestId('toggle-folder-' + folder).click();
    const target = page.getByTestId('file-row-' + folder + '/b.txt');
    const reordered = page.waitForResponse(r => r.request().method() === 'POST' && r.url().endsWith('/api/files/reorder'));
    await target.evaluate((element, source) => {
      const rect = element.getBoundingClientRect();
      const dataTransfer = new DataTransfer();
      dataTransfer.setData('workspace_path', source);
      element.dispatchEvent(new DragEvent('drop', { bubbles: true, cancelable: true, dataTransfer, clientX: rect.x + 100, clientY: rect.y + 2 }));
    }, folder + '/c.txt');
    const response = await reordered;
    expect(response.status()).toBe(200);
    expect(response.request().postDataJSON()).toMatchObject({ path: folder + '/c.txt', previous_path: folder + '/a.txt', next_path: folder + '/b.txt' });
    await page.reload();
    await expect.poll(async () => page.locator('[data-file-path]').evaluateAll((elements, folder) => elements.map(e => e.dataset.filePath).filter(p => p.startsWith(folder + '/') && p.endsWith('.txt')), folder)).toEqual(['a.txt','c.txt','b.txt'].map(n => folder + '/' + n));
  } finally { await api(page, '/api/files/delete', { path: folder }).catch(() => {}); }
});

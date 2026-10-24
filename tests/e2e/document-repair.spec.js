import { createServer } from 'node:http';
import { expect, test } from '@playwright/test';
import { api, open, read, rich } from './helpers/document.js';

test('authorized repair stages a restricted candidate, keeps composer draft and requires confirmation', async ({ page }) => {
  test.setTimeout(60_000);
  const original = '<details>原始载荷</details>\n\n_正文 123_\n';
  const path = await open(page, original);
  let round = 0;
  const tools = [];
  const server = createServer(async (request, response) => {
    const chunks = []; for await (const chunk of request) chunks.push(chunk);
    const payload = JSON.parse(Buffer.concat(chunks).toString());
    const names = (payload.tools || []).map(tool => tool.function?.name || tool.name);
    if (!names.includes('read_workspace_text')) {
      response.setHeader('Content-Type', 'application/json');
      response.end(JSON.stringify({ choices: [{ message: { role: 'assistant', content: '格式调整' } }] })); return;
    }
    tools.push(names);
    const step = round++;
    const name = ['read_workspace_text', 'apply_workspace_patch', 'attempt_completion'][Math.min(step, 2)];
    const args = step === 0 ? { path } : step === 1
      ? { patch: `*** Begin Patch\n*** Update File: ${path}\n@@\n-_正文 123_\n+*正文 123*\n*** End Patch` }
      : { outcome: 'success', result: '格式建议已暂存，原稿未修改。', changed_paths: [] };
    response.setHeader('Content-Type', 'application/json');
    response.end(JSON.stringify({ choices: [{ message: { role: 'assistant', content: '', tool_calls: [{ id: `repair-${step}`, type: 'function', function: { name, arguments: JSON.stringify(args) } }] }, finish_reason: 'tool_calls' }] }));
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  let provider;
  try {
    provider = await api(page, '/api/model-providers', { name: `repair-${Date.now()}`, protocol: 'openai', base_url: `http://127.0.0.1:${server.address().port}/v1`, api_key: 'isolated-mock', model_id: 'grok-repair-fixture', enabled: true, is_default: true });
    await page.reload(); await page.getByTestId(`file-row-${path}`).click();
    await expect(rich(page)).toBeVisible();
    await page.getByTestId('chat-input').fill('尚未发送的聊天草稿');
    await page.getByTestId('preserved-block').getByRole('button', { name: '让助手调整', exact: true }).click();
    expect(round).toBe(0);
    await page.getByRole('button', { name: '提交给助手', exact: true }).click();
    const candidates = () => api(page, `/api/documents/repairs?path=${encodeURIComponent(path)}`);
    await expect.poll(async () => (await candidates()).items.length, { timeout: 20_000 }).toBe(1);
    await expect.poll(() => round).toBe(3);
    expect((await read(page, path)).content).toBe(original);
    await expect(page.getByTestId('chat-input')).toHaveValue('尚未发送的聊天草稿');
    expect(tools.every(names => names.length === 4 && names.every(name => ['read_workspace_text', 'apply_workspace_patch', 'attempt_completion', 'ask_followup_question'].includes(name)))).toBe(true);
    await page.getByRole('button', { name: '版本历史与格式建议', exact: true }).click();
    const dialog = page.getByRole('dialog', { name: '文稿版本与格式建议', exact: true });
    await dialog.getByRole('button', { name: /格式建议 · 第 1 次/ }).click();
    await expect(dialog).toContainText('原稿尚未修改');
    expect((await read(page, path)).content).toBe(original);
    await dialog.getByRole('button', { name: '确认采用此建议', exact: true }).click();
    await expect(dialog).toHaveCount(0);
    await expect.poll(async () => (await read(page, path)).content).toBe(original.replace('_正文 123_', '*正文 123*'));
    expect((await candidates()).items).toHaveLength(0);
  } finally {
    if (provider) await api(page, `/api/model-providers/${provider.id}`, undefined, 'DELETE').catch(() => {});
    server.closeAllConnections(); await new Promise(resolve => server.close(resolve));
  }
});

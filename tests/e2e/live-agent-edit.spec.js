import { expect, test } from '@playwright/test';

// Explicit opt-in: ordinary CI uses local fake providers and incurs no API cost.
const relayURL = process.env.PERSEUS_LIVE_MODEL_BASE_URL;
const models = (process.env.PERSEUS_LIVE_MODEL_IDS || '').split(',').filter(Boolean);

async function api(page, path, body, method = 'POST') {
  return page.evaluate(async ({ path, body, method }) => {
    const response = await fetch(path, { method, credentials: 'include',
      headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
    if (!response.ok) throw new Error(`${response.status}: ${await response.text()}`);
    return response.json();
  }, { path, body, method });
}

for (const model of models.length ? models : ['not configured']) {
  test(`live provider: ${model} edits existing, empty, and new files`, async ({ page }) => {
    test.skip(!relayURL || !models.length, 'Set PERSEUS_LIVE_MODEL_BASE_URL and PERSEUS_LIVE_MODEL_IDS to opt in.');
    test.setTimeout(240_000);
    const folder = `live-edit-${Date.now()}`;
    const draft = `${folder}/draft.md`;
    const empty = `${folder}/empty.md`;
    const output = `${folder}/result.md`;
    let provider;
    let run;
    try {
      await page.goto('/');
      await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
      await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
      await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
      await expect(page.getByTestId('chat-send-button')).toBeVisible();
      provider = await api(page, '/api/model-providers', {
        name: `Live ${model}`, protocol: 'openai', base_url: relayURL,
        api_key: process.env.PERSEUS_LIVE_MODEL_API_KEY || 'local-memory-proxy',
        model_id: model, enabled: true, is_default: true,
      });
      await api(page, '/api/files/mkdir', { path: folder });
      await api(page, '/api/files/create', { path: draft, content: '# Test\nstatus: pending\n' });
      await api(page, '/api/files/create', { path: empty, content: '' });
      await page.reload();
      await page.getByTestId('layout-3').click();
      await page.getByTestId(`toggle-folder-${folder}`).click();
      await page.getByTestId(`file-row-${draft}`).click();
      await page.getByRole('button', { name: '切换为纯文本（当前：富文本）', exact: true }).click();
      await expect(page.getByTestId('editor-textarea')).toHaveValue('# Test\nstatus: pending\n');
      const accepted = page.waitForResponse((response) => response.request().method() === 'POST' && /\/messages$/.test(response.url()));
      await page.getByTestId('chat-input').fill(`请直接在当前会话完成这次小型文件编辑并实际保存：先读取 ${draft} 和 ${empty}，把 ${draft} 中的 status: pending 改为 status: done，保留其他文字；向已有空文件 ${empty} 写入一行 EMPTY_FILE_FIXED；新建 ${output}，内容为一行 LIVE_EDIT_OK。请使用工作区编辑工具，不要只提供建议，不要启动后台任务。完成后核验三个文件。`);
      await page.getByTestId('chat-send-button').click();
      const started = await (await accepted).json();
      run = started.run;
      expect(run?.id).toBeTruthy();
      let finished;
      await expect.poll(async () => {
        finished = await api(page, `/api/agent-runs/${run.id}`, undefined, 'GET');
        return !['queued', 'running'].includes(finished.status);
      }, { timeout: 200_000, intervals: [1000, 2000, 3000] }).toBe(true);
      expect(finished.status, JSON.stringify(finished)).toBe('succeeded');
      const read = (path) => api(page, `/api/files/read?path=${encodeURIComponent(path)}`, undefined, 'GET');
      expect((await read(draft)).content).toBe('# Test\nstatus: done\n');
      expect((await read(empty)).content.trim()).toBe('EMPTY_FILE_FIXED');
      expect((await read(output)).content.trim()).toBe('LIVE_EDIT_OK');
      await expect(page.getByTestId('editor-textarea')).toHaveValue('# Test\nstatus: done\n');
      console.log(JSON.stringify({ model, run_id: run.id, status: finished.status, verified_files: [draft, empty, output] }));
    } finally {
      if (run?.id) await api(page, `/api/agent-runs/${run.id}/cancel`, {}).catch(() => {});
      if (provider) await api(page, `/api/model-providers/${provider.id}`, undefined, 'DELETE').catch(() => {});
      await api(page, '/api/files/delete', { path: folder }).catch(() => {});
    }
  });
}

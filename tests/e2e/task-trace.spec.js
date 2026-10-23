import { createServer } from 'node:http';
import { expect, test } from '@playwright/test';

// Run against an isolated database: these tests install local provider fixtures.
for (const mode of ['success', 'partial', 'cancel']) {
  test(`background ${mode} retains handoff, replies, attempts and terminal events`, async ({ page }) => {
    const marker = `task-trace-${mode}-${Date.now()}`;
    const source = `${marker}-source.md`;
    const report = `${marker}-report.md`;
    let backgroundRequests = 0;
    let taskID;
    const reply = (response, content, name, args) => {
      response.setHeader('Content-Type', 'application/json');
      response.end(JSON.stringify({ choices: [{ message: {
        role: 'assistant', content,
        ...(name ? { tool_calls: [{ id: `${marker}-${backgroundRequests}-${name}`, type: 'function', function: { name, arguments: JSON.stringify(args) } }] } : {}),
      }, finish_reason: name ? 'tool_calls' : 'stop' }] }));
    };
    const mock = createServer(async (request, response) => {
      const chunks = [];
      for await (const chunk of request) chunks.push(chunk);
      const payload = JSON.parse(Buffer.concat(chunks).toString());
      const tools = payload.tools?.map((tool) => tool.function.name) || [];
      if (!tools.length) return reply(response, '后台轨迹验收');
      if (tools.includes('start_background_task')) {
        return reply(response, '', 'start_background_task', {
          title: marker, goal: `只读 ${source}，将核对结果保存到 ${report}。`,
          expected_outputs: [{ type: 'file', path: report, description: '核对报告' }],
          suggested_steps: ['读取', '保存', '回读'],
        });
      }
      if (tools.includes('load_perseus_toolset')) return reply(response, '', 'load_perseus_toolset', { toolset: 'task' });
      backgroundRequests += 1;
      if (mode === 'cancel') return; // Keep the real backend request in flight.
      if (backgroundRequests === 1) return reply(response, '后台第一轮：读取资料。', 'read_workspace_text', { path: source });
      if (mode === 'partial') return reply(response, '', 'attempt_completion', {
        outcome: 'partial', result: '已读取资料；报告部分未完成。', failed_items: ['未生成报告'],
      });
      if (backgroundRequests === 2) return reply(response, '后台第二轮：保存报告。', 'apply_workspace_patch', {
        patch: `*** Begin Patch\n*** Add File: ${report}\n+# 核对报告\n+原稿已核对。\n*** End Patch`,
      });
      if (backgroundRequests === 3) return reply(response, '后台第三轮：回读核验。', 'read_workspace_text', { path: report });
      return reply(response, '', 'attempt_completion', { outcome: 'success', result: '核对报告已保存并回读。', changed_paths: [report] });
    });
    await new Promise((resolve) => mock.listen(0, '127.0.0.1', resolve));
    const api = (path, body, method = body === undefined ? 'GET' : 'POST') => page.evaluate(async ({ path, body, method }) => {
      const response = await fetch(path, { method, headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
      if (!response.ok) throw new Error(`HTTP ${response.status}: ${path}`);
      return response.json();
    }, { path, body, method });
    let provider;
    try {
      await page.goto('/');
      await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
      await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
      await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
      await expect(page.getByTestId('chat-send-button')).toBeVisible();
      provider = await api('/api/model-providers', { name: marker, protocol: 'openai',
        base_url: `http://127.0.0.1:${mock.address().port}/v1`, api_key: 'local-test', model_id: 'mock', enabled: true, is_default: true });
      await api('/api/files/create', { path: source, content: '# 原稿\n\n核对素材。\n' });
      await page.reload();
      await page.getByTestId('conversation-history-button').click();
      await page.getByTestId('conversation-menu-create-button').click();
      const accepted = page.waitForResponse((r) => r.request().method() === 'POST' && /\/api\/conversations\/[^/]+\/messages$/.test(new URL(r.url()).pathname));
      await page.getByTestId('chat-input').fill(`请启动后台核对任务 ${marker}，创建后结束前台。`);
      await page.getByTestId('chat-send-button').click();
      const run = (await (await accepted).json()).run;
      await expect.poll(async () => (await api(`/api/agent-runs/${run.id}`)).status).toBe('succeeded');
      const handoff = (await api(`/api/agent-runs/${run.id}/events`)).items;
      const completed = handoff.filter((e) => e.type === 'run_completed');
      expect(completed).toHaveLength(1);
      expect(completed[0].payload.completion_kind).toBe('background_task_handoff');
      taskID = completed[0].task_id;
      expect(taskID).toBeTruthy();
      if (mode === 'cancel') {
        await expect.poll(() => backgroundRequests).toBe(1);
        await page.locator('button[title="打开任务面板"],button[title$="个任务进行中"]').first().click();
        await page.locator('.task-item-row').filter({ has: page.getByText(marker, { exact: true }) }).click();
        await page.getByRole('button', { name: '取消', exact: true }).click();
      }
      await expect.poll(async () => (await api(`/api/tasks/${taskID}`)).status, { timeout: 15000 }).toBe(mode === 'cancel' ? 'canceled' : 'done');
      if (mode !== 'cancel') await expect(page.getByText('状态：已完成', { exact: true })).toBeVisible();
      const task = await api(`/api/tasks/${taskID}`);
      if (mode !== 'cancel') {
        expect(task.result_json.outcome).toBe(mode);
        expect(task.result_json.ok).toBe(mode === 'success');
        expect(task.result_json.repair_rounds).toBe(0);
      }
      // Reload and page through the durable trace with a deliberately tiny page.
      await page.reload();
      const events = [];
      let cursor = 0;
      for (let i = 0; i < 100; i += 1) {
        const batch = await api(`/api/tasks/${taskID}/events?after_seq=${cursor}&limit=5`);
        events.push(...batch.items);
        if (!batch.items.length) break;
        cursor = batch.next_seq;
      }
      expect(events.map((e) => e.seq)).toEqual(events.map((_, i) => i + 1));
      expect(events.filter((e) => e.type === 'model_call_attempt_started')).toHaveLength(backgroundRequests);
      expect(events.filter((e) => e.type === `task_${mode === 'cancel' ? 'canceled' : 'done'}`)).toHaveLength(1);
      if (mode !== 'cancel') {
        expect(events.some((e) => e.type === 'model_response_observed' && e.payload.content === '后台第一轮：读取资料。')).toBe(true);
        const messages = (await api(`/api/conversations/${run.conversation_id}/messages`)).items;
        const final = messages.find((m) => m.meta?.kind === 'background_task_result' && m.meta.task_id === taskID);
        expect(final.meta.outcome).toBe(mode);
      }
      if (mode === 'success') {
        expect((await api(`/api/files/read?path=${encodeURIComponent(report)}`)).content).toContain('原稿已核对。');
        expect(backgroundRequests).toBe(4);
      }
      if (mode === 'partial') expect(backgroundRequests).toBe(2);
    } finally {
      if (taskID) await api(`/api/tasks/${taskID}/cancel`, {}).catch(() => {});
      if (provider) await api(`/api/model-providers/${provider.id}`, undefined, 'DELETE').catch(() => {});
      mock.closeAllConnections();
      await new Promise((resolve) => mock.close(resolve));
    }
  });
}

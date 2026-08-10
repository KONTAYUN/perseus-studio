import { expect, test } from '@playwright/test';

import { apiGet, apiPost, login } from './helpers.js';

const providerURL = process.env.PERSEUS_E2E_PROVIDER_URL || 'http://provider-fixture:9090/v1';

const variants = [
  {
    id: 'openai-delta',
    protocol: 'openai',
    adapter_mode: 'openai_compatible',
    auth_mode: 'bearer',
    base_url: providerURL,
    tool_argument_stream_mode: 'delta',
  },
  {
    id: 'openai-snapshot',
    protocol: 'openai',
    adapter_mode: 'openai_compatible',
    auth_mode: 'bearer',
    base_url: providerURL.replace(/\/v1$/, '/snapshot/v1'),
    tool_argument_stream_mode: 'snapshot',
  },
  {
    id: 'anthropic-delta',
    protocol: 'anthropic',
    adapter_mode: 'anthropic_native',
    auth_mode: 'x_api_key',
    base_url: providerURL,
    tool_argument_stream_mode: 'delta',
  },
];

async function installProvider(page, variant, testInfo) {
  const suffix = `${variant.id}-${testInfo.workerIndex}-${testInfo.retry}-${Date.now()}-${testInfo.title.replace(/[^a-z]+/gi, '-').toLowerCase()}`;
  await apiPost(page, '/api/model-providers', {
    name: `E2E ${suffix}`,
    visibility: 'public',
    protocol: variant.protocol,
    adapter_mode: variant.adapter_mode,
    auth_mode: variant.auth_mode,
    supports_tools: 'supported',
    supports_parallel_tool_calls: 'supported',
    parallel_capability_source: 'user',
    tool_argument_stream_mode: variant.tool_argument_stream_mode,
    stream_mode_source: 'user_configured',
    supports_vision: 'supported',
    capability_source: 'user',
    base_url: variant.base_url,
    api_key: 'fixture-key',
    model_id: `fixture-${suffix}`,
    enabled: true,
    is_default: true,
    reasoning_capability: 'disabled',
    context_window_tokens: 32768,
  });
  await page.reload();
  await expect(page.getByTestId('chat-input')).toBeVisible();
}

async function runScenario(page, scenario, timeout = 25_000) {
  await page.getByTestId('chat-input').fill(`[fixture:${scenario}] Run the deterministic provider matrix flow.`);
  await page.getByTestId('chat-send-button').click();
  await expect(page.getByText(`E2E ${scenario} completed.`, { exact: true })).toBeVisible({ timeout });
}

for (const variant of variants) {
  test.describe(`provider matrix ${variant.id}`, () => {
    test.beforeEach(async ({ page }, testInfo) => {
      await login(page);
      await installProvider(page, variant, testInfo);
    });

    test('completes a typed tool turn', async ({ page }) => {
      await runScenario(page, 'normal');
    });

    test('executes and pairs multiple independent reads', async ({ page }) => {
      const stamp = Date.now();
      const paths = [`matrix-${stamp}-a.txt`, `matrix-${stamp}-b.txt`];
      await apiPost(page, '/api/files/create', { path: paths[0], content: 'matrix A' });
      await apiPost(page, '/api/files/create', { path: paths[1], content: 'matrix B' });
      try {
        await page.getByTestId('chat-input').fill(`[fixture:multi-read] [paths:${paths.join(',')}] Read both files.`);
        await page.getByTestId('chat-send-button').click();
        await expect(page.getByText('E2E multi-read completed.', { exact: true })).toBeVisible({ timeout: 30_000 });
      } finally {
        await Promise.all(paths.map((path) => apiPost(page, '/api/files/delete', { path }).catch(() => {})));
      }
    });

    test('edits an HTML note atomically', async ({ page }) => {
      const path = `matrix-${variant.id}-${Date.now()}.html`;
      await apiPost(page, '/api/files/create', { path, content: '<p>Before agent edit</p>' });
      try {
        const before = await apiGet(page, `/api/files/history?path=${encodeURIComponent(path)}`);
        await page.reload();
        await page.getByTestId(`file-row-${path}`).click();
        await page.getByTestId('chat-input').fill(`[fixture:file-edit] [path:${path}] Update the note.`);
        await page.getByTestId('chat-send-button').click();
        await expect(page.getByText('E2E file-edit completed.', { exact: true })).toBeVisible({ timeout: 35_000 });
        await expect(page.locator('.rich-text-editor-content')).toContainText('After agent edit');
        await page.waitForTimeout(1_500);
        const after = await apiGet(page, `/api/files/history?path=${encodeURIComponent(path)}`);
        expect((after.items || []).length).toBe((before.items || []).length + 1);
      } finally {
        await apiPost(page, '/api/files/delete', { path }).catch(() => {});
      }
    });

    test('serializes image input without text-base64 fallback', async ({ page }) => {
      await page.getByTestId('chat-attachment-input').setInputFiles({
        name: `matrix-${variant.id}.png`,
        mimeType: 'image/png',
        buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9ZQmcAAAAASUVORK5CYII=', 'base64'),
      });
      await runScenario(page, 'image');
    });

    test('classifies malformed tool arguments as a protocol failure', async ({ page }) => {
      await page.getByTestId('chat-input').fill('[fixture:invalid-arguments] Emit malformed tool arguments.');
      await page.getByTestId('chat-send-button').click();
      await expect(page.getByTestId('chat-run-error').last()).toHaveAttribute('data-failure-class', 'provider_protocol_error');
    });

    test('keeps context overflow distinct', async ({ page }) => {
      await page.getByTestId('chat-input').fill('[fixture:context-overflow] Exceed the provider context window.');
      await page.getByTestId('chat-send-button').click();
      await expect(page.getByTestId('chat-run-error').last()).toHaveAttribute('data-failure-class', 'context_window_exceeded');
    });

    test('automatically resumes after one provider rate limit', async ({ page }) => {
      test.setTimeout(50_000);
      await runScenario(page, 'rate-limit', 45_000);
    });

    test('recovers an interrupted stream without refresh', async ({ page }) => {
      await runScenario(page, 'eof', 35_000);
    });
  });
}

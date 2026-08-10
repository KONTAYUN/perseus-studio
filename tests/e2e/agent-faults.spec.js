import { expect, test } from '@playwright/test';

import { apiGet, apiPost, login } from './helpers.js';

const providerURL = process.env.PERSEUS_E2E_PROVIDER_URL || 'http://provider-fixture:9090/v1';

async function installSlowSnapshotProvider(page, testInfo) {
  const suffix = `${testInfo.workerIndex}-${testInfo.retry}-${Date.now()}`;
  await apiPost(page, '/api/model-providers', {
    name: `E2E fault provider ${suffix}`,
    visibility: 'public',
    protocol: 'openai',
    adapter_mode: 'openai_compatible',
    auth_mode: 'bearer',
    supports_tools: 'supported',
    supports_parallel_tool_calls: 'supported',
    parallel_capability_source: 'user',
    tool_argument_stream_mode: 'snapshot',
    stream_mode_source: 'user_configured',
    supports_vision: 'supported',
    capability_source: 'user',
    base_url: providerURL.replace(/\/v1$/, '/snapshot/v1'),
    api_key: 'fixture-key',
    model_id: `fixture-slow-${suffix}`,
    enabled: true,
    is_default: true,
    reasoning_capability: 'disabled',
    context_window_tokens: 32768,
  });
  await page.reload();
  await expect(page.getByTestId('chat-input')).toBeVisible();
}

test.beforeEach(async ({ page }, testInfo) => {
  await login(page);
  await installSlowSnapshotProvider(page, testInfo);
});

test('preview recovers after a network pause and preserves the reading anchor', async ({ page, context }) => {
  test.setTimeout(60_000);
  const path = `fault-preview-${Date.now()}.html`;
  const beforeTarget = Array.from({ length: 30 }, (_, index) => `<p>Head marker ${index}</p>`).join('');
  const afterTarget = Array.from({ length: 60 }, (_, index) => `<p>Tail marker ${index}</p>`).join('');
  await apiPost(page, '/api/files/create', { path, content: `${beforeTarget}<p>Before agent edit</p>${afterTarget}` });
  try {
    const historyBefore = await apiGet(page, `/api/files/history?path=${encodeURIComponent(path)}`);
    await page.reload();
    await page.getByTestId(`file-row-${path}`).click();
    const anchor = page.getByText('Tail marker 50', { exact: true });
    await anchor.scrollIntoViewIfNeeded();
    const beforeBox = await anchor.boundingBox();
    expect(beforeBox).not.toBeNull();

    await page.getByTestId('chat-input').fill(`[fixture:file-edit-long] [path:${path}] Update the note with a long streamed change.`);
    await page.getByTestId('chat-send-button').click();
    await expect(page.getByTestId('ai-edit-preview-status')).toBeVisible({ timeout: 30_000 });

    await context.setOffline(true);
    await page.waitForTimeout(900);
    await context.setOffline(false);

    await expect(page.getByText('E2E file-edit-long completed.', { exact: true })).toBeVisible({ timeout: 35_000 });
    await expect(page.locator('.rich-text-editor-content')).toContainText('After agent edit streaming preview content');
    await expect.poll(async () => {
      const status = page.getByTestId('ai-edit-preview-status');
      return (await status.count()) === 0 ? 'cleared' : await status.getAttribute('data-preview-status');
    }).toBe('cleared');

    const afterBox = await anchor.boundingBox();
    expect(afterBox).not.toBeNull();
    expect(Math.abs(afterBox.y - beforeBox.y)).toBeLessThan(32);

    await page.waitForTimeout(1_500);
    const historyAfter = await apiGet(page, `/api/files/history?path=${encodeURIComponent(path)}`);
    expect((historyAfter.items || []).length).toBe((historyBefore.items || []).length + 1);
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

test('multiple images survive throttled upload, provider input, reload, and preview', async ({ page, context }) => {
  test.setTimeout(60_000);
  const cdp = await context.newCDPSession(page);
  await cdp.send('Network.enable');
  await cdp.send('Network.emulateNetworkConditions', {
    offline: false,
    latency: 120,
    downloadThroughput: 64 * 1024,
    uploadThroughput: 32 * 1024,
    connectionType: 'cellular3g',
  });
  const imageBuffer = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9ZQmcAAAAASUVORK5CYII=', 'base64');
  const images = Array.from({ length: 4 }, (_, index) => ({
    name: `slow-${index + 1}.png`,
    mimeType: 'image/png',
    buffer: imageBuffer,
  }));
  await page.getByTestId('chat-attachment-input').setInputFiles(images);
  await expect(page.locator('.composer-image-attachment')).toHaveCount(4);
  await page.getByTestId('chat-input').fill('[fixture:image] Inspect all four attached images.');
  await page.getByTestId('chat-send-button').click();
  await expect(page.getByText('E2E image completed.', { exact: true })).toBeVisible({ timeout: 40_000 });
  for (const image of images) await expect(page.getByRole('img', { name: image.name }).first()).toBeVisible();
  const conversations = await apiGet(page, '/api/conversations');
  const conversationID = conversations.items?.[0]?.id;
  expect(conversationID).toBeTruthy();

  await cdp.send('Network.emulateNetworkConditions', {
    offline: false,
    latency: 0,
    downloadThroughput: -1,
    uploadThroughput: -1,
    connectionType: 'none',
  });
  await page.reload();
  await expect(page.getByTestId('chat-input')).toBeVisible();
  await page.getByTestId(`conversation-row-${conversationID}`).click();
  for (const image of images) await expect(page.getByRole('img', { name: image.name }).first()).toBeVisible();
  await page.getByRole('img', { name: images[2].name }).first().click();
  await expect(page.getByTestId('media-preview-close')).toBeVisible();
});

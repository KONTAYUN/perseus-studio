import { expect, test } from '@playwright/test';

import { apiPost, login } from './helpers.js';

const liveConfig = {
  baseURL: process.env.PERSEUS_LIVE_PROVIDER_BASE_URL || '',
  apiKey: process.env.PERSEUS_LIVE_PROVIDER_API_KEY || '',
  modelID: process.env.PERSEUS_LIVE_PROVIDER_MODEL_ID || '',
  protocol: (process.env.PERSEUS_LIVE_PROVIDER_PROTOCOL || 'openai').toLowerCase(),
  streamMode: (process.env.PERSEUS_LIVE_PROVIDER_STREAM_MODE || 'delta').toLowerCase(),
};

test('optional live provider completes one read-only Agent task', async ({ page }) => {
  test.skip(!liveConfig.baseURL || !liveConfig.apiKey || !liveConfig.modelID, 'live provider environment is not configured');
  test.setTimeout(120_000);
  await login(page);
  const anthropic = liveConfig.protocol === 'anthropic';
  await apiPost(page, '/api/model-providers', {
    name: `Live release evaluation ${Date.now()}`,
    visibility: 'public',
    protocol: anthropic ? 'anthropic' : 'openai',
    adapter_mode: anthropic ? 'anthropic_native' : 'openai_compatible',
    auth_mode: anthropic ? 'x_api_key' : 'bearer',
    supports_tools: 'supported',
    supports_parallel_tool_calls: 'unknown',
    tool_argument_stream_mode: anthropic ? 'delta' : liveConfig.streamMode,
    stream_mode_source: 'user_configured',
    supports_vision: 'unknown',
    capability_source: 'user',
    base_url: liveConfig.baseURL,
    api_key: liveConfig.apiKey,
    model_id: liveConfig.modelID,
    enabled: true,
    is_default: true,
    reasoning_capability: 'disabled',
    context_window_tokens: Number(process.env.PERSEUS_LIVE_PROVIDER_CONTEXT_WINDOW || 32768),
  });
  const path = `live-provider-eval-${Date.now()}.txt`;
  const marker = `LIVE_PROVIDER_MARKER_${Date.now()}`;
  await apiPost(page, '/api/files/create', { path, content: marker });
  try {
    await page.reload();
    await page.getByTestId('chat-input').fill(`Read ${path} with the workspace tools and report the exact marker. Do not modify any files.`);
    await page.getByTestId('chat-send-button').click();
    await expect(page.getByText(marker, { exact: false })).toBeVisible({ timeout: 100_000 });
    await expect(page.getByTestId('chat-run-error')).toHaveCount(0);
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

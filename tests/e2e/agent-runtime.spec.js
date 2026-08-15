import { expect, test } from '@playwright/test';

import { apiGet, apiPost, login } from './helpers.js';

async function installFixtureProvider(page, suffix) {
  await apiPost(page, '/api/model-providers', {
    name: `E2E fixture ${suffix}`,
    visibility: 'public',
    protocol: 'openai',
    adapter_mode: 'openai_compatible',
    auth_mode: 'bearer',
    supports_tools: 'supported',
    supports_parallel_tool_calls: 'unsupported',
    tool_argument_stream_mode: 'delta',
    supports_vision: 'supported',
    capability_source: 'user',
    base_url: process.env.PERSEUS_E2E_PROVIDER_URL || 'http://provider-fixture:9090/v1',
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

async function runScenario(page, scenario, timeout = 20_000) {
  await page.getByTestId('chat-input').fill(`[fixture:${scenario}] Run the deterministic Agent flow.`);
  await page.getByTestId('chat-send-button').click();
  await expect(page.getByText(`E2E ${scenario} completed.`)).toBeVisible({ timeout });
}

test.beforeEach(async ({ page }, testInfo) => {
  await login(page);
  await installFixtureProvider(
    page,
    `${testInfo.workerIndex}-${testInfo.retry}-${Date.now()}-${testInfo.title.replace(/[^a-z]+/gi, '-').toLowerCase()}`
  );
});

test('queued Agent run streams commentary and completes through the Docker provider fixture', async ({ page }) => {
  await runScenario(page, 'normal');
  await expect(page.getByText('Preparing the normal deterministic tools.')).toBeVisible();
});

test('exact SSE replay is deduplicated without duplicating commentary', async ({ page }) => {
  await runScenario(page, 'replay');
  await expect(page.getByText('Replay-safe commentary.', { exact: true })).toHaveCount(1);
});

test('invalid tool arguments produce a protocol failure instead of a connection error', async ({ page }) => {
  await page.getByTestId('chat-input').fill('[fixture:invalid-arguments] Emit malformed tool arguments.');
  await page.getByTestId('chat-send-button').click();
  const error = page.getByTestId('chat-run-error').last();
  await expect(error).toBeVisible();
  await expect(error).toHaveAttribute('data-failure-class', 'provider_protocol_error');
  await expect(page.getByText(/连接中断|connection interrupted/i)).toHaveCount(0);
});

test('provider context overflow stays distinct from a connection interruption', async ({ page }) => {
  await page.getByTestId('chat-input').fill('[fixture:context-overflow] Exceed the provider context window.');
  await page.getByTestId('chat-send-button').click();
  const error = page.getByTestId('chat-run-error').last();
  await expect(error).toBeVisible({ timeout: 20_000 });
  await expect(error).toHaveAttribute('data-failure-class', 'context_window_exceeded');
  await expect(page.getByText(/连接中断|connection interrupted/i)).toHaveCount(0);
});

test('provider 429 waits and automatically resumes the same run', async ({ page }) => {
  test.setTimeout(45_000);
  await runScenario(page, 'rate-limit', 40_000);
});

test('interrupted provider stream recovers without user refresh', async ({ page }) => {
  await runScenario(page, 'eof', 30_000);
});

test('Agent note edit reaches the open editor once and creates one history version', async ({ page }) => {
  const path = `e2e-agent-note-${Date.now()}.html`;
  await apiPost(page, '/api/files/create', { path, content: '<p>Before agent edit</p>' });
  try {
    const before = await apiGet(page, `/api/files/history?path=${encodeURIComponent(path)}`);
    await page.reload();
    await expect(page.getByTestId('chat-input')).toBeVisible();
    await page.getByTestId(`file-row-${path}`).click();
    await expect(page.locator('.rich-text-editor-content')).toContainText('Before agent edit');

    await page.getByTestId('chat-input').fill(`[fixture:file-edit] [path:${path}] Update the open note.`);
    await page.getByTestId('chat-send-button').click();
    await expect(page.getByText('E2E file-edit completed.', { exact: true })).toBeVisible({ timeout: 30_000 });
    await expect(page.locator('.rich-text-editor-content')).toContainText('After agent edit');

    await page.waitForTimeout(1_500);
    const after = await apiGet(page, `/api/files/history?path=${encodeURIComponent(path)}`);
    expect((after.items || []).length).toBe((before.items || []).length + 1);
    await expect(page.locator('.rich-text-editor-content')).toContainText('After agent edit');
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

test('streamed Markdown creation opens one preview tab and appears in the workspace without refresh', async ({ page }) => {
  const path = `Seedance-storyboard-${Date.now()}.md`;
  try {
    await page.getByTestId('chat-input').fill(`[fixture:file-create] [path:${path}] Create one Markdown file.`);
    await page.getByTestId('chat-send-button').click();
    await expect(page.getByText('E2E file-create completed.', { exact: true })).toBeVisible({ timeout: 30_000 });

    await expect(page.getByTestId(`file-row-${path}`)).toBeVisible();
    await expect(page.getByTestId(`editor-tab-${path}`)).toHaveCount(1);
    await expect(page.getByTestId('editor-download')).toHaveCount(0);
    await expect(page.getByTestId('markdown-editor')).toContainText('A deterministic streamed Markdown file.');
    const codeWrapper = page.locator('[class*="codeMirrorWrapper"]').first();
    const codeToolbar = page.getByTestId('markdown-code-toolbar');
    await expect(codeToolbar).toHaveCount(0);
    await codeWrapper.locator('.cm-content').click();
    await expect(codeToolbar).toBeVisible();
    await expect(codeToolbar.locator('[role="combobox"]')).toHaveText('Text');

    const visualEditor = page.getByTestId('markdown-editor').locator('[contenteditable="true"]').first();
    await expect(visualEditor).toBeVisible();
    await visualEditor.click();
    await page.keyboard.press('Control+End');
    await page.keyboard.insertText('visual edit');
    await expect(page.getByTestId('markdown-editor')).toContainText('visual edit');

    await page.getByTestId('markdown-mode-source').click();
    await expect(page.getByTestId('editor-textarea')).toContainText('```');
    await page.getByTestId('editor-textarea').fill('# Seedance storyboard\n\n```text\nupdated source\n```');
    await page.getByTestId('markdown-mode-rich-text').click();
    await expect(page.getByTestId('markdown-editor')).toContainText('updated source');
    const updatedCodeWrapper = page.locator('[class*="codeMirrorWrapper"]').first();
    const updatedCodeToolbar = page.getByTestId('markdown-code-toolbar');
    await expect(updatedCodeToolbar).toHaveCount(0);
    await updatedCodeWrapper.locator('.cm-content').click();
    await expect(updatedCodeToolbar).toBeVisible();
    await expect(updatedCodeToolbar.locator('[role="combobox"]')).toHaveText('Text');

    await page.waitForTimeout(1_200);
    const saved = await apiGet(page, `/api/files/read?path=${encodeURIComponent(path)}`);
    expect(saved.content).toContain('updated source');
    await expect(page.getByTestId(`download-${path}`)).toHaveCount(1);
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

test('Markdown tabs keep independent modes and workspace links open sibling files', async ({ page }) => {
  const suffix = Date.now();
  const firstPath = `markdown-mode-${suffix}-a.md`;
  const secondPath = `markdown-mode-${suffix}-b.markdown`;
  await apiPost(page, '/api/files/create', {
    path: firstPath,
    content: `# First document\n\n[Open sibling](./${secondPath})`,
  });
  await apiPost(page, '/api/files/create', {
    path: secondPath,
    content: '# Second document',
  });
  try {
    await page.reload();
    await page.getByTestId(`file-row-${firstPath}`).click();
    await expect(page.getByTestId('markdown-editor')).toContainText('First document');
    await page.getByRole('link', { name: 'Open sibling' }).click();
    await expect(page.getByTestId(`editor-tab-${secondPath}`)).toHaveAttribute('data-active', 'true');
    await expect(page.getByTestId('markdown-editor')).toContainText('Second document');

    await page.getByTestId(`editor-tab-${firstPath}`).click();
    await page.getByTestId('markdown-mode-source').click();
    await expect(page.getByTestId('editor-textarea')).toContainText('Open sibling');
    await page.getByTestId(`editor-tab-${secondPath}`).click();
    await expect(page.getByTestId('markdown-editor')).toContainText('Second document');
    await page.getByTestId(`editor-tab-${firstPath}`).click();
    await expect(page.getByTestId('editor-textarea')).toContainText('Open sibling');
  } finally {
    await apiPost(page, '/api/files/delete', { path: firstPath }).catch(() => {});
    await apiPost(page, '/api/files/delete', { path: secondPath }).catch(() => {});
  }
});

test('open user input resumes the same Agent run without synthetic choices', async ({ page }) => {
  await page.getByTestId('chat-input').fill('[fixture:user-input] Request one external result.');
  await page.getByTestId('chat-send-button').click();
  await expect(page.getByText('Paste the external command output here.', { exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Update in place' })).toHaveCount(0);

  await page.getByTestId('chat-input').fill('external fixture output');
  await page.getByTestId('chat-send-button').click();
  await expect(page.getByText('E2E user-input completed.', { exact: true })).toBeVisible({ timeout: 20_000 });
});

test('bounded user choice submits one option and resumes the same Agent run', async ({ page }) => {
  await page.getByTestId('chat-input').fill('[fixture:user-choice] Request a bounded decision.');
  await page.getByTestId('chat-send-button').click();
  await expect(page.getByText('Choose the bounded update strategy.', { exact: true })).toBeVisible();
  await page.getByRole('button', { name: 'Update in place', exact: true }).click();
  await expect(page.getByText('E2E user-choice completed.', { exact: true })).toBeVisible({ timeout: 20_000 });
});

test('chat image persists across the Agent request, preview dialog, and reload', async ({ page }) => {
  const image = {
    name: 'fixture.png',
    mimeType: 'image/png',
    buffer: Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9ZQmcAAAAASUVORK5CYII=', 'base64'),
  };
  await page.getByTestId('chat-attachment-input').setInputFiles(image);
  await expect(page.locator('.composer-image-attachment')).toContainText('fixture.png');
  await page.getByTestId('chat-input').fill('[fixture:normal] Inspect the attached image.');
  await page.getByTestId('chat-send-button').click();
  await expect(page.getByText('E2E normal completed.', { exact: true })).toBeVisible({ timeout: 20_000 });
  const conversations = await apiGet(page, '/api/conversations');
  const conversationID = conversations.items?.[0]?.id;
  expect(conversationID).toBeTruthy();

  const thumbnail = page.getByRole('img', { name: 'fixture.png' }).first();
  await expect(thumbnail).toBeVisible();
  await thumbnail.click();
  await expect(page.getByTitle('关闭')).toBeVisible();
  await page.keyboard.press('Escape');

  await page.reload();
  await expect(page.getByTestId('chat-input')).toBeVisible();
  await page.getByTestId(`conversation-row-${conversationID}`).click();
  await expect(page.getByRole('img', { name: 'fixture.png' }).first()).toBeVisible();
});

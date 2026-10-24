import { expect, test } from '@playwright/test';

async function api(page, path, body) {
  return page.evaluate(async ({ path, body }) => {
    const response = await fetch(path, body === undefined ? {} : {
      method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body),
    });
    return { status: response.status, data: await response.json() };
  }, { path, body });
}
async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
}
async function open(page, path, content = '原文') {
  expect((await api(page, '/api/files/create', { path, content })).status).toBe(201);
  await page.reload();
  await page.getByTestId('layout-3').click();
  await page.getByTestId(`file-row-${path}`).click();
  await expect(page.getByTestId('editor-textarea')).toHaveValue(content);
}
const read = (page, path) => api(page, `/api/files/read?path=${encodeURIComponent(path)}`);
const convert = page => page.getByTestId('editor-convert-markdown');

test('toolbar and folder new-file actions default to Markdown and avoid name collisions', async ({ page }) => {
  await login(page);
  await page.getByTestId('layout-3').click();
  await page.getByTestId('workspace-add-button').click();
  await page.getByTestId('workspace-create-file').click();
  const input = page.locator('input[data-testid^="inline-rename-"]');
  await expect(input).toHaveValue(/^新建文稿.*\.md$/);
  await input.press('Enter');
  const folder = `new-md-${Date.now()}`;
  await api(page, '/api/files/mkdir', { path: folder });
  await api(page, '/api/files/create', { path: `${folder}/新建文稿.md`, content: '' });
  await page.reload();
  const row = page.getByTestId(`file-row-${folder}`);
  await row.hover();
  await page.getByTestId(`create-file-${folder}`).click();
  await expect(input).toHaveValue(/新建文稿.+\.md$/);
  const newName = await input.inputValue();
  await input.press('Enter');
  expect((await read(page, `${folder}/${newName}`)).status).toBe(200);
});

test('TXT conversion saves unsaved content exactly and reopens in rich view without source controls', async ({ page }) => {
  await login(page);
  const path = `convert-${Date.now()}.TXT`, target = path.replace(/\.txt$/i, '.md');
  await open(page, path);
  const draft = '# 文稿标题\n\n**未保存的内容**\n\n```bash\necho hello\n```\n';
  await page.getByTestId('editor-textarea').fill(draft);
  await expect(page.getByTestId('editor-mode-toggle')).toHaveCount(0);
  await convert(page).click();
  await expect(page.getByTestId('rich-text-editor').getByRole('heading', { name: '文稿标题' })).toBeVisible();
  await expect(convert(page)).toHaveCount(0);
  await expect(page.getByTestId(`file-row-${target}`)).toBeVisible();
  await expect(page.getByTestId(`file-row-${path}`)).toHaveCount(0);
  expect((await read(page, path)).status).toBe(404);
  expect((await read(page, target)).data.content).toBe(draft);
  await expect(page.getByTestId('editor-mode-toggle')).toHaveCount(0);
  await page.reload();
  await page.getByTestId(`file-row-${target}`).click();
  await expect(page.getByTestId('rich-text-editor').locator('strong')).toHaveText('未保存的内容');
  expect((await read(page, target)).data.content).toBe(draft);
});

test('existing Markdown destination blocks conversion and preserves both documents', async ({ page }) => {
  await login(page);
  const path = `collision-${Date.now()}.txt`, target = path.replace('.txt', '.md');
  await api(page, '/api/files/create', { path: target, content: '不能覆盖的 Markdown' });
  await open(page, path, 'TXT 正文');
  await convert(page).click();
  await expect(page.getByTestId('editor-panel')).toContainText('目标文件已存在');
  await expect(convert(page)).toBeEnabled();
  await expect(page.getByTestId('editor-textarea')).toHaveValue('TXT 正文');
  expect((await read(page, target)).data.content).toBe('不能覆盖的 Markdown');
  expect((await read(page, path)).data.content).toBe('TXT 正文');
});

test('failed draft save prevents rename and retry preserves all typing', async ({ page }) => {
  await login(page);
  const path = `save-failure-${Date.now()}.txt`, target = path.replace('.txt', '.md');
  await open(page, path);
  await page.route('**/api/files/write', route => route.fulfill({ status: 500, json: { error: '测试保存失败' } }));
  await page.getByTestId('editor-textarea').fill('未保存的修改');
  await convert(page).click();
  await expect(page.getByTestId('editor-panel')).toContainText('转换为 Markdown 失败');
  await expect(page.getByTestId('editor-textarea')).toHaveValue('未保存的修改');
  expect((await read(page, target)).status).toBe(404);
  await page.unroute('**/api/files/write');
  await convert(page).click();
  await expect(page.getByTestId('rich-text-editor')).toBeVisible();
  expect((await read(page, target)).data.content).toBe('未保存的修改');
});

test('tab navigation waits for conversion; no save is sent to the obsolete TXT path', async ({ page }) => {
  await login(page);
  const other = `other-${Date.now()}.txt`, path = `slow-${Date.now()}.txt`, target = path.replace('.txt', '.md');
  await api(page, '/api/files/create', { path: other, content: '另一个文件' });
  await open(page, path);
  let release, entered;
  const enteredRequest = new Promise(resolve => { entered = resolve; });
  const wait = new Promise(resolve => { release = resolve; });
  await page.route('**/api/files/rename', async route => { entered(); await wait; await route.continue(); });
  await page.getByTestId('editor-textarea').fill('转换中的正文');
  await convert(page).click();
  await enteredRequest;
  await expect(convert(page)).toBeDisabled();
  await expect(page.getByTestId('editor-textarea')).toHaveAttribute('readonly', '');
  await page.getByTestId(`file-row-${other}`).click();
  release();
  await expect(page.getByTestId('editor-textarea')).toHaveValue('另一个文件');
  expect((await read(page, target)).data.content).toBe('转换中的正文');
  expect((await read(page, path)).status).toBe(404);
});

test('external edit between save and rename blocks stale conversion', async ({ page }) => {
  await login(page);
  const path = `external-${Date.now()}.txt`, target = path.replace('.txt', '.md');
  await open(page, path);
  await page.route('**/api/files/rename', async route => {
    await api(page, '/api/files/write', { path, content: '其他任务的新版本' });
    await route.continue();
  });
  await convert(page).click();
  await expect(page.getByTestId('editor-panel')).toContainText('文件已变化');
  await expect(convert(page)).toBeEnabled();
  expect((await read(page, target)).status).toBe(404);
  expect((await read(page, path)).data.content).toBe('其他任务的新版本');
  await expect(page.getByTestId('editor-textarea')).toHaveValue('原文');
});

test('conversion action is absent for other text formats', async ({ page }) => {
  await login(page);
  await open(page, `other-${Date.now()}.csv`, '名称,值\n文稿,1');
  await expect(convert(page)).toHaveCount(0);
  await expect(page.getByTestId('editor-mode-toggle')).toHaveCount(0);
});

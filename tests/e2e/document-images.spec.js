import { expect, test } from '@playwright/test';
import { api, open, read, rich } from './helpers/document.js';

const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+ip1sAAAAASUVORK5CYII=', 'base64');
async function imageDialog(page) {
  await rich(page).locator('p').first().click();
  await page.getByTestId('block-format-handle').click();
  await page.getByRole('menuitem', { name: '图片', exact: true }).click();
  return page.getByRole('dialog', { name: '图片', exact: true });
}

test('image upload previews, saves, reloads and stays out of the workspace tree', async ({ page }) => {
  const path = await open(page, '图片测试正文');
  const dialog = await imageDialog(page);
  await expect(dialog.getByRole('textbox', { name: '图片地址' })).toHaveCount(0);
  await expect(dialog.getByRole('button', { name: '应用', exact: true })).toBeDisabled();
  await dialog.getByLabel('上传图片文件').setInputFiles({ name: '测试插图.png', mimeType: 'image/png', buffer: png });
  await expect(dialog.getByAltText('所选图片预览')).toBeVisible();
  await expect(dialog.getByText('测试插图.png', { exact: true })).toBeVisible();
  await dialog.getByRole('textbox', { name: '图片说明' }).fill('示意图片');
  await dialog.getByRole('button', { name: '应用', exact: true }).click();
  await expect.poll(async () => (await read(page, path)).content).toContain('![示意图片](/.perseus-images/');
  const tree = await api(page, '/api/files/tree');
  expect(JSON.stringify(tree)).not.toContain('.perseus-images');
  await page.reload(); await page.getByTestId(`file-row-${path}`).click();
  const image = rich(page).getByAltText('示意图片');
  await expect(image).toBeVisible();
  await expect.poll(() => image.evaluate(el => el.naturalWidth)).toBe(1);
  await image.dblclick();
  await expect(page.getByRole('dialog', { name: '图片' }).getByAltText('所选图片预览')).toBeVisible();
});

test('workspace image picker offers thumbnails, search, empty feedback and keyboard dismissal', async ({ page }) => {
  await open(page, '选择工作区图片');
  const name = `项目封面-${crypto.randomUUID()}.png`;
  await page.evaluate(async ({ bytes, name }) => {
    const form = new FormData(); form.append('path', `参考图片/${name}`); form.append('file', new Blob([new Uint8Array(bytes)], { type: 'image/png' }), name);
    const response = await fetch('/api/files/upload', { method: 'POST', body: form });
    if (!response.ok) throw new Error(await response.text());
  }, { bytes: [...png], name });
  const dialog = await imageDialog(page);
  await dialog.getByRole('button', { name: '工作区图片', exact: true }).click();
  await dialog.getByRole('textbox', { name: '搜索工作区图片' }).fill(name);
  await dialog.getByRole('button', { name: `选择 ${name}` }).click();
  await expect(dialog.getByAltText('所选图片预览')).toBeVisible();
  await dialog.getByRole('textbox', { name: '搜索工作区图片' }).fill('不存在的图片');
  await expect(dialog.getByText('没有找到匹配的图片')).toBeVisible();
  await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0);
});

test('image picker rejects text and failed uploads without inserting content', async ({ page }) => {
  const path = await open(page, '原始正文'); const dialog = await imageDialog(page);
  await dialog.getByLabel('上传图片文件').setInputFiles({ name: '假图片.txt', mimeType: 'text/plain', buffer: Buffer.from('text') });
  await expect(dialog.getByRole('alert')).toContainText('请选择');
  await dialog.getByLabel('上传图片文件').setInputFiles({ name: '假图片.png', mimeType: 'image/png', buffer: Buffer.from('text') });
  await expect(dialog.getByRole('alert')).toContainText('请选择');
  await expect(dialog.getByRole('button', { name: '应用', exact: true })).toBeDisabled();
  await dialog.getByRole('button', { name: '取消', exact: true }).click();
  expect((await read(page, path)).content).toBe('原始正文');
});

test('image dialog stays usable in both themes and a narrow viewport', async ({ page }) => {
  await open(page, '界面检查');
  for (const theme of ['dark', 'light']) {
    const toggle = page.getByRole('button', { name: theme === 'dark' ? '切换深色模式' : '切换浅色模式' });
    if (await toggle.count()) await toggle.click();
    const dialog = await imageDialog(page);
    await expect(dialog.locator('.document-image-upload')).toHaveCSS('background-color', theme === 'dark' ? 'rgb(32, 32, 34)' : 'rgb(248, 250, 252)');
    await page.screenshot({ path: `_local_artifacts/image-picker-${theme}.png`, animations: 'disabled' });
    await expect(dialog.getByRole('button', { name: '选择图片', exact: true })).toBeVisible();
    await page.keyboard.press('Escape');
  }
  const dialog = await imageDialog(page);
  await page.setViewportSize({ width: 390, height: 760 });
  await expect(dialog.getByRole('button', { name: '取消', exact: true })).toBeInViewport();
  expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
  await page.screenshot({ path: '_local_artifacts/image-picker-narrow.png' });
});

import { expect, test } from '@playwright/test';
import { api, open, read } from './helpers/document.js';

const history = page => page.getByRole('dialog', { name: '文稿版本与格式建议', exact: true });
const trigger = page => page.getByRole('button', { name: '版本历史与格式建议', exact: true });

async function openHistory(page) {
  const path = await open(page, '# 初稿\n\n这是第一版的正文。\n');
  await api(page, '/api/files/write', { path, content: '# 修改稿\n\n这是第二版的正文。\n' });
  await api(page, '/api/files/write', { path, content: '# 当前稿\n\n这是当前的正文。\n' });
  await page.reload(); await page.getByTestId(`file-row-${path}`).click();
  await trigger(page).click();
  await expect(history(page).getByRole('heading', { name: '修改稿', exact: true })).toBeVisible();
  return path;
}

test('history defaults to the newest snapshot, marks selection and preserves the current document until restore', async ({ page }) => {
  const path = await openHistory(page), dialog = history(page);
  const rows = dialog.getByRole('navigation', { name: '文稿版本' }).getByRole('button');
  await expect(rows).toHaveCount(2);
  await expect(rows.first()).toHaveAttribute('aria-pressed', 'true');
  await rows.nth(1).click();
  await expect(dialog.getByRole('heading', { name: '初稿', exact: true })).toBeVisible();
  await expect(rows.nth(1)).toHaveAttribute('aria-pressed', 'true');
  expect((await read(page, path)).content).toContain('# 当前稿');
  await dialog.getByRole('button', { name: '恢复此版本', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect.poll(async () => (await read(page, path)).content).toContain('# 初稿');
});

test('history has a visible top-right close action, closes outside and with Escape, and keeps inside clicks open', async ({ page }) => {
  await openHistory(page); const dialog = history(page);
  const close = dialog.getByRole('button', { name: '关闭历史版本', exact: true });
  const dialogBox = await dialog.boundingBox(), closeBox = await close.boundingBox();
  expect(closeBox.x).toBeGreaterThan(dialogBox.x + dialogBox.width / 2);
  expect(closeBox.y).toBeLessThan(dialogBox.y + 50);
  await dialog.getByRole('heading', { name: '历史版本', exact: true }).click();
  await expect(dialog).toBeVisible();
  // Releasing a selection outside the dialog must not dismiss it.
  await page.mouse.move(dialogBox.x + 50, dialogBox.y + 30); await page.mouse.down();
  await page.mouse.move(5, 5); await page.mouse.up();
  await expect(dialog).toBeVisible();
  await page.mouse.click(5, 5); await expect(dialog).toHaveCount(0);
  await expect(trigger(page)).toBeFocused();
  await trigger(page).click(); await close.click(); await expect(dialog).toHaveCount(0);
  await trigger(page).click(); await page.keyboard.press('Escape'); await expect(dialog).toHaveCount(0);
});

test('history uses readable text and stable geometry in dark, light and narrow layouts', async ({ page }) => {
  await openHistory(page); await page.keyboard.press('Escape');
  for (const theme of ['dark', 'light']) {
    const toggle = page.getByRole('button', { name: theme === 'dark' ? '切换深色模式' : '切换浅色模式' });
    if (await toggle.count()) await toggle.click();
    await trigger(page).click(); const dialog = history(page);
    const color = theme === 'dark' ? 'rgb(216, 222, 233)' : 'rgb(51, 65, 85)';
    await expect(dialog).toHaveCSS('color', color);
    await expect(dialog.locator('.markdown-content p')).toHaveCSS('color', color);
    await page.screenshot({ path: `_local_artifacts/history-dialog-${theme}.png`, animations: 'disabled' });
    await page.keyboard.press('Escape');
  }
  await page.setViewportSize({ width: 390, height: 760 });
  await trigger(page).click(); const dialog = history(page);
  await expect(dialog.getByRole('button', { name: '关闭历史版本' })).toBeInViewport();
  await expect(dialog.getByRole('button', { name: '恢复此版本' })).toBeInViewport();
  expect(await dialog.evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);
  await page.screenshot({ path: '_local_artifacts/history-dialog-narrow.png', animations: 'disabled' });
});

test('empty history has clear feedback and a disabled restore action', async ({ page }) => {
  await open(page, '刚创建的文稿'); await trigger(page).click();
  await expect(history(page).getByText('还没有历史版本', { exact: true })).toBeVisible();
  await expect(history(page).getByRole('button', { name: '恢复此版本', exact: true })).toBeDisabled();
});

test('a pending preview cannot restore the previously selected snapshot', async ({ page }) => {
  await openHistory(page); const dialog = history(page);
  let pending;
  await page.route('**/api/documents/versions?*version_id=*', route => { pending = route; });
  const rows = dialog.getByRole('navigation', { name: '文稿版本' }).getByRole('button');
  await rows.nth(1).click();
  await expect(dialog.getByText('正在读取内容…', { exact: true })).toBeVisible();
  await expect(dialog.getByRole('button', { name: '恢复此版本', exact: true })).toBeDisabled();
  await expect.poll(() => Boolean(pending)).toBe(true); await pending.continue();
  await expect(dialog.getByRole('heading', { name: '初稿', exact: true })).toBeVisible();
  await expect(dialog.getByRole('button', { name: '恢复此版本', exact: true })).toBeEnabled();
});

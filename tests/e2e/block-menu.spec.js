import { expect, test } from '@playwright/test';
import { api, open, rich } from './helpers/document.js';

test('block menu toggles from its handle and stays closed after outside clicks and editing', async ({ page }) => {
  const path = await open(page, '第一段正文\n\n第二段正文');
  const handle = page.getByTestId('block-format-handle');
  const menu = page.getByTestId('block-format-menu');
  const first = rich(page).getByText('第一段正文', { exact: true });
  try {
    await first.click();
    await handle.click();
    await expect(menu).toHaveAttribute('data-state', 'open');
    await handle.click();
    await expect(handle).toHaveAttribute('aria-expanded', 'false');
    await expect(menu).toHaveCount(0);

    await handle.click();
    await page.getByTestId('rich-document-surface').click({ position: { x: 5, y: 5 } });
    await expect(menu).toHaveCount(0);
    await first.click();
    await page.keyboard.press('End');
    await page.keyboard.type('继续编辑');
    await expect(menu).toHaveCount(0);
    await expect(handle).toHaveAttribute('aria-expanded', 'false');

    await handle.click();
    await rich(page).getByText('第二段正文', { exact: true }).click();
    await expect(menu).toHaveCount(0);
    await page.keyboard.type('直接编辑');
    await expect(rich(page)).toContainText('直接编辑');
    await expect(menu).toHaveCount(0);
  } finally { await api(page, '/api/files/delete', { path }); }
});

test('block menu closes on typing, Escape and format selection without losing the paragraph', async ({ page }) => {
  const path = await open(page, '需要格式化的正文');
  const handle = page.getByTestId('block-format-handle');
  const menu = page.getByTestId('block-format-menu');
  try {
    await rich(page).getByText('需要格式化的正文', { exact: true }).click();
    await page.keyboard.press('End');
    await handle.click();
    await page.keyboard.type('新增文字');
    await expect(rich(page)).toContainText('需要格式化的正文新增文字');
    await expect(menu).toHaveCount(0);
    await expect(handle).toHaveAttribute('aria-expanded', 'false');

    await handle.click();
    await page.keyboard.press('Escape');
    await expect(menu).toHaveCount(0);
    await expect(handle).toBeFocused();
    await handle.click();
    await menu.getByRole('menuitem', { name: '四级标题', exact: true }).click();
    await expect(menu).toHaveCount(0);
    await expect(rich(page).locator('h4')).toHaveText('需要格式化的正文新增文字');
    await handle.click();
    await expect(menu.getByRole('menuitem', { name: /^四级标题/ })).toHaveClass(/is-active/);
  } finally { await api(page, '/api/files/delete', { path }); }
});

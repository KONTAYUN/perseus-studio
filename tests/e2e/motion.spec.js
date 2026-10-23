import { expect, test } from '@playwright/test';

async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
}

for (const reducedMotion of ['no-preference', 'reduce']) {
  test(`rapid layout and overlay interaction stays stable (${reducedMotion})`, async ({ page }) => {
    await page.emulateMedia({ reducedMotion });
    const errors = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await login(page);
    await page.getByTestId('layout-1').click();
    await page.waitForTimeout(280);
    const activeAnimations = await page.getByTestId('layout-3').evaluate(async (button) => {
      button.click();
      await new Promise(requestAnimationFrame);
      await new Promise(requestAnimationFrame);
      return [...document.querySelectorAll('.panel-wrapper')].reduce((count, node) => count + node.getAnimations().length, 0);
    });
    if (reducedMotion === 'reduce') expect(activeAnimations).toBe(0);
    else expect(activeAnimations).toBeGreaterThan(0);
    // Dispatch with a short interval to exercise replacement before completion.
    for (const layout of [3, 1, 2, 3, 2, 1, 3]) {
      await page.getByTestId(`layout-${layout}`).click();
      await page.waitForTimeout(35);
    }
    await expect(page.getByTestId('file-tree-panel')).toBeVisible();
    await expect(page.getByTestId('editor-panel')).toBeVisible();
    await page.waitForTimeout(320);
    const panels = await page.locator('.panel-wrapper').evaluateAll((nodes) => nodes.map((node) => ({
      animations: node.getAnimations().length,
      transform: getComputedStyle(node).transform,
      width: node.getBoundingClientRect().width,
    })));
    expect(panels.every((item) => item.animations === 0 && item.transform === 'none' && item.width > 0)).toBe(true);

    for (let i = 0; i < 3; i++) {
      await page.getByTestId('workspace-add-button').click();
      const menu = page.locator('.motion-popover[data-state="open"]');
      await expect(menu).toBeVisible();
      await page.waitForTimeout(210);
      const bounds = await menu.boundingBox();
      expect(bounds.x).toBeGreaterThanOrEqual(0);
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(page.viewportSize().width);
      await page.keyboard.press('Escape');
      await expect(page.locator('.motion-popover')).toHaveCount(0);
    }
    for (let i = 0; i < 3; i++) {
      await page.getByTestId('settings-button').click();
      await expect(page.getByRole('region', { name: '工作台设置' })).toBeVisible();
      await page.getByTestId('settings-return').click();
      await expect(page.getByRole('region', { name: '工作台设置' })).toHaveCount(0);
    }
    if (reducedMotion === 'reduce') {
      const duration = await page.getByTestId('chat-send-button').evaluate((node) => getComputedStyle(node).transitionDuration);
      expect(duration.split(',').every((item) => parseFloat(item) === 0)).toBe(true);
    }
    await page.screenshot({ path: `_local_artifacts/motion-${reducedMotion}.png`, fullPage: true });
    expect(errors).toEqual([]);
  });
}

import { expect, test } from '@playwright/test';

async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
}

async function openSettings(page) {
  await page.getByTestId('settings-button').click();
  await expect(page.getByTestId('model-name-input')).toBeVisible();
}

async function measureSelectPopover(page, labelText) {
  const trigger = page.locator('label').filter({ hasText: labelText }).locator('button');
  await trigger.scrollIntoViewIfNeeded();
  const triggerBox = await trigger.boundingBox();
  await trigger.click();
  const popover = page.locator('body > div.ui-popover-surface').last();
  await expect(popover).toBeVisible();
  const popoverBox = await popover.boundingBox();
  if (!triggerBox || !popoverBox) throw new Error(`missing bounds for ${labelText}`);
  return { triggerBox, popoverBox };
}

test('select popovers stay anchored for reasoning and context length', async ({ page }) => {
  await login(page);
  await openSettings(page);

  const reasoning = await measureSelectPopover(page, '原生思考能力');
  await page.locator('body > div.fixed.inset-0').last().click({ position: { x: 4, y: 4 } });
  await expect(page.locator('body > div.ui-popover-surface').last()).toBeHidden();

  const context = await measureSelectPopover(page, '上下文长度');

  for (const { triggerBox, popoverBox, label } of [
    { ...reasoning, label: '原生思考能力' },
    { ...context, label: '上下文长度' },
  ]) {
    const popoverBottom = popoverBox.y + popoverBox.height;
    const triggerBottom = triggerBox.y + triggerBox.height;
    const aboveGap = triggerBox.y - popoverBottom;
    const belowGap = popoverBox.y - triggerBottom;
    const minGap = Math.min(Math.abs(aboveGap), Math.abs(belowGap));
    expect(minGap, `${label} popover should stay close to its trigger`).toBeLessThan(28);
  }
});

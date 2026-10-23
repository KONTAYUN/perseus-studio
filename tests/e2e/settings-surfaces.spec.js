import { expect, test } from '@playwright/test';

async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByTestId('settings-button')).toBeVisible();
}

async function expectPlainPage(page) {
  const container = page.locator('.settings-page > :last-child');
  await expect(container).toBeVisible();
  await expect(container).toHaveCSS('border-top-width', '0px');
  await expect(container).toHaveCSS('border-right-width', '0px');
  await expect(container).toHaveCSS('border-bottom-width', '0px');
  await expect(container).toHaveCSS('border-left-width', '0px');
  await expect(container).toHaveCSS('box-shadow', 'none');
  await expect(container).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)');
}

for (const theme of ['light', 'dark']) {
  test(`settings page surfaces stay flat in ${theme} mode while inner cards and keyboard focus remain visible`, async ({ page }) => {
    await login(page);
    if (theme === 'dark') await page.getByRole('button', { name: '切换深色模式' }).click();
    await page.getByTestId('settings-button').click();
    for (const width of [1440, 390]) {
      await page.setViewportSize({ width, height: 900 });
      for (const [id, title] of [['models', '模型与连接'], ['search', '联网搜索'], ['skills', '工具扩展'], ['users', '用户与访问'], ['usage', '用量与费用'], ['diagnostics', '诊断日志']]) {
        if (width <= 760) await page.getByRole('combobox', { name: '设置分类', exact: true }).selectOption(id);
        else await page.getByRole('navigation', { name: '设置分类' }).getByText(title, { exact: true }).click();
        await expect(page.getByRole('heading', { name: title, exact: true })).toBeVisible();
        if (id === 'users') await expect(page.getByRole('heading', { name: '系统用户', exact: true })).toBeVisible();
        await expectPlainPage(page);
        expect(await page.getByTestId('settings-workspace').evaluate(root => root.scrollWidth <= root.clientWidth)).toBe(true);
        if (id === 'skills' || id === 'users') {
          const card = page.locator('.settings-page .settings-card').first();
          await expect(card).toBeVisible();
          await expect(card).toHaveCSS('border-top-width', '1px');
          expect(await card.evaluate(node => parseFloat(getComputedStyle(node).borderTopLeftRadius))).toBeGreaterThan(0);
          await page.screenshot({ path: `_local_artifacts/settings-surfaces/${id}-${theme}-${width}.png` });
        }
      }
    }
    await page.getByTestId('settings-return').focus();
    await page.keyboard.press('Tab');
    await page.keyboard.press('Shift+Tab');
    await expect(page.getByTestId('settings-return')).toBeFocused();
    await expect(page.getByTestId('settings-return')).toHaveCSS('outline-style', 'solid');
    await expect(page.getByTestId('settings-return')).toHaveCSS('outline-width', '2px');
  });
}

test('user loading and creation keep the same flat page container', async ({ page }) => {
  await login(page);
  let release;
  const gate = new Promise(resolve => { release = resolve; });
  await page.route('**/api/admin/users', async route => { await gate; await route.continue(); });
  await page.getByRole('button', { name: '切换深色模式' }).click();
  await page.getByTestId('settings-button').click();
  await page.getByTestId('settings-nav-users').click();
  try {
    await expect(page.getByText('加载中...', { exact: true })).toBeVisible();
    await expectPlainPage(page);
  } finally { release(); }
  await page.getByRole('button', { name: '创建用户', exact: true }).click();
  await expect(page.getByPlaceholder('username', { exact: true })).toBeVisible();
  await expectPlainPage(page);
});

test('populated Skills and its detail view have no extra outer frame', async ({ page }) => {
  const skill = { id: 'surface-fixture', name: '排版检查', enabled: true, visibility: 'private', description: '用于检查扩展列表和详情的视觉层级' };
  await page.route('**/api/skills', route => route.fulfill({ json: { items: [skill] } }));
  await page.route('**/api/skills/surface-fixture', route => route.fulfill({ json: { skill, tree: ['SKILL.md'] } }));
  await page.route('**/api/skills/surface-fixture/read', route => route.fulfill({ json: { content: '# 排版检查\n\n检查文稿格式。' } }));
  await login(page);
  await page.getByRole('button', { name: '切换深色模式' }).click();
  await page.getByTestId('settings-button').click();
  await page.getByTestId('settings-nav-skills').click();
  await expectPlainPage(page);
  await expect(page.locator('.settings-card').filter({ hasText: skill.description })).toHaveCSS('border-top-width', '1px');
  await page.getByRole('button', { name: /排版检查.*启用.*私密/ }).click();
  await expect(page.getByText('目录结构', { exact: true })).toBeVisible();
  await expect(page.locator('pre')).toContainText('检查文稿格式。');
  await expectPlainPage(page);
  await page.screenshot({ path: '_local_artifacts/settings-surfaces/skill-detail-dark.png' });
});

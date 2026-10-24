import { expect, test } from '@playwright/test';

async function api(page, path, body, method = 'POST') {
  return page.evaluate(async ({path,body,method}) => {
    const response = await fetch(path, {method,headers:{'Content-Type':'application/json'},body:body === undefined ? undefined : JSON.stringify(body)});
    if (!response.ok) throw new Error(`API status ${response.status}`);
    return response.json();
  }, {path,body,method});
}
async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button',{name:/进入|创建并进入|登录/}).click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
}
async function openDocument(page, path, content) {
  await api(page,'/api/files/create',{path,content});
  await page.reload();
  await page.getByTestId('layout-3').click();
  await page.getByTestId(`file-row-${path}`).click();
  await expect(page.getByTestId('rich-text-editor')).toBeVisible();
}
const read = (page,path) => api(page,`/api/files/read?path=${encodeURIComponent(path)}`,undefined,'GET');

test('rich-only controls omit downloads and source switching; scrollbars include portalled dark menus', async ({page, browserName}) => {
  const path=`polish-scroll-${Date.now()}.md`;
  await login(page);
  try {
    await openDocument(page,path,'# 检查菜单\n\n普通段落\n\n'+Array(50).fill('这是用来检查滚动条的正文。').join('\n\n'));
    await expect(page.getByTestId('editor-mode-toggle')).toHaveCount(0);
    await expect(page.getByTestId('editor-download')).toHaveCount(0);
    await expect(page.getByRole('group',{name:'文档显示格式'})).toHaveCount(0);
    const surface = page.getByTestId('rich-document-surface');
    await expect(surface).toHaveClass(/custom-scrollbar/);
    await expect.poll(() => surface.evaluate(element => element.scrollHeight > element.clientHeight)).toBe(true);
    // Firefox headless reports its overlay scrollbar as `none` even when the
    // element is scrollable; Chromium exposes the authored `thin` value.
    if (browserName === 'chromium') await expect(surface).toHaveCSS('scrollbar-width','thin');
    await page.getByTestId('rich-text-editor').getByText('普通段落',{exact:true}).click();
    await page.getByTestId('block-format-handle').click();
    const menu=page.getByTestId('block-format-menu');
    await expect(menu).toHaveClass(/custom-scrollbar/);
    if (browserName !== 'firefox') expect(await menu.evaluate(el=>getComputedStyle(el,'::-webkit-scrollbar-button').display)).toBe('none');
    await menu.getByRole('menuitem',{name:'删除当前段落',exact:true}).scrollIntoViewIfNeeded();
    await expect(menu.getByRole('menuitem',{name:'删除当前段落',exact:true})).toBeInViewport();
    await page.keyboard.press('Escape');
    await page.getByRole('button',{name:'切换深色模式',exact:true}).click();
    await expect(page.locator('.app-shell')).toHaveClass(/dark/);
    await page.getByTestId('rich-text-editor').getByText('普通段落',{exact:true}).click();
    await page.getByTestId('block-format-handle').click();
    await expect(menu).toHaveCSS('opacity','1');
    if (await page.evaluate(() => CSS.supports('scrollbar-color', 'rgb(148 163 184 / 0.4) transparent'))) {
      await expect(menu).toHaveCSS('scrollbar-color','rgba(148, 163, 184, 0.4) rgba(0, 0, 0, 0)');
    }
    await menu.getByRole('menuitem',{name:'删除当前段落',exact:true}).scrollIntoViewIfNeeded();
    await expect(menu.getByRole('menuitem',{name:'删除当前段落',exact:true})).toBeInViewport({ratio:1});
    await menu.hover();
    await page.screenshot({path:'_local_artifacts/editor-polish-menu-dark.png',fullPage:true});
  } finally {await api(page,'/api/files/delete',{path}).catch(()=>{});}
});

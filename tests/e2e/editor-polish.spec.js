import { createServer } from 'node:http';
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
  await expect(page.getByTestId('editor-mode-toggle')).toBeVisible();
}
const read = (page,path) => api(page,`/api/files/read?path=${encodeURIComponent(path)}`,undefined,'GET');

test('one mode icon replaces downloads and segmented buttons; scrollbars include portalled dark menus', async ({page}) => {
  const path=`polish-scroll-${Date.now()}.md`;
  await login(page);
  try {
    await openDocument(page,path,'# 检查菜单\n\n普通段落\n\n'+Array(50).fill('这是用来检查滚动条的正文。').join('\n\n'));
    await expect(page.getByTestId('editor-mode-toggle')).toHaveCount(1);
    await expect(page.getByTestId('editor-download')).toHaveCount(0);
    await expect(page.getByRole('group',{name:'文档显示格式'})).toHaveCount(0);
    await expect(page.getByTestId('editor-mode-toggle')).toHaveAttribute('title', /切换为纯文本/);
    await expect(page.getByTestId('rich-document-surface')).toHaveCSS('scrollbar-width','thin');
    await page.getByTestId('editor-mode-toggle').click();
    await expect(page.getByTestId('editor-textarea')).toBeVisible();
    await expect(page.getByTestId('editor-textarea')).toHaveCSS('scrollbar-width','thin');
    await page.getByTestId('editor-mode-toggle').click();
    await page.getByTestId('rich-text-editor').getByText('普通段落',{exact:true}).click();
    await page.getByTestId('block-format-handle').click();
    const menu=page.getByTestId('block-format-menu');
    await expect(menu).toHaveCSS('scrollbar-width','thin');
    expect(await menu.evaluate(el=>getComputedStyle(el,'::-webkit-scrollbar-button').display)).toBe('none');
    await menu.getByRole('menuitem',{name:'删除当前段落',exact:true}).scrollIntoViewIfNeeded();
    await expect(menu.getByRole('menuitem',{name:'删除当前段落',exact:true})).toBeInViewport();
    await page.keyboard.press('Escape');
    await page.getByRole('button',{name:'切换深色模式',exact:true}).click();
    await expect(page.locator('.app-shell')).toHaveClass(/dark/);
    await page.getByTestId('rich-text-editor').getByText('普通段落',{exact:true}).click();
    await page.getByTestId('block-format-handle').click();
    await expect(menu).toHaveCSS('opacity','1');
    await expect(menu).toHaveCSS('scrollbar-color','rgba(148, 163, 184, 0.4) rgba(0, 0, 0, 0)');
    await menu.getByRole('menuitem',{name:'删除当前段落',exact:true}).scrollIntoViewIfNeeded();
    await expect(menu.getByRole('menuitem',{name:'删除当前段落',exact:true})).toBeInViewport({ratio:1});
    await menu.hover();
    await page.screenshot({path:'_local_artifacts/editor-polish-menu-dark.png',fullPage:true});
  } finally {await api(page,'/api/files/delete',{path}).catch(()=>{});}
});

test('format repair requires confirmation, saves the source draft, calls current Agent and preserves composer input', async ({page}) => {
  const path=`polish-repair-${Date.now()}.md`;
  const source='# 原标题\n\n**重要内容\n\n```bash\necho keep\n```\n';
  let round=0;
  let submitted=0;
  const mock=createServer(async(request,response)=>{
    const chunks=[];for await(const chunk of request)chunks.push(chunk);
    const payload=JSON.parse(Buffer.concat(chunks).toString());
    const names=(payload.tools||[]).map(tool=>tool.name||tool.function?.name);
    if(!names.includes('read_workspace_text')){
      response.setHeader('Content-Type','application/json');
      response.end(JSON.stringify({choices:[{message:{role:'assistant',content:'格式修复'}}]}));return;
    }
    const step=round++;
    const name=['read_workspace_text','apply_workspace_patch','attempt_completion'][Math.min(step,2)];
    const args=step===0?{path}:step===1?{patch:`*** Begin Patch\n*** Update File: ${path}\n@@\n-**重要内容\n+**重要内容**\n*** End Patch`}:{result:`已修复 ${path} 的强调格式。`,changed_paths:[path]};
    response.setHeader('Content-Type','text/event-stream');
    response.write(`data: ${JSON.stringify({choices:[{index:0,delta:{tool_calls:[{index:0,id:`format_${step}`,type:'function',function:{name,arguments:JSON.stringify(args)}}]},finish_reason:null}]})}\n\n`);
    response.end(`data: ${JSON.stringify({choices:[{index:0,delta:{},finish_reason:'tool_calls'}]})}\n\ndata: [DONE]\n\n`);
  });
  await new Promise(resolve=>mock.listen(0,'127.0.0.1',resolve));
  let provider;
  await login(page);
  const errors=[];page.on('pageerror',e=>errors.push(e.message));
  page.on('request',r=>{if(r.method()==='POST'&&/\/conversations\/[^/]+\/messages$/.test(new URL(r.url()).pathname))submitted++;});
  try {
    provider=await api(page,'/api/model-providers',{name:'Format repair test',protocol:'openai',base_url:`http://127.0.0.1:${mock.address().port}/v1`,api_key:'local-test',model_id:'grok-test-model',enabled:true,is_default:true});
    await openDocument(page,path,source);
    await page.getByTestId('editor-mode-toggle').click();
    await page.getByTestId('editor-textarea').fill(source+'继续保留正文\n');
    await page.getByTestId('chat-input').fill('这是一条尚未发送的草稿');
    await expect(page.getByTestId('markdown-format-warning')).toBeVisible();
    expect(submitted).toBe(0);
    await page.getByTestId('markdown-format-warning').click();
    await expect(page.getByRole('dialog',{name:'Markdown 格式检查'})).toContainText('可能是合法写法');
    await page.getByRole('button',{name:'暂不修复',exact:true}).click();
    expect(submitted).toBe(0);
    await page.getByTestId('markdown-format-warning').click();
    await expect(page.getByTestId('markdown-format-popover')).toHaveCSS('opacity','1');
    await page.screenshot({path:'_local_artifacts/editor-polish-warning-light.png',fullPage:true});
    await page.getByRole('button',{name:'让 Agent 修复',exact:true}).click();
    await expect.poll(async()=>(await read(page,path)).content).toBe(source.replace('**重要内容','**重要内容**')+'继续保留正文\n');
    await expect(page.getByTestId('markdown-format-warning')).toHaveCount(0);
    await expect(page.getByTestId('chat-input')).toHaveValue('这是一条尚未发送的草稿');
    await expect(page.locator('.message-bubble-user').last()).toContainText(path);
    await expect(page.locator('.message-bubble-user').last()).toContainText('只做必要的 Markdown 格式修复');
    expect(submitted).toBe(1);expect(round).toBeGreaterThanOrEqual(2);expect(errors).toEqual([]);
  } finally {
    await api(page,'/api/files/delete',{path}).catch(()=>{});
    if(provider)await api(page,`/api/model-providers/${provider.id}`,undefined,'DELETE').catch(()=>{});
    mock.closeAllConnections();await new Promise(resolve=>mock.close(resolve));
  }
});

test('no model and a failed source save cannot submit a repair; valid edits clear the warning', async ({page}) => {
  const path=`polish-failure-${Date.now()}.md`;
  let submitted=0;
  let provider;
  await login(page);
  page.on('request',r=>{if(r.method()==='POST'&&/\/conversations\/[^/]+\/messages$/.test(new URL(r.url()).pathname))submitted++;});
  try {
    // This boundary must stay independent of configurations left by other tests.
    await page.route('**/api/model-providers', route => route.request().method() === 'GET'
      ? route.fulfill({ json: { items: [] } }) : route.continue());
    await openDocument(page,path,'**缺少结束标记\n');
    await page.getByTestId('markdown-format-warning').click();
    await expect(page.getByRole('button',{name:'让 Agent 修复',exact:true})).toBeDisabled();
    await expect(page.getByTestId('markdown-format-popover')).toContainText('请先在聊天区选择可用模型');
    await page.keyboard.press('Escape');
    await page.unroute('**/api/model-providers');
    provider=await api(page,'/api/model-providers',{name:'Unused format model',protocol:'openai',base_url:'http://127.0.0.1:9/v1',api_key:'local-test',model_id:'grok-test-model',enabled:true,is_default:true});
    await page.reload();await page.getByTestId('layout-3').click();await page.getByTestId(`file-row-${path}`).click();
    await page.getByTestId('editor-mode-toggle').click();
    await page.getByTestId('editor-textarea').fill('**缺少结束标记，还没保存\n');
    await page.route('**/api/files/write',route=>route.fulfill({status:503,json:{error:'模拟保存失败'}}));
    await page.getByTestId('markdown-format-warning').click();
    await page.getByRole('button',{name:'让 Agent 修复',exact:true}).click();
    await expect(page.getByTestId('markdown-format-popover').getByRole('alert')).toContainText('模拟保存失败');
    expect(submitted).toBe(0);expect((await read(page,path)).content).toBe('**缺少结束标记\n');
    await page.keyboard.press('Escape');
    await page.unroute('**/api/files/write');
    await page.getByTestId('editor-textarea').fill('**格式已修好**\n');
    await expect(page.getByTestId('markdown-format-warning')).toHaveCount(0);
    await expect.poll(async()=>(await read(page,path)).content).toBe('**格式已修好**\n');
    expect(submitted).toBe(0);
    await api(page,'/api/files/write',{path,content:'磁盘上的新版本\n'});
    await page.getByTestId('editor-textarea').fill('**本地新增但有冲突\n');
    await expect(page.getByRole('button',{name:'保留本地修改',exact:true})).toBeVisible();
    await page.getByTestId('markdown-format-warning').click();
    await expect(page.getByRole('button',{name:'让 Agent 修复',exact:true})).toBeDisabled();
    await expect(page.getByTestId('markdown-format-popover')).toContainText('请先处理文件保存冲突');
    expect((await read(page,path)).content).toBe('磁盘上的新版本\n');expect(submitted).toBe(0);
  } finally {
    await api(page,'/api/files/delete',{path}).catch(()=>{});
    if(provider)await api(page,`/api/model-providers/${provider.id}`,undefined,'DELETE').catch(()=>{});
  }
});

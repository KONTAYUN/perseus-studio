import { expect, test } from '@playwright/test';

async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|创建并进入|登录/ }).click();
  await expect(page.getByText('Perseus Studio').first()).toBeVisible();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
}

async function apiPost(page, path, body) {
  return page.evaluate(
    async ({ path: requestPath, body: requestBody }) => {
      const response = await fetch(requestPath, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(requestBody),
      });
      const text = await response.text();
      if (!response.ok) {
        throw new Error(`${response.status} ${text}`);
      }
      return text ? JSON.parse(text) : null;
    },
    { path, body }
  );
}

async function apiGet(page, path) {
  return page.evaluate(async (requestPath) => {
    const response = await fetch(requestPath, {
      credentials: 'include',
    });
    const text = await response.text();
    if (!response.ok) {
      throw new Error(`${response.status} ${text}`);
    }
    return text ? JSON.parse(text) : null;
  }, path);
}

test('health endpoint is available', async ({ request }) => {
  const response = await request.get('/api/health');
  expect(response.ok()).toBeTruthy();
  expect(await response.text()).toContain('"ok":true');
});

test('main workspace smoke flow works', async ({ page }) => {
  const errors = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text());
  });

  await login(page);

  await page.getByTestId('layout-1').click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();

  await page.getByTestId('layout-2').click();
  await expect(page.getByTestId('editor-panel')).toBeVisible();
  await expect(page.getByTestId('chat-panel')).toBeVisible();

  await page.getByTestId('layout-3').click();
  await expect(page.getByTestId('file-tree-panel')).toBeVisible();
  await expect(page.getByTestId('editor-panel')).toBeVisible();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();

  await page.getByTestId('add-panel-button').click();
  const editorOption = page.getByTestId('add-panel-editor');
  if (await editorOption.isVisible().catch(() => false)) {
    await editorOption.click();
  }
  await expect(page.getByTestId('editor-panel').first()).toBeVisible();

  await page.getByTestId('settings-button').click();
  await expect(page.getByText('模型供应商')).toBeVisible();
  await page.getByTestId('open-diagnostics-settings').scrollIntoViewIfNeeded();
  await page.getByTestId('open-diagnostics-settings').click();
  await expect(page.getByTestId('diagnostics-view')).toBeVisible();

  expect(errors.filter((item) => !/favicon/i.test(item))).toEqual([]);
});

test('workspace file can be created through API and opened from file tree', async ({ page }) => {
  const stamp = Date.now();
  const folderPath = `e2e-${stamp}`;
  const filePath = `${folderPath}/open-me.txt`;
  const fileContent = `hello from e2e ${stamp}`;

  await login(page);

  try {
    await apiPost(page, '/api/files/mkdir', { path: folderPath });
    await apiPost(page, '/api/files/create', { path: filePath, content: fileContent });

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();

    const row = page.getByTestId(`file-row-${filePath}`);
    await expect(row).toBeVisible();
    await row.click();

    await expect(page.getByTestId(`editor-tab-${filePath}`)).toBeVisible();
    await expect(page.getByTestId('editor-textarea')).toHaveValue(fileContent);

    await page.getByTestId('settings-button').click();
    await expect(page.getByTestId('model-name-input')).toBeVisible();
    await expect(page.getByTestId('search-provider-select')).toBeVisible();
  } finally {
    await apiPost(page, '/api/files/delete', { path: folderPath }).catch(() => {});
  }
});

test('trash panel shows deleted items and cleanup controls', async ({ page }) => {
  const stamp = Date.now();
  const folderPath = `trash-e2e-${stamp}`;
  const fileName = `restore-${stamp}.txt`;
  const filePath = `${folderPath}/${fileName}`;
  let trashPath = '';

  await login(page);

  try {
    await apiPost(page, '/api/files/mkdir', { path: folderPath });
    await apiPost(page, '/api/files/create', { path: filePath, content: 'from trash' });
    await apiPost(page, '/api/files/delete', { path: filePath });
    const trashBeforeOpen = await apiGet(page, '/api/files/trash');
    const targetTrashItem = (trashBeforeOpen.items || []).find(
      (item) => item.original_path === filePath || item.name === fileName
    );
    expect(targetTrashItem).toBeTruthy();
    trashPath = targetTrashItem.path;

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();

    await expect(page.getByTestId(`file-row-${trashPath}`)).toHaveCount(0);
    await page.getByTestId('workspace-trash-button').click();
    await expect(page.getByTestId('trash-popover')).toBeVisible();
    const popoverBox = await page.getByTestId('trash-popover').boundingBox();
    const viewport = page.viewportSize();
    if (!popoverBox || !viewport) throw new Error('missing popover bounds');
    expect(popoverBox.x).toBeGreaterThanOrEqual(0);
    expect(popoverBox.y).toBeGreaterThanOrEqual(0);
    expect(popoverBox.x + popoverBox.width).toBeLessThanOrEqual(viewport.width + 1);
    expect(popoverBox.y + popoverBox.height).toBeLessThanOrEqual(viewport.height + 1);
    await expect(page.getByTestId('empty-trash-from-popover')).toBeVisible();
    const trashItem = page.getByTestId('trash-popover').getByText(targetTrashItem.name, { exact: true });
    await expect(trashItem).toBeVisible();

    await trashItem.hover();
    await expect(page.getByTestId(`restore-trash-${trashPath}`)).toBeVisible();
    await expect(page.getByTestId(`delete-trash-${trashPath}`)).toBeVisible();
  } finally {
    if (trashPath) {
      await apiPost(page, '/api/files/trash/delete', { path: trashPath }).catch(() => {});
    }
    await apiPost(page, '/api/files/delete', { path: folderPath }).catch(() => {});
    const trash = await apiGet(page, '/api/files/trash').catch(() => ({ items: [] }));
    for (const item of trash.items || []) {
      if (
        item.original_path === folderPath ||
        item.original_path?.startsWith(`${folderPath}/`) ||
        item.name === fileName ||
        (trashPath && item.path === trashPath)
      ) {
        await apiPost(page, '/api/files/trash/delete', { path: item.path }).catch(() => {});
      }
    }
  }
});

test('row delete action respects multi-selection', async ({ page }) => {
  const stamp = Date.now();
  const folderPath = `multi-delete-e2e-${stamp}`;
  const fileA = `${folderPath}/a.txt`;
  const fileB = `${folderPath}/b.txt`;
  const fileC = `${folderPath}/c.txt`;

  await login(page);

  try {
    await apiPost(page, '/api/files/mkdir', { path: folderPath });
    await apiPost(page, '/api/files/create', { path: fileA, content: 'a' });
    await apiPost(page, '/api/files/create', { path: fileB, content: 'b' });
    await apiPost(page, '/api/files/create', { path: fileC, content: 'c' });

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();

    await page.getByTestId(`file-row-${fileA}`).click({ modifiers: ['Control'] });
    await page.getByTestId(`file-row-${fileB}`).click({ modifiers: ['Control'] });
    await page.getByTestId(`file-row-${fileB}`).hover();
    await page.getByTestId(`delete-${fileB}`).click();
    await expect(page.getByText('确定要将选中的 2 项移入回收站吗？')).toBeVisible();
    await page.getByRole('button', { name: '移入回收站' }).click();

    await expect(page.getByTestId(`file-row-${fileA}`)).toHaveCount(0);
    await expect(page.getByTestId(`file-row-${fileB}`)).toHaveCount(0);
    await expect(page.getByTestId(`file-row-${fileC}`)).toBeVisible();

    const trash = await apiGet(page, '/api/files/trash');
    const originals = new Set((trash.items || []).map((item) => item.original_path));
    expect(originals.has(fileA)).toBeTruthy();
    expect(originals.has(fileB)).toBeTruthy();
  } finally {
    await apiPost(page, '/api/files/delete', { path: folderPath }).catch(() => {});
    const trash = await apiGet(page, '/api/files/trash').catch(() => ({ items: [] }));
    for (const item of trash.items || []) {
      if (item.original_path === folderPath || item.original_path?.startsWith(`${folderPath}/`)) {
        await apiPost(page, '/api/files/trash/delete', { path: item.path }).catch(() => {});
      }
    }
  }
});

test('file tree supports manual drag ordering and self-drop hint', async ({ page }) => {
  const stamp = Date.now();
  const folderPath = `sort-e2e-${stamp}`;
  const folderA = `${folderPath}/folder-a`;
  const folderChild = `${folderA}/child`;
  const fileA = `${folderPath}/a.txt`;
  const fileB = `${folderPath}/b.txt`;
  const fileC = `${folderPath}/c.txt`;

  await login(page);

  try {
    await apiPost(page, '/api/files/mkdir', { path: folderPath });
    await apiPost(page, '/api/files/mkdir', { path: folderA });
    await apiPost(page, '/api/files/mkdir', { path: folderChild });
    await apiPost(page, '/api/files/create', { path: fileA, content: 'a' });
    await apiPost(page, '/api/files/create', { path: fileB, content: 'b' });
    await apiPost(page, '/api/files/create', { path: fileC, content: 'c' });

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();

    const rowB = page.getByTestId(`file-row-${fileB}`);
    const rowC = page.getByTestId(`file-row-${fileC}`);
    await expect(rowB).toBeVisible();
    await expect(rowC).toBeVisible();
    await rowC.dragTo(rowB, { targetPosition: { x: 24, y: 2 } });
    await expect(page.getByText('已调整顺序')).toBeVisible();

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();
    const orderedPaths = await page
      .locator('[data-file-path]')
      .evaluateAll((elements, prefix) => elements.map((element) => element.dataset.filePath).filter((path) => path?.startsWith(`${prefix}/`)), folderPath);
    expect(orderedPaths.filter((path) => path.endsWith('.txt')).slice(0, 3)).toEqual([fileA, fileC, fileB]);

    const folderRow = page.getByTestId(`file-row-${folderA}`);
    const childFolderRow = page.getByTestId(`file-row-${folderChild}`);
    await expect(childFolderRow).toBeVisible();
    await folderRow.dragTo(childFolderRow, { targetPosition: { x: 36, y: 12 } });
    await expect(page.getByText('不能移动到自身文件夹内部')).toBeVisible();
  } finally {
    await apiPost(page, '/api/files/delete', { path: folderPath }).catch(() => {});
    const trash = await apiGet(page, '/api/files/trash').catch(() => ({ items: [] }));
    for (const item of trash.items || []) {
      if (item.original_path === folderPath || item.original_path?.startsWith(`${folderPath}/`)) {
        await apiPost(page, '/api/files/trash/delete', { path: item.path }).catch(() => {});
      }
    }
  }
});

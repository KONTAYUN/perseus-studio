import { expect, test } from '@playwright/test';

import { login } from './helpers.js';

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

async function setStoredOpenFolders(page, paths) {
  await page.evaluate(async (nextPaths) => {
    const response = await fetch('/api/auth/me', {
      credentials: 'include',
    });
    const data = await response.json();
    const userId = data?.user?.id || data?.user?.user_id;
    if (!userId) throw new Error('missing user id for file tree state');
    const key = `perseus:file-tree:open-folders:${userId}`;
    localStorage.setItem(key, JSON.stringify(nextPaths));
  }, paths);
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
  await expect(page.getByTestId('model-name-input')).toBeVisible();
  await page.getByTestId('open-diagnostics-settings').scrollIntoViewIfNeeded();
  await page.getByTestId('open-diagnostics-settings').click();
  await expect(page.getByTestId('diagnostics-view')).toBeVisible();

  expect(errors.filter((item) => !/favicon/i.test(item))).toEqual([]);
});

test('workspace file can be created through API and opened from file tree', async ({ page }) => {
  const stamp = Date.now();
  const folderPath = `e2e-${stamp}`;
  const filePath = `${folderPath}/open-me.html`;
  const fileContent = `hello from e2e ${stamp}`;

  await login(page);

  try {
    await apiPost(page, '/api/files/mkdir', { path: folderPath });
    await apiPost(page, '/api/files/create', { path: filePath, content: fileContent });

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();
    await page.getByTestId(`toggle-folder-${folderPath}`).click();

    const row = page.getByTestId(`file-row-${filePath}`);
    await expect(row).toBeVisible();
    await row.click();

    await expect(page.getByTestId(`editor-tab-${filePath}`)).toBeVisible();
    await expect(page.locator('.rich-text-editor-content')).toContainText(fileContent);

    await page.getByTestId('settings-button').click();
    await expect(page.getByTestId('model-name-input')).toBeVisible();
    await expect(page.getByTestId('search-provider-select')).toBeVisible();
  } finally {
    await apiPost(page, '/api/files/delete', { path: folderPath }).catch(() => {});
  }
});

test('editor tabs can be reordered by dragging within the tab strip', async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 820 });
  const stamp = Date.now();
  const folderPath = `tab-reorder-e2e-${stamp}`;
  const fileA = `${folderPath}/a.html`;
  const fileB = `${folderPath}/b.html`;
  const fileC = `${folderPath}/c.html`;
  const files = [fileA, fileB, fileC];

  await login(page);

  try {
    await apiPost(page, '/api/files/mkdir', { path: folderPath });
    for (const file of files) {
      await apiPost(page, '/api/files/create', { path: file, content: file });
    }

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();
    await page.getByTestId(`toggle-folder-${folderPath}`).click();
    for (const file of files) {
      await page.getByTestId(`file-row-${file}`).click();
      await expect(page.getByTestId(`editor-tab-${file}`)).toBeVisible();
    }
    await page.getByTestId('layout-2').click();

    const sourceTab = page.getByTestId(`editor-tab-${fileC}`);
    const targetTab = page.getByTestId(`editor-tab-${fileA}`);
    const targetBox = await targetTab.boundingBox();
    if (!targetBox) throw new Error('missing target tab bounds for reorder');
    await sourceTab.dragTo(targetTab, {
      targetPosition: { x: 2, y: targetBox.height / 2 },
    });

    const orderedTabs = await page
      .locator('.editor-tab-item')
      .evaluateAll((tabs) => tabs.map((tab) => tab.getAttribute('title')));
    expect(orderedTabs.slice(0, 3)).toEqual([fileC, fileA, fileB]);
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

test('trash panel shows deleted items and cleanup controls', async ({ page }) => {
  const stamp = Date.now();
  const folderPath = `trash-e2e-${stamp}`;
  const fileName = `restore-${stamp}.html`;
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
  const fileA = `${folderPath}/a.html`;
  const fileB = `${folderPath}/b.html`;
  const fileC = `${folderPath}/c.html`;

  await login(page);

  try {
    await apiPost(page, '/api/files/mkdir', { path: folderPath });
    await apiPost(page, '/api/files/create', { path: fileA, content: 'a' });
    await apiPost(page, '/api/files/create', { path: fileB, content: 'b' });
    await apiPost(page, '/api/files/create', { path: fileC, content: 'c' });

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();
    await page.getByTestId(`toggle-folder-${folderPath}`).click();

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
  const fileA = `${folderPath}/a.html`;
  const fileB = `${folderPath}/b.html`;
  const fileC = `${folderPath}/c.html`;

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
    await page.getByTestId(`toggle-folder-${folderPath}`).click();

    const rowB = page.getByTestId(`file-row-${fileB}`);
    const rowC = page.getByTestId(`file-row-${fileC}`);
    await expect(rowB).toBeVisible();
    await expect(rowC).toBeVisible();
    await rowC.dragTo(rowB, { targetPosition: { x: 24, y: 2 } });
    await expect(page.getByText('已调整顺序')).toBeVisible();

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();
    await expect(page.getByTestId(`file-row-${fileA}`)).toBeVisible();
    const orderedPaths = await page
      .locator('[data-file-path]')
      .evaluateAll((elements, prefix) => elements.map((element) => element.dataset.filePath).filter((path) => path?.startsWith(`${prefix}/`)), folderPath);
    expect(orderedPaths.filter((path) => path.endsWith('.html')).slice(0, 3)).toEqual([fileA, fileC, fileB]);

    await page.getByTestId(`toggle-folder-${folderA}`).click();
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

test('file tree remembers folder expansion state across reloads', async ({ page }) => {
  const stamp = Date.now();
  const rootFolder = `tree-state-e2e-${stamp}`;
  const childFolder = `${rootFolder}/nested`;
  const leafFile = `${childFolder}/note.txt`;

  await login(page);

  try {
    await apiPost(page, '/api/files/mkdir', { path: rootFolder });
    await apiPost(page, '/api/files/mkdir', { path: childFolder });
    await apiPost(page, '/api/files/create', { path: leafFile, content: 'persist me' });
    await setStoredOpenFolders(page, []);

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();

    await expect(page.getByTestId(`file-row-${rootFolder}`)).toBeVisible();
    await expect(page.getByTestId(`file-row-${childFolder}`)).toHaveCount(0);
    await page.getByTestId(`toggle-folder-${rootFolder}`).click();
    await expect(page.getByTestId(`file-row-${childFolder}`)).toBeVisible();

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();
    await expect(page.getByTestId(`file-row-${childFolder}`)).toBeVisible();
    await page.getByTestId(`toggle-folder-${rootFolder}`).click();
    await expect(page.getByTestId(`file-row-${childFolder}`)).toHaveCount(0);

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();
    await expect(page.getByTestId(`file-row-${childFolder}`)).toHaveCount(0);
  } finally {
    await apiPost(page, '/api/files/delete', { path: rootFolder }).catch(() => {});
    const trash = await apiGet(page, '/api/files/trash').catch(() => ({ items: [] }));
    for (const item of trash.items || []) {
      if (item.original_path === rootFolder || item.original_path?.startsWith(`${rootFolder}/`)) {
        await apiPost(page, '/api/files/trash/delete', { path: item.path }).catch(() => {});
      }
    }
  }
});

test('workspace file search opens files inside collapsed folders', async ({ page }) => {
  const stamp = Date.now();
  const folderPath = `search-e2e-${stamp}`;
  const nestedFolder = `${folderPath}/nested`;
  const fileName = `find-me-${stamp}.txt`;
  const filePath = `${nestedFolder}/${fileName}`;
  const fileContent = `search result ${stamp}`;

  await login(page);

  try {
    await apiPost(page, '/api/files/mkdir', { path: folderPath });
    await apiPost(page, '/api/files/mkdir', { path: nestedFolder });
    await apiPost(page, '/api/files/create', { path: filePath, content: fileContent });
    await setStoredOpenFolders(page, []);

    await page.reload();
    await expect(page.getByTestId('chat-send-button')).toBeVisible();
    await page.getByTestId('layout-3').click();

    await expect(page.getByTestId(`file-row-${nestedFolder}`)).toHaveCount(0);
    await page.getByTestId('workspace-search-button').click();
    await expect(page.getByTestId('workspace-search-popover')).toBeVisible();
    await page.getByTestId('workspace-search-input').fill(fileName);
    await page.getByTestId(`workspace-search-result-${filePath}`).click();

    await expect(page.getByTestId(`file-row-${nestedFolder}`)).toBeVisible();
    await expect(page.getByTestId(`editor-tab-${filePath}`)).toBeVisible();
    await expect(page.getByTestId('editor-textarea')).toHaveValue(fileContent);
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

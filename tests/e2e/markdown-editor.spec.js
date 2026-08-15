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
      if (!response.ok) throw new Error(`${response.status} ${text}`);
      return text ? JSON.parse(text) : null;
    },
    { path, body }
  );
}

async function openMarkdown(page, path, content) {
  await apiPost(page, '/api/files/create', { path, content });
  await page.reload();
  await page.getByTestId(`file-row-${path}`).click();
  await expect(page.getByTestId('markdown-editor')).toBeVisible();
}

async function enableDarkTheme(page) {
  const appShell = page.locator('.app-shell');
  if (!(await appShell.evaluate((element) => element.classList.contains('dark')))) {
    await page.getByTestId('theme-toggle').click();
  }
  await expect(appShell).toHaveClass(/\bdark\b/);
}

test('Markdown selection tools keep link editing discoverable', async ({ page }) => {
  const path = `markdown-selection-${Date.now()}.md`;

  await login(page);
  try {
    await openMarkdown(page, path, 'Select this paragraph to format it.\n');

    const editor = page.getByTestId('markdown-editor').locator('[contenteditable="true"]').first();
    await editor.click();
    await page.keyboard.press('Control+A');

    const selectionToolbar = page.getByTestId('markdown-selection-toolbar');
    await expect(selectionToolbar).toBeVisible();
    const firstSelectionTool = selectionToolbar.locator('[data-toolbar-item="true"]').first();
    await firstSelectionTool.hover();
    await expect(page.getByRole('tooltip')).toBeVisible();

    const linkButton = selectionToolbar.locator('[data-toolbar-item="true"]').last();
    await linkButton.click();
    const urlInput = page.locator('input[aria-label="链接地址"]');
    await expect(urlInput).toBeVisible();
    await urlInput.fill('https://example.com');
    await page.getByRole('button', { name: '保存链接' }).click();
    await expect(page.locator('.workspace-markdown-editor-content a[href="https://example.com"]')).toBeVisible();
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

test('Markdown image dialog uses the workspace language and controls', async ({ page }) => {
  const path = `markdown-image-dialog-${Date.now()}.md`;

  await login(page);
  try {
    await openMarkdown(page, path, 'Image insertion\n');

    const editor = page.getByTestId('markdown-editor').locator('[contenteditable="true"]').first();
    await editor.click();
    await page.getByTestId('markdown-block-trigger').click();
    const blockToolbar = page.getByTestId('markdown-block-toolbar');
    await expect(blockToolbar).toBeVisible();
    await blockToolbar.getByRole('button', { name: '插入图片' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog).toBeVisible();
    await expect(dialog).toContainText('插入图片');
    await expect(dialog).toContainText('从设备上传图片');
    await expect(dialog).toContainText('或通过图片地址添加');
    await expect(dialog).toContainText('替代文本');
    await expect(dialog).toContainText('图片标题');
    await expect(dialog.getByRole('button', { name: '保存' })).toBeVisible();
    await expect(dialog.getByRole('button', { name: '取消' })).toBeVisible();
    await expect(dialog).not.toContainText('Upload an image');
    await expect(dialog).not.toContainText('Save');
    await expect(dialog).not.toContainText('Cancel');
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

test('Markdown list controls serialize ordered and unordered Markdown', async ({ page }) => {
  const path = `markdown-lists-${Date.now()}.md`;

  await login(page);
  try {
    await openMarkdown(page, path, 'List item\n');

    const editor = page.getByTestId('markdown-editor').locator('[contenteditable="true"]').first();
    await editor.click();
    await page.keyboard.press('Control+End');
    const currentBlockTrigger = page.getByTestId('markdown-block-trigger');
    await expect.poll(() => currentBlockTrigger.evaluate((element) => getComputedStyle(element).opacity)).toBe('1');
    await currentBlockTrigger.click();
    const blockToolbar = page.getByTestId('markdown-block-toolbar');
    await expect(blockToolbar).toBeVisible();

    const unorderedList = blockToolbar.getByRole('radio', { name: '无序列表' });
    await unorderedList.click();
    await expect(page.locator('.workspace-markdown-editor-content ul')).toBeVisible();
    await expect(unorderedList).toHaveAttribute('data-state', 'on');
    const unorderedStyle = await page.locator('.workspace-markdown-editor-content ul').evaluate((element) => {
      const style = getComputedStyle(element);
      return { listStyleType: style.listStyleType, paddingLeft: style.paddingLeft };
    });
    expect(unorderedStyle.listStyleType).toBe('disc');
    expect(Number.parseFloat(unorderedStyle.paddingLeft)).toBeGreaterThan(0);

    await page.locator('.workspace-markdown-editor-content ul li').first().click();
    await expect.poll(() => currentBlockTrigger.evaluate((element) => getComputedStyle(element).opacity)).toBe('1');
    await currentBlockTrigger.click();
    const listBlockToolbar = page.getByTestId('markdown-block-toolbar');
    await expect(listBlockToolbar).toBeVisible();
    await expect(listBlockToolbar.locator('[role="combobox"]')).toHaveCount(0);
    await expect(listBlockToolbar.locator('xpath=..')).toHaveAttribute('data-list-context', 'true');
    const listToolbarWidth = await listBlockToolbar.evaluate((element) => element.getBoundingClientRect().width);
    expect(listToolbarWidth).toBeLessThan(280);

    await page.getByTestId('markdown-mode-source').click();
    const unorderedMarkdown = await page.getByTestId('editor-textarea').inputValue();
    expect(unorderedMarkdown).toMatch(/(?:^|\n)[*-]\s/);

    await page.getByTestId('markdown-mode-rich-text').click();
    await page.locator('.workspace-markdown-editor-content ul li').first().click();
    await page.getByTestId('markdown-block-trigger').click();
    await expect(blockToolbar).toBeVisible();

    const orderedList = blockToolbar.getByRole('radio', { name: '有序列表' });
    await orderedList.click();
    await expect(page.locator('.workspace-markdown-editor-content ol')).toBeVisible();
    await expect(orderedList).toHaveAttribute('data-state', 'on');
    const orderedStyle = await page.locator('.workspace-markdown-editor-content ol').evaluate((element) => {
      const style = getComputedStyle(element);
      return { listStyleType: style.listStyleType, paddingLeft: style.paddingLeft };
    });
    expect(orderedStyle.listStyleType).toBe('decimal');
    expect(Number.parseFloat(orderedStyle.paddingLeft)).toBeGreaterThan(0);

    await page.getByTestId('markdown-mode-source').click();
    const orderedMarkdown = await page.getByTestId('editor-textarea').inputValue();
    expect(orderedMarkdown).toMatch(/(?:^|\n)1\./);
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

test('Markdown block type menu changes the active block', async ({ page }) => {
  const path = `markdown-block-type-${Date.now()}.md`;

  await login(page);
  try {
    await enableDarkTheme(page);
    await openMarkdown(page, path, 'Paragraph to transform.\n');
    const editor = page.getByTestId('markdown-editor').locator('[contenteditable="true"]').first();
    await editor.click();
    await page.keyboard.press('Control+Home');
    await page.getByTestId('markdown-block-trigger').click();
    await expect(page.locator('.mdxeditor-popup-container')).toHaveCSS('position', 'fixed');
    await page.waitForTimeout(4000);
    await expect(page.getByTestId('markdown-block-toolbar')).toBeVisible();

    const blockType = page.getByTestId('markdown-block-toolbar').locator('[role="combobox"]').first();
    await expect(blockType).toHaveText(/\S+/);
    await expect(blockType).toHaveCSS('width', '112px');
    await blockType.hover();
    await expect(page.getByRole('tooltip')).toBeVisible();
    await blockType.click();
    await expect(page.getByRole('listbox')).toBeVisible();
    const listboxBackground = await page.getByRole('listbox').evaluate((element) => getComputedStyle(element).backgroundColor);
    expect(listboxBackground).toBe('rgb(31, 32, 35)');
    await page.getByRole('option').filter({ hasText: /2/ }).first().click();
    await expect(page.locator('.workspace-markdown-editor-content h2')).toContainText('Paragraph to transform.');
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

test('Markdown code block controls live outside the code surface', async ({ page }) => {
  const path = `markdown-code-${Date.now()}.md`;

  await login(page);
  try {
    await enableDarkTheme(page);
    await openMarkdown(page, path, '```text\nhello\n```\n');
    const codeContent = page.locator('.cm-content');
    await expect(codeContent).toBeVisible();
    await page.waitForTimeout(600);

    const codeWrapper = page.locator('[class*="codeMirrorWrapper"]').first();
    const codeToolbar = page.getByTestId('markdown-code-toolbar');
    await expect(codeToolbar).toHaveCount(0);
    const lineBeforeFocusBox = await page.locator('.cm-line').first().boundingBox();
    expect(lineBeforeFocusBox).not.toBeNull();
    const lineYBeforeFocus = lineBeforeFocusBox.y;

    await codeContent.click();
    await expect(codeToolbar).toBeVisible();
    await expect(codeWrapper.locator(':scope > [class*="codeMirrorToolbar"]')).toHaveCSS('display', 'none');
    await expect(codeToolbar.locator('[role="combobox"]')).toHaveCount(1);

    const toolbarBox = await codeToolbar.boundingBox();
    const firstCodeLineBox = await page.locator('.cm-line').first().boundingBox();
    const wrapperBox = await codeWrapper.boundingBox();
    expect(toolbarBox).not.toBeNull();
    expect(firstCodeLineBox).not.toBeNull();
    expect(wrapperBox).not.toBeNull();
    expect(toolbarBox.bottom <= wrapperBox.y || toolbarBox.y >= wrapperBox.y + wrapperBox.height).toBe(true);

    const editorSurface = await page.getByTestId('markdown-editor').locator('.workspace-markdown-editor').evaluate((element) => {
      const style = getComputedStyle(element);
      return { background: style.backgroundColor, color: style.color };
    });
    expect(editorSurface.background).not.toBe('rgb(255, 255, 255)');
    expect(editorSurface.color).toBe('rgb(229, 231, 235)');
    expect(Math.abs(firstCodeLineBox.y - lineYBeforeFocus)).toBeLessThan(1);

    const copyButton = page.getByTestId('markdown-code-copy');
    await copyButton.hover();
    await expect(page.getByRole('tooltip')).toBeVisible();
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write'], { origin: new URL(page.url()).origin });
    await copyButton.click();
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('hello');

    const languageControl = codeToolbar.locator('[role="combobox"]').first();
    await expect(languageControl).toHaveText('Text');
    await expect(languageControl).toHaveCSS('width', '112px');
    await languageControl.click();
    await expect(page.getByRole('listbox')).toBeVisible();
    await page.waitForTimeout(4000);
    await expect(page.getByRole('listbox')).toBeVisible();
    await page.getByRole('option', { name: 'Bash', exact: true }).click();
    await expect(languageControl).toHaveText('Bash');

    await page.getByTestId('markdown-code-delete').click();
    await expect(codeWrapper).toHaveCount(0);
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

test('Markdown editor does not render a fixed undo toolbar', async ({ page }) => {
  const path = `markdown-toolbar-${Date.now()}.md`;

  await login(page);
  try {
    await openMarkdown(page, path, 'Toolbar-free Markdown.\n');
    await expect(page.locator('.workspace-markdown-top-toolbar')).toHaveCount(0);
    await expect(page.getByTestId('markdown-editor').locator('[aria-label="Undo"]')).toHaveCount(0);
    await expect(page.getByTestId('markdown-editor').locator('[aria-label="Redo"]')).toHaveCount(0);
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

test('Markdown contextual surfaces keep list and checklist layout stable', async ({ page }) => {
  const path = `markdown-list-layout-${Date.now()}.md`;

  await login(page);
  try {
    await enableDarkTheme(page);
    await openMarkdown(page, path, [
      '- item one',
      '- item two',
      '',
      '- [ ] pending item',
      '- [x] completed item',
    ].join('\n'));

    const content = page.locator('.workspace-markdown-editor-content');
    const listItem = content.locator('ul:not([data-type="taskList"]) li').first();
    await listItem.click();
    const currentBlockTrigger = page.getByTestId('markdown-block-trigger');
    await expect.poll(() => currentBlockTrigger.evaluate((element) => getComputedStyle(element).opacity)).toBe('1');
    await currentBlockTrigger.click();
    const blockToolbar = page.getByTestId('markdown-block-toolbar');
    await expect(blockToolbar).toBeVisible();
    await expect(blockToolbar.locator('[role="combobox"]')).toHaveCount(0);

    const checklistItem = content.locator('li[role="checkbox"]').filter({ hasText: 'pending item' }).first();
    await expect(checklistItem).toBeVisible();
    const checklistLayout = await checklistItem.evaluate((element) => {
      const style = getComputedStyle(element);
      const before = getComputedStyle(element, '::before');
      const after = getComputedStyle(element, '::after');
      return {
        paddingLeft: Number.parseFloat(style.paddingLeft),
        beforeLeft: before.left,
        beforeWidth: before.width,
        beforeBackgroundImage: before.backgroundImage,
        afterBorderLeft: after.borderLeftStyle,
        afterBorderBottom: after.borderBottomStyle,
        afterBorderRight: after.borderRightStyle,
        afterDisplay: after.display,
        text: element.textContent,
      };
    });
    expect(checklistLayout.paddingLeft).toBeGreaterThan(20);
    expect(checklistLayout.beforeLeft).toBe('0px');
    expect(checklistLayout.beforeWidth).toBe('16px');
    expect(checklistLayout.beforeBackgroundImage).toBe('none');
    expect(checklistLayout.afterBorderLeft).toBe('none');
    expect(checklistLayout.afterBorderBottom).toBe('none');
    expect(checklistLayout.afterDisplay).toBe('none');
    expect(checklistLayout.text).toContain('pending item');

    const checkedItem = content.locator('li[role="checkbox"][aria-checked="true"]').first();
    const checkedStyle = await checkedItem.evaluate((element) => {
      const before = getComputedStyle(element, '::before');
      const after = getComputedStyle(element, '::after');
      return {
        beforeBackgroundImage: before.backgroundImage,
        afterBorderLeft: after.borderLeftStyle,
        afterBorderBottom: after.borderBottomStyle,
        afterBorderRight: after.borderRightStyle,
        afterBorderTop: after.borderTopStyle,
        afterDisplay: after.display,
      };
    });
    expect(checkedStyle.beforeBackgroundImage).toBe('none');
    expect(checkedStyle.afterBorderLeft).toBe('solid');
    expect(checkedStyle.afterBorderBottom).toBe('solid');
    expect(checkedStyle.afterBorderRight).toBe('none');
    expect(checkedStyle.afterBorderTop).toBe('none');
    expect(checkedStyle.afterDisplay).toBe('block');
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

test('Markdown table controls reveal per row and keep tooltips localized', async ({ page }) => {
  const path = `markdown-table-controls-${Date.now()}.md`;

  await login(page);
  try {
    await enableDarkTheme(page);
    await openMarkdown(page, path, [
      '| Name | Status |',
      '| --- | --- |',
      '| Perseus | Ready |',
      '| Editor | Stable |',
    ].join('\n'));

    const table = page.locator('.workspace-markdown-editor-content table');
    await expect(table).toBeVisible();
    const controls = table.locator('[data-tool-cell="true"] button');
    await expect(controls).not.toHaveCount(0);
    const hiddenOpacity = await controls.first().evaluate((element) => getComputedStyle(element).opacity);
    expect(hiddenOpacity).toBe('0');

    const firstRow = table.locator('tbody tr').first();
    const rowControl = firstRow.locator('[data-tool-cell="true"] button').first();
    const otherRowControl = table.locator('tbody tr').nth(1).locator('[data-tool-cell="true"] button').first();
    const columnControl = table.locator('thead [data-tool-cell="true"] button').first();
    await expect(rowControl).toBeVisible();
    await expect(columnControl).toBeVisible();
    await firstRow.locator(':is(th, td):not([data-tool-cell="true"])').first().hover();
    await expect.poll(() => rowControl.evaluate((element) => getComputedStyle(element).opacity)).toBe('1');
    await expect.poll(() => otherRowControl.evaluate((element) => getComputedStyle(element).opacity)).toBe('0');

    const tableMenuButton = rowControl;
    await tableMenuButton.hover();
    await expect(page.getByRole('tooltip')).toBeVisible();
    const tooltipText = await page.getByRole('tooltip').last().textContent();
    expect(tooltipText).toMatch(/[\u4e00-\u9fff]/);
    expect(tooltipText).not.toMatch(/table\.|align|delete/i);

    await tableMenuButton.click();
    const rowMenu = page.locator('[class*="tableColumnEditorPopoverContent"]').last();
    await expect(rowMenu).toBeVisible();
    await page.waitForTimeout(3000);
    await expect(rowMenu).toBeVisible();
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

test('Markdown contextual menus survive an autosave reconciliation', async ({ page }) => {
  const path = `markdown-menu-autosave-${Date.now()}.md`;

  await login(page);
  try {
    await enableDarkTheme(page);
    await openMarkdown(page, path, 'Menu should stay open while the document saves.\n');
    const editor = page.getByTestId('markdown-editor').locator('[contenteditable="true"]').first();
    await editor.click();
    await page.keyboard.press('Control+End');
    await page.keyboard.type(' changed');
    await page.getByTestId('markdown-block-trigger').click();
    const blockToolbar = page.getByTestId('markdown-block-toolbar');
    await expect(blockToolbar).toBeVisible();
    const blockType = blockToolbar.locator('[role="combobox"]').first();
    await blockType.click();
    await expect(page.getByRole('listbox')).toBeVisible();
    await page.waitForTimeout(3000);
    await expect(page.getByRole('listbox')).toBeVisible();
  } finally {
    await apiPost(page, '/api/files/delete', { path }).catch(() => {});
  }
});

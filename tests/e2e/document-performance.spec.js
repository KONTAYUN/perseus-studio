import { test, expect } from '@playwright/test';
import os from 'node:os';
import fs from 'node:fs/promises';
import { login, api, rich } from './helpers/document.js';

test('document resource budgets: 50k, 500k and the UTF-8 read boundary', async ({ page }, info) => {
  test.skip(process.env.PERSEUS_DOCUMENT_PERFORMANCE !== '1', 'Explicit isolated performance acceptance run');
  test.setTimeout(240_000);
  expect(new URL(info.project.use.baseURL || process.env.PERSEUS_E2E_BASE_URL).port).not.toBe('8080');
  await login(page);
  const results = { machine: { platform: os.platform(), cpu: os.cpus()[0].model, ramBytes: os.totalmem(), browser: info.project.name }, cases: [] };
  for (const [name, length] of [['50k', 50_000], ['500k', 500_000], ['8MiB', Math.floor((8 * 1024 * 1024 - 200) / 3)], ['50k-single', 50_000], ['500k-single', 500_000]]) {
    const paragraph = '这是用于验证长文稿分段编辑的中文内容。'.repeat(10);
    let source = name.endsWith('-single') ? '正文'.repeat(length / 2)
      : ('# 性能稿\n\n' + (paragraph + '\n\n').repeat(Math.ceil(length / paragraph.length))).slice(0, length);
    if (name === '8MiB') source += 'a'.repeat(8 * 1024 * 1024 - Buffer.byteLength(source));
    const path = `performance-${name}-${crypto.randomUUID()}.md`;
    await api(page, '/api/files/create', { path, content: source });
    await page.reload(); await page.getByTestId('layout-3').click();
    const started = Date.now();
    await page.getByTestId(`file-row-${path}`).click();
    await expect(rich(page)).toBeVisible({ timeout: 65_000 });
    const openMs = Date.now() - started;
    await rich(page).locator('p').first().click(); await rich(page).press('End');
    if (name === '8MiB') for (let i = 0; i < 90; i++) await page.keyboard.press('Backspace');
    const timings = [];
    for (let i = 0; i < 30; i++) {
      const start = Date.now(); await page.keyboard.insertText('测');
      await rich(page).evaluate(element => new Promise(resolve => requestAnimationFrame(() => resolve(element.textContent.length))));
      timings.push(Date.now() - start);
    }
    timings.sort((a, b) => a - b);
    const heap = await page.evaluate(() => performance.memory?.usedJSHeapSize ?? null);
    results.cases.push({ name, characters: source.length, utf8Bytes: Buffer.byteLength(source), openMs, inputP95Ms: timings[Math.ceil(timings.length * 0.95) - 1], mainHeapBytes: heap });
    await rich(page).press('Control+s');
    await expect.poll(async () => (await api(page, `/api/files/read?path=${encodeURIComponent(path)}`)).content.includes('测'.repeat(30)), { timeout: 65_000 }).toBe(true);
  }
  await fs.mkdir('_local_artifacts/document-closure', { recursive: true });
  await fs.writeFile(`_local_artifacts/document-closure/performance-${info.project.name}.json`, JSON.stringify(results, null, 2));
  console.log(JSON.stringify(results));
  expect(results.cases[0].openMs).toBeLessThanOrEqual(2000);
  expect(results.cases[0].inputP95Ms).toBeLessThanOrEqual(100);
});

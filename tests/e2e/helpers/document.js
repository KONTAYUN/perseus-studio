import { expect } from '@playwright/test';
export async function api(page, path, body, method) {
  return page.evaluate(async ({ path, body, method }) => {
    const response = await fetch(path, { method: method || (body === undefined ? 'GET' : 'POST'), headers: { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
    if (!response.ok) throw new Error(`${response.status}: ${await response.text()}`);
    return response.json();
  }, { path, body, method });
}
export async function login(page) {
  await page.goto('/');
  await page.getByPlaceholder('用户名').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByPlaceholder('密码').fill(process.env.PERSEUS_E2E_PASSWORD || 'admin');
  await page.getByRole('button', { name: /进入|登录/ }).click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
}
export const rich = page => page.getByTestId('rich-text-editor');
export const read = (page, path) => api(page, `/api/files/read?path=${encodeURIComponent(path)}`);
export async function open(page, content) {
  await login(page);
  const path = `closure-${crypto.randomUUID()}.md`;
  await api(page, '/api/files/create', { path, content });
  await page.reload(); await page.getByTestId('layout-3').click();
  await page.getByTestId(`file-row-${path}`).click();
  return path;
}
export async function append(page, locator, text) {
  await locator.click(); await waitForCaret(locator);
  await page.keyboard.press('End'); await page.keyboard.insertText(text);
}
export async function waitForCaret(locator, atEnd = false) {
  // Native selectionchange and the editor's focus reconciliation run after
  // click/key dispatch. Observe the settled caret before sending the next input.
  await locator.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
  await expect.poll(() => locator.evaluate((element, end) => {
    const selection = window.getSelection();
    if (!selection?.isCollapsed || !element.contains(selection.anchorNode)) return false;
    if (!end) return true;
    const remaining = document.createRange();
    remaining.selectNodeContents(element);
    remaining.setStart(selection.anchorNode, selection.anchorOffset);
    return remaining.toString() === '';
  }, atEnd)).toBe(true);
}
export async function paste(page, { html = '', text = '' }) {
  // Deliver real DataTransfer payloads through the editor's DOM event boundary;
  // unlike OS clipboard permissions this is supported by all three engines.
  await waitForCaret(rich(page));
  await rich(page).evaluate((element, payload) => {
    const data = new DataTransfer();
    if (payload.html) data.setData('text/html', payload.html);
    if (payload.text) data.setData('text/plain', payload.text);
    const event = new ClipboardEvent('paste', { clipboardData: data, bubbles: true, cancelable: true });
    // Firefox may ignore clipboardData passed to an untrusted constructor.
    // Supply the fixture on the event, keeping the real editor DOM boundary.
    Object.defineProperty(event, 'clipboardData', { value: data });
    element.dispatchEvent(event);
  }, { html, text });
}

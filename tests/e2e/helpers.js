import { expect } from '@playwright/test';

export async function login(page) {
  await page.goto('/');
  await page.getByTestId('login-username').fill(process.env.PERSEUS_E2E_USERNAME || 'admin');
  await page.getByTestId('login-password').fill(process.env.PERSEUS_E2E_PASSWORD || 'password');
  await page.getByTestId('login-submit').click();
  await expect(page.getByTestId('chat-send-button')).toBeVisible();
}

export async function apiRequest(page, method, path, body) {
  return page.evaluate(
    async ({ method: requestMethod, path: requestPath, body: requestBody }) => {
      const response = await fetch(requestPath, {
        method: requestMethod,
        credentials: 'include',
        headers: requestBody === undefined ? undefined : { 'Content-Type': 'application/json' },
        body: requestBody === undefined ? undefined : JSON.stringify(requestBody),
      });
      const text = await response.text();
      if (!response.ok) throw new Error(`${response.status} ${text}`);
      return text ? JSON.parse(text) : null;
    },
    { method, path, body }
  );
}

export const apiPost = (page, path, body) => apiRequest(page, 'POST', path, body);
export const apiGet = (page, path) => apiRequest(page, 'GET', path);

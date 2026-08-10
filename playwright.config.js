import { defineConfig, devices } from '@playwright/test';

const baseURL = process.env.PERSEUS_E2E_BASE_URL || 'http://127.0.0.1:8080';

export default defineConfig({
  testDir: './tests/e2e',
  timeout: 30_000,
  expect: {
    timeout: 8_000,
  },
  fullyParallel: false,
  workers: 1,
  outputDir: 'test-results',
  reporter: [
    ['list'],
    ['html', { outputFolder: 'playwright-report', open: 'never' }],
    ['json', { outputFile: 'test-results/playwright-results.json' }],
  ],
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chrome',
      use: {
        ...devices['Desktop Chrome'],
      },
    },
  ],
});

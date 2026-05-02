import { defineConfig, devices } from '@playwright/test';
import path from 'path';

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: [['html', { outputFolder: 'playwright-report' }], ['list']],
  use: {
    baseURL: 'http://localhost:5000',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },

  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],

  webServer: [
    {
      command: 'cd .. && go run ./cmd/registry',
      port: 9081,
      reuseExistingServer: !process.env.CI,
      timeout: 30000,
      env: {
        MOONLIGHT_SERVER_PORT: '9081',
        MOONLIGHT_DATABASE_DSN: './data/test_registry.db',
      },
    },
    {
      command: 'npm run dev',
      port: 5000,
      reuseExistingServer: !process.env.CI,
      timeout: 30000,
    },
  ],
});

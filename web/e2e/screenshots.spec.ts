import { test, expect } from '@playwright/test';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const screenshotDir = path.join(__dirname, '../docs/images');

if (!fs.existsSync(screenshotDir)) {
  fs.mkdirSync(screenshotDir, { recursive: true });
}

test.describe('界面截图', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/');
    await page.waitForLoadState('networkidle');
  });

  test('首页截图', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.screenshot({ 
      path: path.join(screenshotDir, 'dashboard.png'),
      fullPage: true 
    });
  });

  test('仓库管理页面截图', async ({ page }) => {
    await page.goto('/admin/repositories');
    
    const loginBtn = page.locator('button.login-btn');
    if (await loginBtn.isVisible()) {
      await page.locator('input[placeholder="用户名"]').fill('admin');
      await page.locator('input[placeholder="密码"]').fill('admin123');
      await loginBtn.click();
      await page.waitForURL('**/admin/repositories');
    }
    
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.screenshot({ 
      path: path.join(screenshotDir, 'repositories.png'),
      fullPage: true 
    });
  });

  test('包管理页面截图', async ({ page }) => {
    await page.goto('/admin/packages');
    
    const loginBtn = page.locator('button.login-btn');
    if (await loginBtn.isVisible()) {
      await page.locator('input[placeholder="用户名"]').fill('admin');
      await page.locator('input[placeholder="密码"]').fill('admin123');
      await loginBtn.click();
      await page.waitForURL('**/admin/packages');
    }
    
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.screenshot({ 
      path: path.join(screenshotDir, 'packages.png'),
      fullPage: true 
    });
  });

  test('安全中心截图', async ({ page }) => {
    await page.goto('/admin/security');
    
    const loginBtn = page.locator('button.login-btn');
    if (await loginBtn.isVisible()) {
      await page.locator('input[placeholder="用户名"]').fill('admin');
      await page.locator('input[placeholder="密码"]').fill('admin123');
      await loginBtn.click();
      await page.waitForURL('**/admin/security');
    }
    
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.screenshot({ 
      path: path.join(screenshotDir, 'security.png'),
      fullPage: true 
    });
  });

  test('帮助中心截图', async ({ page }) => {
    await page.goto('/help');
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.screenshot({ 
      path: path.join(screenshotDir, 'help.png'),
      fullPage: true 
    });
  });

  test('关于页面截图', async ({ page }) => {
    await page.goto('/about');
    await page.setViewportSize({ width: 1280, height: 800 });
    await page.screenshot({ 
      path: path.join(screenshotDir, 'about.png'),
      fullPage: true 
    });
  });
});

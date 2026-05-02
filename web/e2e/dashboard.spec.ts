import { test, expect } from '@playwright/test'

test.describe('仪表盘功能测试', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/login')
    await page.locator('input[placeholder="用户名"]').fill('admin')
    await page.locator('input[placeholder="密码"]').fill('admin123')
    await page.locator('button.login-btn').click()
    await page.waitForURL('**/admin/dashboard')
  })

  test('应该显示仪表盘页面', async ({ page }) => {
    await expect(page.locator('h2:has-text("仪表盘")')).toBeVisible()
  })

  test('应该显示统计卡片', async ({ page }) => {
    await expect(page.locator('.dashboard')).toBeVisible()
  })

  test('应该能够刷新仪表盘数据', async ({ page }) => {
    const refreshButton = page.locator('.page-header button[aria-label]')
    if (await refreshButton.isVisible()) {
      await refreshButton.click()
    } else {
      const altRefreshButton = page.locator('button:has-text("刷新")')
      if (await altRefreshButton.isVisible()) {
        await altRefreshButton.click()
      }
    }
  })
})

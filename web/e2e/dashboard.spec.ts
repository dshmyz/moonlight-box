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
    await expect(page.locator('.stat-card, .el-card')).toBeVisible()
  })
})

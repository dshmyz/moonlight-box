import { test, expect } from '@playwright/test'

test.describe('仓库管理功能测试', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/login')
    await page.locator('input[placeholder="用户名"]').fill('admin')
    await page.locator('input[placeholder="密码"]').fill('admin123')
    await page.locator('button.login-btn').click()
    await page.waitForURL('**/admin/dashboard')
    await page.locator('text=仓库管理').click()
    await page.waitForURL('**/admin/repositories')
  })

  test('应该显示仓库列表页面', async ({ page }) => {
    await expect(page.locator('h2:has-text("仓库管理")')).toBeVisible()
  })

  test('应该能够打开创建仓库对话框', async ({ page }) => {
    await page.locator('button:has-text("创建仓库")').click()
    await expect(page.locator('.el-dialog')).toBeVisible()
  })

  test('应该能够筛选仓库类型', async ({ page }) => {
    await expect(page.locator('.el-tabs')).toBeVisible()
    await page.getByRole('tab', { name: 'Local' }).click()
    await page.waitForTimeout(500)
  })
})

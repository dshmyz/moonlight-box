import { test, expect } from '@playwright/test'

test.describe('认证功能测试', () => {
  test('应该能够显示登录页面', async ({ page }) => {
    await page.goto('/login')
    await expect(page.locator('h1:has-text("Moonlight Registry")')).toBeVisible()
    await expect(page.locator('input[placeholder="用户名"]')).toBeVisible()
    await expect(page.locator('input[placeholder="密码"]')).toBeVisible()
    await expect(page.locator('button.login-btn')).toBeVisible()
  })

  test('应该拒绝使用错误凭据登录', async ({ page }) => {
    await page.goto('/login')
    await page.locator('input[placeholder="用户名"]').fill('admin')
    await page.locator('input[placeholder="密码"]').fill('wrongpassword')
    await page.locator('button.login-btn').click()
    await expect(page.locator('.el-message--error')).toBeVisible()
  })

  test('应该允许使用正确凭据登录并重定向到仪表盘', async ({ page }) => {
    await page.goto('/login')
    await page.locator('input[placeholder="用户名"]').fill('admin')
    await page.locator('input[placeholder="密码"]').fill('admin123')
    await page.locator('button.login-btn').click()
    await page.waitForURL('**/admin/dashboard')
    await expect(page.locator('h2:has-text("仪表盘")')).toBeVisible()
  })

  test('应该在未认证时阻止访问管理页面', async ({ page }) => {
    await page.goto('/admin/dashboard')
    await page.waitForURL('**/login**')
    await expect(page.locator('input[placeholder="用户名"]')).toBeVisible()
  })

  test('应该能够成功登出', async ({ page }) => {
    await page.goto('/login')
    await page.locator('input[placeholder="用户名"]').fill('admin')
    await page.locator('input[placeholder="密码"]').fill('admin123')
    await page.locator('button.login-btn').click()
    await page.waitForURL('**/admin/dashboard')
    
    // 点击用户菜单
    await page.locator('.user-info').click()
    // 等待下拉菜单出现并点击退出登录
    await page.locator('text=退出登录').waitFor({ state: 'visible' })
    await page.locator('text=退出登录').click()
    
    // 确认对话框出现后点击确定
    await page.getByRole('button', { name: '确定' }).click()
    
    // 等待跳转到登录页
    await page.waitForURL('**/login')
  })
})

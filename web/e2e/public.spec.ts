import { test, expect } from '@playwright/test'

test.describe('公共页面功能测试', () => {
  test('应该能够访问首页', async ({ page }) => {
    await page.goto('/')
    await expect(page.locator('h1:has-text("软件包中心")')).toBeVisible()
  })

  test('应该显示浏览仓库页面', async ({ page }) => {
    await page.goto('/')
    await expect(page.locator('.page-title:has-text("软件包中心")')).toBeVisible()
    await expect(page.locator('.page-desc:has-text("统一管理、搜索和分发多语言软件包")')).toBeVisible()
  })

  test('应该能够切换标签页', async ({ page }) => {
    await page.goto('/')
    await page.getByRole('tab', { name: '仓库' }).click()
    await page.waitForTimeout(500)
  })

  test('应该能够搜索包', async ({ page }) => {
    await page.goto('/')
    const searchInput = page.locator('input[placeholder="搜索包名、描述或标签..."]')
    await expect(searchInput).toBeVisible()
    await searchInput.fill('test')
    await page.getByRole('button', { name: '搜索' }).click()
  })
})

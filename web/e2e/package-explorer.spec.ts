import { test, expect } from '@playwright/test'

test.describe('PackageExplorer 公共端', () => {
  test('URL 参数直接访问恢复状态', async ({ page }) => {
    await page.goto('/?q=react&type=npm&sort=download_count&page=2&page_size=24')
    await page.waitForLoadState('networkidle')

    // 验证 Hero 存在
    await expect(page.locator('.public-hero')).toBeVisible()
    // 验证搜索框内容（Hero 中的搜索框）
    await expect(page.locator('.package-search-bar input').first()).toHaveValue('react')
  })

  test('包/仓库 Tab 切换', async ({ page }) => {
    await page.goto('/')
    await page.waitForLoadState('networkidle')

    // 默认在包 Tab
    await expect(page.locator('.browse-tab--active')).toContainText('包')

    // 点击仓库 Tab
    await page.click('.browse-tab:last-child')
    await expect(page.locator('.repos-section')).toBeVisible()
  })

  test('筛选面板行内展开', async ({ page }) => {
    await page.goto('/')
    await page.waitForLoadState('networkidle')

    // 点击筛选按钮
    await page.click('.el-badge .el-button')
    await expect(page.locator('.filter-panel-inline')).toBeVisible()
  })

  test('/ 快捷键聚焦，Esc 清空', async ({ page }) => {
    await page.goto('/')
    await page.waitForLoadState('networkidle')

    await page.keyboard.press('/')
    await expect(page.locator('.package-search-bar input').first()).toBeFocused()

    await page.fill('.package-search-bar input', 'test')
    await page.keyboard.press('Escape')
    await expect(page.locator('.package-search-bar input').first()).toHaveValue('')
  })
})

test.describe('PackageExplorer 管理端', () => {
  test('行内筛选面板展开', async ({ page }) => {
    // 管理端需要登录，这里仅验证 UI 结构
    await page.goto('/admin/packages')
    await page.waitForLoadState('networkidle')

    // 点击筛选按钮
    await page.click('.el-badge .el-button')
    await expect(page.locator('.filter-panel-inline')).toBeVisible()
  })
})

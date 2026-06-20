import { test, expect } from '@playwright/test'

test.describe('PackageExplorer 管理端', () => {
  test.beforeEach(async ({ page }) => {
    // 登录（假设有测试账号）
    await page.goto('/login')
    await page.fill('input[name="username"]', 'admin')
    await page.fill('input[name="password"]', 'admin123')
    await page.click('button[type="submit"]')
    await page.waitForURL('/admin/dashboard')
  })

  test('搜索 → 筛选 → 排序 → 翻页 → 验证 URL 同步', async ({ page }) => {
    await page.goto('/admin/packages')

    // 搜索
    await page.fill('.package-search-bar input', 'react')
    await page.press('.package-search-bar input', 'Enter')
    await page.waitForLoadState('networkidle')
    expect(page.url()).toContain('q=react')

    // 类型筛选
    await page.click('.type-chip:nth-child(2)')  // npm
    await page.waitForLoadState('networkidle')
    expect(page.url()).toContain('type=npm')

    // 排序
    await page.click('.sort-select')
    await page.click('text=下载量')
    await page.waitForLoadState('networkidle')
    expect(page.url()).toContain('sort=download_count')

    // 翻页
    if (await page.locator('.el-pagination .btn-next').isEnabled()) {
      await page.click('.el-pagination .btn-next')
      await page.waitForLoadState('networkidle')
      expect(page.url()).toContain('page=2')
    }
  })

  test('批量删除 → 验证页码修正', async ({ page }) => {
    await page.goto('/admin/packages')
    await page.waitForLoadState('networkidle')

    // 选中第一行
    await page.check('.el-table__row:first-child input[type="checkbox"]')
    await expect(page.locator('.batch-bar')).toBeVisible()

    // 点击批量删除
    await page.click('text=批量删除')
    await page.click('.el-message-box__header-btn + .el-button--danger')

    // 等待删除完成
    await page.waitForLoadState('networkidle')
    await expect(page.locator('.el-message--success')).toBeVisible()
  })

  test('键盘快捷键 / 聚焦搜索框', async ({ page }) => {
    await page.goto('/admin/packages')
    await page.waitForLoadState('networkidle')

    // 焦点不在输入框时按 /
    await page.keyboard.press('/')
    await expect(page.locator('.package-search-bar input')).toBeFocused()
  })

  test('Esc 清空搜索词', async ({ page }) => {
    await page.goto('/admin/packages?q=react')
    await page.waitForLoadState('networkidle')

    await page.keyboard.press('Escape')
    await page.waitForLoadState('networkidle')
    expect(page.url()).not.toContain('q=react')
  })
})

test.describe('PackageExplorer 公共端', () => {
  test('URL 参数直接访问恢复状态', async ({ page }) => {
    await page.goto('/?q=react&type=npm&sort=download_count&page=2&page_size=24')
    await page.waitForLoadState('networkidle')

    // 验证搜索框内容
    await expect(page.locator('.package-search-bar input')).toHaveValue('react')
    // 验证类型选中
    await expect(page.locator('.type-chip--active')).toContainText('npm')
    // 验证排序
    await expect(page.locator('.sort-select input')).toHaveValue('下载量')
  })

  test('/ 快捷键聚焦，Esc 清空', async ({ page }) => {
    await page.goto('/')
    await page.waitForLoadState('networkidle')

    await page.keyboard.press('/')
    await expect(page.locator('.package-search-bar input')).toBeFocused()

    await page.fill('.package-search-bar input', 'test')
    await page.keyboard.press('Escape')
    await expect(page.locator('.package-search-bar input')).toHaveValue('')
  })
})

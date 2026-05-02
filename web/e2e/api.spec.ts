import { test, expect } from '@playwright/test'

test.describe('API 接口测试', () => {
  test('应该能够访问健康检查端点', async ({ request }) => {
    const response = await request.get('http://localhost:9081/health')
    expect(response.ok()).toBeTruthy()
    const body = await response.json()
    expect(body.status).toBe('ok')
  })

  test('应该能够访问 ping 端点', async ({ request }) => {
    const response = await request.get('http://localhost:9081/api/v1/ping')
    expect(response.ok()).toBeTruthy()
    const body = await response.json()
    expect(body.message).toBe('pong')
  })

  test('应该能够使用正确凭据登录 API', async ({ request }) => {
    const response = await request.post('http://localhost:9081/api/v1/auth/login', {
      data: {
        username: 'admin',
        password: 'admin123',
      },
    })
    expect(response.ok()).toBeTruthy()
    const body = await response.json()
    expect(body.data.access_token).toBeTruthy()
  })

  test('应该拒绝使用错误凭据登录 API', async ({ request }) => {
    const response = await request.post('http://localhost:9081/api/v1/auth/login', {
      data: {
        username: 'admin',
        password: 'wrongpassword',
      },
    })
    expect(response.status()).toBe(401)
  })

  test('应该能够在认证后访问仪表盘 API', async ({ request }) => {
    const loginResponse = await request.post('http://localhost:9081/api/v1/auth/login', {
      data: {
        username: 'admin',
        password: 'admin123',
      },
    })
    const loginBody = await loginResponse.json()
    const token = loginBody.data.access_token

    const statsResponse = await request.get('http://localhost:9081/api/v1/dashboard/stats', {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    })
    expect(statsResponse.ok()).toBeTruthy()
  })

  test('应该能够在未认证时拒绝访问仪表盘 API', async ({ request }) => {
    const response = await request.get('http://localhost:9081/api/v1/dashboard/stats')
    expect(response.status()).toBe(401)
  })
})

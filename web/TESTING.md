# 仓库组件测试脚本

## 测试概览

已为所有仓库相关组件创建了完整的测试脚本，使用 Vitest + @vue/test-utils 进行测试。

## 测试覆盖的组件

| 组件 | 测试文件 | 测试用例数 |
|------|----------|------------|
| BasicInfoForm | `__tests__/BasicInfoForm.spec.ts` | 11 |
| AuthConfigForm | `__tests__/AuthConfigForm.spec.ts` | 12 |
| CacheConfigForm | `__tests__/CacheConfigForm.spec.ts` | 12 |
| PermissionsConfigForm | `__tests__/PermissionsConfigForm.spec.ts` | 8 |
| VirtualMembersForm | `__tests__/VirtualMembersForm.spec.ts` | 10 |
| TimeoutConfigForm | `__tests__/TimeoutConfigForm.spec.ts` | 12 |

**总计：65 个测试用例**

## 测试覆盖率

```
Statements   : 80.2% ( 77/96 )
Branches     : 100% ( 68/68 )
Functions    : 71.21% ( 47/66 )
Lines        : 82.5% ( 66/80 )
```

## 运行测试

```bash
# 运行所有测试
npm run test:run

# 运行测试并生成覆盖率报告
npm run test:coverage

# 以 watch 模式运行测试（开发时使用）
npm run test
```

## 测试内容

### BasicInfoForm
- 渲染所有表单字段
- 绑定 name、display_name、description 字段
- 显示类型和包类型选项
- disabled 属性控制
- 提示文本显示

### AuthConfigForm
- 渲染远程地址和认证配置
- Basic Auth 字段显示和隐藏
- Bearer Token 字段显示和隐藏
- API Key 字段显示和隐藏
- proxy_priority 绑定
- 表单数据更新

### CacheConfigForm
- 缓存配置表单渲染
- 缓存启用/禁用状态
- 缓存 TTL 字段显示
- 负向缓存 TTL（仅 proxy 类型）
- 失败缓存规则（仅 proxy 类型）
- 缓存最大大小
- 事件触发

### PermissionsConfigForm
- 权限配置表单渲染
- 允许覆盖开关
- 允许删除开关
- 表单数据更新

### VirtualMembersForm
- 虚拟成员表单渲染
- 成员列表解析
- 添加/删除成员
- 成员索引显示
- 事件触发

### TimeoutConfigForm
- 超时配置表单渲染
- 超时时间字段
- 最大重定向次数字段
- 跳过证书校验开关
- 提示文本显示
- 表单数据更新

## 技术栈

- **测试框架**: Vitest
- **测试工具**: @vue/test-utils
- **UI 库**: Element Plus
- **环境**: jsdom

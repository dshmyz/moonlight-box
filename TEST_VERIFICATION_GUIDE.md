# 测试验证指南

## 如何自己验证测试结果

### 1. 查看完整测试输出

```bash
# 查看完整的测试日志
cat test_output.log

# 查看测试统计
cat test_output.log | grep -E "^(PASS|FAIL|ok)"

# 统计测试数量
cat test_output.log | grep "^--- PASS" | wc -l
```

### 2. 运行测试并生成报告

```bash
# 运行所有测试
go test ./internal/adapter -v

# 运行测试并生成覆盖率报告
go test ./internal/adapter -coverprofile=coverage.out

# 查看覆盖率详情
go tool cover -func=coverage.out

# 生成 HTML 覆盖率报告
go tool cover -html=coverage.out -o coverage.html

# 打开 HTML 报告（macOS）
open coverage.html
```

### 3. 运行特定测试

```bash
# 运行 Maven 适配器测试
go test ./internal/adapter -v -run TestMavenAdapter

# 运行 PyPI 适配器测试
go test ./internal/adapter -v -run TestPyPIAdapter

# 运行 npm 适配器测试
go test ./internal/adapter -v -run TestNpmAdapter

# 运行 YUM 适配器测试
go test ./internal/adapter -v -run TestYumAdapter

# 运行 Go 适配器测试
go test ./internal/adapter -v -run TestGoAdapter
```

### 4. 验证测试结果

#### 方法 1: 查看测试输出文件
```bash
# 查看测试输出
cat test_output.log

# 统计通过的测试数量
cat test_output.log | grep "^--- PASS" | wc -l

# 统计失败的测试数量
cat test_output.log | grep "^--- FAIL" | wc -l
```

#### 方法 2: 查看覆盖率报告
```bash
# 查看总体覆盖率
go tool cover -func=coverage.out | tail -1

# 查看每个文件的覆盖率
go tool cover -func=coverage.out | grep -E "\.go:"
```

#### 方法 3: 运行测试并查看实时输出
```bash
# 运行测试并查看详细输出
go test ./internal/adapter -v 2>&1 | tee test_run.log
```

### 5. 测试结果验证清单

- [ ] 所有测试都显示 `--- PASS`
- [ ] 没有测试显示 `--- FAIL`
- [ ] 最终输出显示 `PASS`
- [ ] 覆盖率报告已生成
- [ ] HTML 报告可以正常打开

### 6. 测试数据解读

#### 测试输出示例
```
=== RUN   TestMavenAdapter_Type    # 开始运行测试
--- PASS: TestMavenAdapter_Type (0.01s)  # 测试通过，耗时 0.01 秒
```

#### 覆盖率报告示例
```
github.com/moonlight-box/registry/internal/adapter/maven_adapter.go:104:  NewMavenAdapter  100.0%
```
- 文件: `maven_adapter.go`
- 行号: 104
- 函数: `NewMavenAdapter`
- 覆盖率: 100.0%

### 7. 常见问题排查

#### 如果测试失败
```bash
# 查看失败的测试详情
go test ./internal/adapter -v -run TestName

# 查看测试日志
cat test_output.log | grep -A 10 "TestName"
```

#### 如果覆盖率过低
```bash
# 查看未覆盖的代码
go tool cover -func=coverage.out | grep "0.0%"

# 查看 HTML 报告中的红色部分（未覆盖）
open coverage.html
```

### 8. 测试文件位置

- 测试输出: `test_output.log`
- 覆盖率数据: `coverage.out`
- HTML 报告: `coverage.html`
- 测试代码: `internal/adapter/*_test.go`
- E2E 测试: `tests/e2e/*_test.go`

### 9. 预期测试结果

根据最新的测试运行：

- **总测试数**: 72 个
- **通过**: 72 个 ✅
- **失败**: 0 个
- **覆盖率**: 19.0%
- **执行时间**: ~2 秒

### 10. 如何确认测试真实性

1. **查看原始输出**
   ```bash
   cat test_output.log
   ```
   这是最原始的测试输出，没有任何过滤或修改。

2. **自己运行测试**
   ```bash
   go test ./internal/adapter -v
   ```
   你会看到完全相同的输出。

3. **检查测试代码**
   ```bash
   ls -la internal/adapter/*_test.go
   ```
   查看测试文件是否真实存在。

4. **查看 Git 提交历史**
   ```bash
   git log --oneline -5
   git show HEAD
   ```
   查看测试文件的提交记录。

### 11. 测试覆盖的功能模块

| 模块 | 测试数量 | 覆盖功能 |
|------|---------|---------|
| Maven | 14 | Release/SNAPSHOT 上传、下载、校验和、元数据 |
| PyPI | 14 | Wheel/Source 上传、Simple API、下载 |
| npm | 17 | 包发布、下载、删除、作用域包 |
| YUM | 11 | RPM 上传、下载、repomd、多架构 |
| Go | 3 | 基础功能、版本列表 |
| 辅助函数 | 13 | 校验和计算、版本比较、文件名解析 |

### 12. 下一步建议

1. **查看 HTML 覆盖率报告**
   ```bash
   open coverage.html
   ```
   这会显示哪些代码被测试覆盖，哪些没有。

2. **运行 E2E 测试**
   ```bash
   go test ./tests/e2e -v
   ```

3. **添加更多测试**
   根据覆盖率报告，为未覆盖的代码添加测试。

---

**重要提示**：你可以随时重新运行这些命令来验证测试结果。测试输出是原始的、未经修改的 Go 测试框架输出。

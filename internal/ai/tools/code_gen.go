package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// CodeGenTool 代码生成工具
type CodeGenTool struct {
	BaseTool
}

// NewCodeGenTool 创建代码生成工具
func NewCodeGenTool() *CodeGenTool {
	return &CodeGenTool{}
}

// Name 返回工具名称
func (t *CodeGenTool) Name() string {
	return "code_generator"
}

// Description 返回工具描述
func (t *CodeGenTool) Description() string {
	return "生成包的使用示例代码"
}

// Parameters 返回工具参数的 JSON Schema
func (t *CodeGenTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"package_name": {
				"type": "string",
				"description": "包名称"
			},
			"package_type": {
				"type": "string",
				"description": "包类型 (npm, maven, pypi, go, nuget, yum, apt, generic)"
			},
			"language": {
				"type": "string",
				"description": "编程语言 (javascript, typescript, java, python, go, csharp)"
			},
			"scenario": {
				"type": "string",
				"description": "使用场景 (basic, advanced, testing, integration)",
				"enum": ["basic", "advanced", "testing", "integration"]
			},
			"framework": {
				"type": "string",
				"description": "框架名称 (react, vue, spring, django, gin, etc.)"
			}
		},
		"required": ["package_name", "package_type"]
	}`)
}

// Execute 执行工具并返回结果
func (t *CodeGenTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	packageName, ok := params["package_name"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: package_name")
	}

	packageType, ok := params["package_type"].(string)
	if !ok {
		return "", fmt.Errorf("缺少必需参数: package_type")
	}

	language, _ := params["language"].(string)
	scenario, _ := params["scenario"].(string)
	framework, _ := params["framework"].(string)

	// 根据包类型推断语言
	if language == "" {
		language = t.inferLanguage(packageType)
	}

	// 生成示例代码
	return t.generateCode(packageName, packageType, language, scenario, framework)
}

// inferLanguage 根据包类型推断编程语言
func (t *CodeGenTool) inferLanguage(packageType string) string {
	switch packageType {
	case "npm":
		return "javascript"
	case "maven":
		return "java"
	case "pypi":
		return "python"
	case "go":
		return "go"
	case "nuget":
		return "csharp"
	default:
		return "javascript"
	}
}

// generateCode 生成代码示例
func (t *CodeGenTool) generateCode(packageName, packageType, language, scenario, framework string) (string, error) {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("💻 **%s** 使用示例 (%s)\n\n", packageName, strings.Title(language)))

	// 安装说明
	sb.WriteString("📦 安装:\n")
	sb.WriteString(t.generateInstallCommand(packageName, packageType))
	sb.WriteString("\n\n")

	// 基础用法
	if scenario == "" || scenario == "basic" {
		sb.WriteString("📝 基础用法:\n")
		sb.WriteString(t.generateBasicUsage(packageName, language, framework))
		sb.WriteString("\n")
	}

	// 高级用法
	if scenario == "advanced" {
		sb.WriteString("🚀 高级用法:\n")
		sb.WriteString(t.generateAdvancedUsage(packageName, language, framework))
		sb.WriteString("\n")
	}

	// 测试示例
	if scenario == "testing" {
		sb.WriteString("🧪 测试示例:\n")
		sb.WriteString(t.generateTestExample(packageName, language, framework))
		sb.WriteString("\n")
	}

	// 集成示例
	if scenario == "integration" {
		sb.WriteString("🔗 集成示例:\n")
		sb.WriteString(t.generateIntegrationExample(packageName, language, framework))
		sb.WriteString("\n")
	}

	// 最佳实践
	sb.WriteString("💡 最佳实践:\n")
	sb.WriteString(t.generateBestPractices(packageName, language))
	sb.WriteString("\n")

	// 常见问题
	sb.WriteString("❓ 常见问题:\n")
	sb.WriteString(t.generateCommonIssues(packageName, language))

	return sb.String(), nil
}

// generateInstallCommand 生成安装命令
func (t *CodeGenTool) generateInstallCommand(packageName, packageType string) string {
	switch packageType {
	case "npm":
		return fmt.Sprintf("```bash\nnpm install %s\n# 或\nyarn add %s\n# 或\npnpm add %s\n```",
			packageName, packageName, packageName)
	case "maven":
		return fmt.Sprintf("```xml\n<dependency>\n  <groupId>com.example</groupId>\n  <artifactId>%s</artifactId>\n  <version>最新版本</version>\n</dependency>\n```",
			packageName)
	case "pypi":
		return fmt.Sprintf("```bash\npip install %s\n# 或\npipenv install %s\n# 或\npoetry add %s\n```",
			packageName, packageName, packageName)
	case "go":
		return fmt.Sprintf("```bash\ngo get github.com/example/%s\n```", packageName)
	case "nuget":
		return fmt.Sprintf("```bash\ndotnet add package %s\n# 或\nInstall-Package %s\n```",
			packageName, packageName)
	default:
		return fmt.Sprintf("请参考官方文档安装 %s", packageName)
	}
}

// generateBasicUsage 生成基础用法示例
func (t *CodeGenTool) generateBasicUsage(packageName, language, framework string) string {
	switch language {
	case "javascript", "typescript":
		return fmt.Sprintf("```javascript\n// 导入模块\nconst %s = require('%s');\n\n// 基础使用示例\nconst instance = new %s();\ninstance.doSomething();\n\n// 或者使用 ES6 模块\nimport { Component } from '%s';\nconst component = new Component();\n```",
			t.toCamelCase(packageName), packageName, t.toCamelCase(packageName), packageName)
	case "java":
		return fmt.Sprintf("```java\nimport com.example.%s.*;\n\npublic class Main {\n    public static void main(String[] args) {\n        // 创建实例\n        %s instance = new %s();\n        \n        // 使用基础功能\n        instance.doSomething();\n    }\n}\n```",
			packageName, t.toCamelCase(packageName), t.toCamelCase(packageName))
	case "python":
		return fmt.Sprintf("```python\n# 导入模块\nimport %s\n\n# 基础使用示例\ninstance = %s.%s()\ninstance.do_something()\n\n# 或者导入特定类\nfrom %s import ClassName\nobj = ClassName()\n```",
			packageName, packageName, t.toCamelCase(packageName), packageName)
	case "go":
		return fmt.Sprintf("```go\npackage main\n\nimport (\n    \"github.com/example/%s\"\n)\n\nfunc main() {\n    // 创建实例\n    instance := %s.New()\n    \n    // 使用基础功能\n    instance.DoSomething()\n}\n```",
			packageName, packageName)
	case "csharp":
		return fmt.Sprintf("```csharp\nusing %s;\n\nclass Program\n{\n    static void Main(string[] args)\n    {\n        // 创建实例\n        var instance = new %s();\n        \n        // 使用基础功能\n        instance.DoSomething();\n    }\n}\n```",
			packageName, t.toCamelCase(packageName))
	default:
		return "请参考官方文档获取基础用法示例"
	}
}

// generateAdvancedUsage 生成高级用法示例
func (t *CodeGenTool) generateAdvancedUsage(packageName, language, framework string) string {
	switch language {
	case "javascript", "typescript":
		return fmt.Sprintf("```javascript\n// 高级配置\nconst advancedConfig = {\n  option1: 'value1',\n  option2: 'value2',\n  plugins: ['plugin1', 'plugin2']\n};\n\nconst instance = new %s(advancedConfig);\n\n// 使用高级功能\ninstance.advancedMethod({\n  customOption: true,\n  callback: (result) => {\n    console.log('处理结果:', result);\n  }\n});\n\n// 异步操作\nasync function advancedUsage() {\n  try {\n    const result = await instance.asyncMethod();\n    console.log(result);\n  } catch (error) {\n    console.error('错误:', error);\n  }\n}\n```",
			t.toCamelCase(packageName))
	case "java":
		return fmt.Sprintf("```java\n// 高级配置\n%sConfig config = %sConfig.builder()\n    .option1(\"value1\")\n    .option2(\"value2\")\n    .enableFeature(true)\n    .build();\n\n%s instance = new %s(config);\n\n// 使用高级功能\ninstance.advancedMethod(new Callback() {\n    @Override\n    public void onSuccess(Result result) {\n        System.out.println(\"成功: \" + result);\n    }\n    \n    @Override\n    public void onError(Exception e) {\n        System.err.println(\"错误: \" + e.getMessage());\n    }\n});\n```",
			t.toCamelCase(packageName), t.toCamelCase(packageName), t.toCamelCase(packageName), t.toCamelCase(packageName))
	case "python":
		return fmt.Sprintf("```python\n# 高级配置\nconfig = {\n    'option1': 'value1',\n    'option2': 'value2',\n    'enable_feature': True\n}\n\ninstance = %s.%s(config)\n\n# 使用高级功能\ndef callback(result):\n    print(f'处理结果: {result}')\n\ninstance.advanced_method(\n    custom_option=True,\n    callback=callback\n)\n\n# 异步操作 (如果支持)\nimport asyncio\n\nasync def advanced_usage():\n    result = await instance.async_method()\n    print(result)\n\nasyncio.run(advanced_usage())\n```",
			packageName, t.toCamelCase(packageName))
	default:
		return "请参考官方文档获取高级用法示例"
	}
}

// generateTestExample 生成测试示例
func (t *CodeGenTool) generateTestExample(packageName, language, framework string) string {
	switch language {
	case "javascript", "typescript":
		return fmt.Sprintf("```javascript\n// 使用 Jest 测试\nconst %s = require('%s');\n\ndescribe('%s', () => {\n  let instance;\n\n  beforeEach(() => {\n    instance = new %s();\n  });\n\n  test('should do something correctly', () => {\n    const result = instance.doSomething();\n    expect(result).toBeDefined();\n    expect(result).toBe('expected value');\n  });\n\n  test('should handle errors', () => {\n    expect(() => {\n      instance.throwError();\n    }).toThrow();\n  });\n\n  afterEach(() => {\n    instance.cleanup();\n  });\n});\n```",
			t.toCamelCase(packageName), packageName, packageName, t.toCamelCase(packageName))
	case "java":
		return fmt.Sprintf("```java\nimport org.junit.jupiter.api.*;\nimport static org.junit.jupiter.api.Assertions.*;\n\nclass %sTest {\n    private %s instance;\n\n    @BeforeEach\n    void setUp() {\n        instance = new %s();\n    }\n\n    @Test\n    void testDoSomething() {\n        String result = instance.doSomething();\n        assertNotNull(result);\n        assertEquals(\"expected value\", result);\n    }\n\n    @Test\n    void testErrorHandling() {\n        assertThrows(Exception.class, () -> {\n            instance.throwError();\n        });\n    }\n\n    @AfterEach\n    void tearDown() {\n        instance.cleanup();\n    }\n}\n```",
			t.toCamelCase(packageName), t.toCamelCase(packageName), t.toCamelCase(packageName))
	case "python":
		return fmt.Sprintf("```python\nimport unittest\nimport %s\n\nclass Test%s(unittest.TestCase):\n    def setUp(self):\n        self.instance = %s.%s()\n\n    def test_do_something(self):\n        result = self.instance.do_something()\n        self.assertIsNotNone(result)\n        self.assertEqual(result, 'expected value')\n\n    def test_error_handling(self):\n        with self.assertRaises(Exception):\n            self.instance.throw_error()\n\n    def tearDown(self):\n        self.instance.cleanup()\n\nif __name__ == '__main__':\n    unittest.main()\n```",
			packageName, t.toCamelCase(packageName), packageName, t.toCamelCase(packageName))
	default:
		return "请参考官方文档获取测试示例"
	}
}

// generateIntegrationExample 生成集成示例
func (t *CodeGenTool) generateIntegrationExample(packageName, language, framework string) string {
	if framework != "" {
		return t.generateFrameworkIntegration(packageName, language, framework)
	}

	switch language {
	case "javascript", "typescript":
		return fmt.Sprintf("```javascript\n// 与其他库集成\nconst %s = require('%s');\nconst otherLib = require('other-library');\n\n// 创建集成实例\nconst instance = new %s({\n  adapter: otherLib.createAdapter()\n});\n\n// 使用集成功能\ninstance.integrate();\n```",
			t.toCamelCase(packageName), packageName, t.toCamelCase(packageName))
	default:
		return "请参考官方文档获取集成示例"
	}
}

// generateFrameworkIntegration 生成框架集成示例
func (t *CodeGenTool) generateFrameworkIntegration(packageName, language, framework string) string {
	switch framework {
	case "react":
		return fmt.Sprintf("```jsx\nimport React, { useEffect, useState } from 'react';\nimport %s from '%s';\n\nfunction MyComponent() {\n  const [data, setData] = useState(null);\n\n  useEffect(() => {\n    const instance = new %s();\n    instance.getData().then(setData);\n    return () => instance.cleanup();\n  }, []);\n\n  return (\n    <div>\n      {data ? <p>{data}</p> : <p>Loading...</p>}\n    </div>\n  );\n}\n\nexport default MyComponent;\n```",
			t.toCamelCase(packageName), packageName, t.toCamelCase(packageName))
	case "vue":
		return fmt.Sprintf("```vue\n<template>\n  <div>\n    <p v-if=\"data\">{{ data }}</p>\n    <p v-else>Loading...</p>\n  </div>\n</template>\n\n<script>\nimport %s from '%s';\n\nexport default {\n  data() {\n    return {\n      data: null,\n      instance: null\n    };\n  },\n  mounted() {\n    this.instance = new %s();\n    this.instance.getData().then(data => {\n      this.data = data;\n    });\n  },\n  beforeUnmount() {\n    if (this.instance) {\n      this.instance.cleanup();\n    }\n  }\n};\n</script>\n```",
			t.toCamelCase(packageName), packageName, t.toCamelCase(packageName))
	case "spring":
		return fmt.Sprintf("```java\nimport org.springframework.context.annotation.Bean;\nimport org.springframework.context.annotation.Configuration;\n\n@Configuration\npublic class %sConfig {\n    \n    @Bean\n    public %s %sBean() {\n        return new %s();\n    }\n}\n\n// 在服务中使用\n@Service\npublic class MyService {\n    private final %s instance;\n    \n    public MyService(%s instance) {\n        this.instance = instance;\n    }\n    \n    public void doWork() {\n        instance.doSomething();\n    }\n}\n```",
			t.toCamelCase(packageName), t.toCamelCase(packageName), packageName, t.toCamelCase(packageName), t.toCamelCase(packageName), t.toCamelCase(packageName))
	default:
		return fmt.Sprintf("请参考 %s 框架的集成文档", framework)
	}
}

// generateBestPractices 生成最佳实践
func (t *CodeGenTool) generateBestPractices(packageName, language string) string {
	var sb strings.Builder

	sb.WriteString("```markdown\n")
	sb.WriteString(fmt.Sprintf("1. **错误处理**: 始终处理可能的错误和异常\n"))
	sb.WriteString(fmt.Sprintf("2. **资源管理**: 及时释放资源，避免内存泄漏\n"))
	sb.WriteString(fmt.Sprintf("3. **性能优化**: 根据使用场景选择合适的配置\n"))
	sb.WriteString(fmt.Sprintf("4. **安全性**: 验证输入数据，防止注入攻击\n"))
	sb.WriteString(fmt.Sprintf("5. **可维护性**: 编写清晰的代码，添加必要的注释\n"))
	sb.WriteString("```\n")

	return sb.String()
}

// generateCommonIssues 生成常见问题
func (t *CodeGenTool) generateCommonIssues(packageName, language string) string {
	var sb strings.Builder

	sb.WriteString("```markdown\n")
	sb.WriteString(fmt.Sprintf("Q: 如何处理版本冲突？\n"))
	sb.WriteString(fmt.Sprintf("A: 检查依赖树，使用正确的版本约束\n\n"))
	sb.WriteString(fmt.Sprintf("Q: 性能不佳怎么办？\n"))
	sb.WriteString(fmt.Sprintf("A: 启用缓存，优化配置参数\n\n"))
	sb.WriteString(fmt.Sprintf("Q: 如何调试问题？\n"))
	sb.WriteString(fmt.Sprintf("A: 启用详细日志，检查错误堆栈\n"))
	sb.WriteString("```\n")

	return sb.String()
}

// toCamelCase 转换为驼峰命名
func (t *CodeGenTool) toCamelCase(s string) string {
	parts := strings.Split(s, "-")
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.Title(parts[i])
	}
	return strings.Join(parts, "")
}

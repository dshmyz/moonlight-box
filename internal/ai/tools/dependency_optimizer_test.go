package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDependencyOptimizerTool_Parameters(t *testing.T) {
	tool := NewDependencyOptimizerTool()

	// 测试名称
	if tool.Name() != "dependency_optimizer" {
		t.Errorf("expected name 'dependency_optimizer', got '%s'", tool.Name())
	}

	// 测试描述
	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}

	// 测试参数 schema
	params := tool.Parameters()
	if len(params) == 0 {
		t.Fatal("expected non-empty parameters")
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("failed to parse parameters schema: %v", err)
	}

	// 验证 required 字段
	required, ok := schema["required"].([]interface{})
	if !ok {
		t.Fatal("expected 'required' array in schema")
	}
	found := false
	for _, r := range required {
		if r == "project_name" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'project_name' in required fields")
	}

	// 验证 properties
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties in schema")
	}

	expectedParams := []string{"project_name", "package_type", "analysis_scope", "include_transitive", "min_severity"}
	for _, param := range expectedParams {
		if _, ok := properties[param]; !ok {
			t.Errorf("expected '%s' parameter in schema", param)
		}
	}
}

func TestDependencyOptimizerTool_MissingProjectName(t *testing.T) {
	tool := NewDependencyOptimizerTool()

	// 设置空的上下文
	tool.SetContext(&ToolContext{})

	_, err := tool.Execute(context.Background(), map[string]interface{}{})

	if err == nil {
		t.Fatal("expected error for missing project_name")
	}

	expected := "缺少必需参数: project_name"
	if err.Error() != expected {
		t.Errorf("expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestDependencyOptimizerTool_InvalidProjectName(t *testing.T) {
	tool := NewDependencyOptimizerTool()
	tool.SetContext(&ToolContext{})

	// 传入非字符串的 project_name
	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"project_name": 123,
	})

	if err == nil {
		t.Fatal("expected error for invalid project_name type")
	}

	expected := "缺少必需参数: project_name"
	if err.Error() != expected {
		t.Errorf("expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestDependencyOptimizerTool_AnalysisScopeValues(t *testing.T) {
	tool := NewDependencyOptimizerTool()
	tool.SetContext(&ToolContext{})

	// 测试不同的分析范围值
	// 注意: 由于没有真实数据库连接，这里只测试参数解析不报错
	// 实际执行会因数据库连接失败而返回错误，这是预期的
	analysisScopes := []string{"conflicts", "security", "unused", "all", ""}

	for _, scope := range analysisScopes {
		params := map[string]interface{}{
			"project_name":   "test-project",
			"analysis_scope": scope,
		}
		
		_, err := tool.Execute(context.Background(), params)
		// 期望因数据库未配置而失败，而不是参数解析错误
		if err == nil || err.Error() == "缺少必需参数: project_name" {
			t.Errorf("unexpected result for scope '%s': %v", scope, err)
		}
	}
}

func TestDependencyOptimizerTool_PackageTypeValidation(t *testing.T) {
	tool := NewDependencyOptimizerTool()
	tool.SetContext(&ToolContext{})

	// 测试有效的包类型
	validTypes := []string{"npm", "maven", "pypi", "go", "nuget", "generic", ""}
	for _, pkgType := range validTypes {
		params := map[string]interface{}{
			"project_name": "test",
			"package_type": pkgType,
		}
		_, err := tool.Execute(context.Background(), params)
		// 期望因数据库未配置而失败
		if err == nil || err.Error() == "缺少必需参数: project_name" {
			t.Errorf("unexpected result for package_type '%s': %v", pkgType, err)
		}
	}
}

func TestDependencyOptimizerTool_MinSeverityValues(t *testing.T) {
	tool := NewDependencyOptimizerTool()
	tool.SetContext(&ToolContext{})

	// 测试不同的严重级别
	severityLevels := []string{"low", "medium", "high", "critical", ""}
	for _, level := range severityLevels {
		params := map[string]interface{}{
			"project_name": "test",
			"min_severity": level,
		}
		_, err := tool.Execute(context.Background(), params)
		// 期望因数据库未配置而失败
		if err == nil || err.Error() == "缺少必需参数: project_name" {
			t.Errorf("unexpected result for min_severity '%s': %v", level, err)
		}
	}
}

func TestDependencyOptimizerTool_IncludeTransitive(t *testing.T) {
	tool := NewDependencyOptimizerTool()
	tool.SetContext(&ToolContext{})

	// 测试 include_transitive 参数
	params := map[string]interface{}{
		"project_name":        "test",
		"include_transitive":  true,
	}
	_, err := tool.Execute(context.Background(), params)
	// 期望因数据库未配置而失败
	if err == nil || err.Error() == "缺少必需参数: project_name" {
		t.Errorf("unexpected result for include_transitive=true: %v", err)
	}

	params["include_transitive"] = false
	_, err = tool.Execute(context.Background(), params)
	if err == nil || err.Error() == "缺少必需参数: project_name" {
		t.Errorf("unexpected result for include_transitive=false: %v", err)
	}
}

package tools

import (
	"context"
	"encoding/json"
	"testing"
)

func TestBlockLogAnalyzerTool_Parameters(t *testing.T) {
	tool := NewBlockLogAnalyzerTool(nil)

	if tool.Name() != "block_log_analyzer" {
		t.Errorf("expected name 'block_log_analyzer', got '%s'", tool.Name())
	}

	if tool.Description() == "" {
		t.Error("expected non-empty description")
	}

	params := tool.Parameters()
	if len(params) == 0 {
		t.Fatal("expected non-empty parameters")
	}

	var schema map[string]interface{}
	if err := json.Unmarshal(params, &schema); err != nil {
		t.Fatalf("failed to parse parameters schema: %v", err)
	}

	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("expected properties in schema")
	}

	if _, ok := properties["analysis_type"]; !ok {
		t.Error("expected 'analysis_type' parameter")
	}

	if _, ok := properties["hours"]; !ok {
		t.Error("expected 'hours' parameter")
	}

	if _, ok := properties["threshold"]; !ok {
		t.Error("expected 'threshold' parameter")
	}
}

func TestBlockLogAnalyzerTool_InvalidAnalysisType(t *testing.T) {
	tool := NewBlockLogAnalyzerTool(nil)

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"analysis_type": "invalid_type",
	})

	if err == nil {
		t.Fatal("expected error for invalid analysis type")
	}

	expected := "不支持的分析类型: invalid_type"
	if err.Error() != expected {
		t.Errorf("expected error '%s', got '%s'", expected, err.Error())
	}
}

func TestBlockLogAnalyzerTool_MissingAnalysisType(t *testing.T) {
	tool := NewBlockLogAnalyzerTool(nil)

	_, err := tool.Execute(context.Background(), map[string]interface{}{})

	if err == nil {
		t.Fatal("expected error for missing analysis_type")
	}

	expected := "缺少必需参数: analysis_type"
	if err.Error() != expected {
		t.Errorf("expected error '%s', got '%s'", expected, err.Error())
	}
}

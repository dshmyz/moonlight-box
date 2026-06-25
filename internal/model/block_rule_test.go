package model

import (
	"reflect"
	"testing"
)

// TestBlockRuleConditionFields 验证 BlockRule 结构体新增的条件阻断字段及其 tag
func TestBlockRuleConditionFields(t *testing.T) {
	t.Run("ConditionType 字段存在且 tag 正确", func(t *testing.T) {
		typ := reflect.TypeOf(BlockRule{})
		field, ok := typ.FieldByName("ConditionType")
		if !ok {
			t.Fatalf("BlockRule 结构体缺少 ConditionType 字段")
		}
		if got := field.Tag.Get("gorm"); got != "size:30" {
			t.Errorf("ConditionType 的 gorm tag = %q, 期望 %q", got, "size:30")
		}
		if got := field.Tag.Get("json"); got != "condition_type" {
			t.Errorf("ConditionType 的 json tag = %q, 期望 %q", got, "condition_type")
		}
	})

	t.Run("ConditionOp 字段存在且 tag 正确", func(t *testing.T) {
		typ := reflect.TypeOf(BlockRule{})
		field, ok := typ.FieldByName("ConditionOp")
		if !ok {
			t.Fatalf("BlockRule 结构体缺少 ConditionOp 字段")
		}
		if got := field.Tag.Get("gorm"); got != "size:20" {
			t.Errorf("ConditionOp 的 gorm tag = %q, 期望 %q", got, "size:20")
		}
		if got := field.Tag.Get("json"); got != "condition_op" {
			t.Errorf("ConditionOp 的 json tag = %q, 期望 %q", got, "condition_op")
		}
	})

	t.Run("ConditionValue 字段存在且 tag 正确", func(t *testing.T) {
		typ := reflect.TypeOf(BlockRule{})
		field, ok := typ.FieldByName("ConditionValue")
		if !ok {
			t.Fatalf("BlockRule 结构体缺少 ConditionValue 字段")
		}
		if got := field.Tag.Get("gorm"); got != "size:500" {
			t.Errorf("ConditionValue 的 gorm tag = %q, 期望 %q", got, "size:500")
		}
		if got := field.Tag.Get("json"); got != "condition_value" {
			t.Errorf("ConditionValue 的 json tag = %q, 期望 %q", got, "condition_value")
		}
	})
}

// TestBlockRuleConditionTypeConstants 验证 ConditionType 常量
func TestBlockRuleConditionTypeConstants(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ConditionTypeLicense", string(ConditionTypeLicense), "license"},
		{"ConditionTypePublishTime", string(ConditionTypePublishTime), "publish_time"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("%s = %q, 期望 %q", c.name, c.got, c.want)
			}
		})
	}
}

// TestBlockRuleConditionOpConstants 验证 ConditionOp 常量
func TestBlockRuleConditionOpConstants(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ConditionOpEquals", string(ConditionOpEquals), "equals"},
		{"ConditionOpContains", string(ConditionOpContains), "contains"},
		{"ConditionOpBefore", string(ConditionOpBefore), "before"},
		{"ConditionOpAfter", string(ConditionOpAfter), "after"},
		{"ConditionOpWithinLast", string(ConditionOpWithinLast), "within_last"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("%s = %q, 期望 %q", c.name, c.got, c.want)
			}
		})
	}
}

// TestBlockRuleDefaultConditionTypeEmpty 验证默认创建的 BlockRule 的 ConditionType 为空字符串
func TestBlockRuleDefaultConditionTypeEmpty(t *testing.T) {
	rule := BlockRule{
		PackageName: "test-pkg",
		Version:     "1.0.0",
		MatchType:   BlockMatchExact,
		PackageType: "npm",
	}
	if rule.ConditionType != "" {
		t.Errorf("默认 BlockRule.ConditionType = %q, 期望空字符串", rule.ConditionType)
	}
	if rule.ConditionOp != "" {
		t.Errorf("默认 BlockRule.ConditionOp = %q, 期望空字符串", rule.ConditionOp)
	}
	if rule.ConditionValue != "" {
		t.Errorf("默认 BlockRule.ConditionValue = %q, 期望空字符串", rule.ConditionValue)
	}
}

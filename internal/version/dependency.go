package version

import (
	"strings"

	"github.com/Masterminds/semver/v3"
)

// IsFamily 判断风险版本是否为版本族（1.2.x / 1.2 / 1.2*），版本族不做精确门控。
func IsFamily(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return false
	}
	if strings.IndexAny(v, "xX*") >= 0 {
		return true
	}
	segments := strings.Split(v, ".")
	if len(segments) <= 2 {
		for _, s := range segments {
			if !allDigits(s) {
				return false
			}
		}
		return true
	}
	return false
}

// Matches 判断实际版本是否命中风险版本（精确或版本族/前缀）。
// 比较前剥离 Go 模块的 v 前缀（v1.2.3），使清单按常见形式填写也能命中 Go 制品。
func Matches(risky, actual string) bool {
	risky = strings.TrimSpace(risky)
	actual = strings.TrimSpace(actual)
	risky = trimV(risky)
	actual = trimV(actual)
	if risky == "" || actual == "" {
		return false
	}
	if idx := strings.IndexAny(risky, "xX*"); idx >= 0 {
		// 裸 x/* 视为匹配全部版本
		if idx == 0 {
			return true
		}
		prefix := strings.TrimRight(risky[:idx], ".") + "."
		base := strings.TrimSuffix(prefix, ".")
		// 1.2.x 同时命中基础版 1.2 与其下所有小版本
		return actual == base || strings.HasPrefix(actual, prefix)
	}
	segments := strings.Split(risky, ".")
	if len(segments) <= 2 && allDigitsIn(segments) {
		return actual == risky || strings.HasPrefix(actual, risky+".")
	}
	return actual == risky
}

// trimV 剥离 Go 模块版本的前导 v（仅当后接数字，避免影响普通版本串）。
func trimV(s string) string {
	if len(s) >= 2 && s[0] == 'v' && s[1] >= '0' && s[1] <= '9' {
		return s[1:]
	}
	return s
}

// ConstraintCovers 判断依赖约束（semver 范围）是否覆盖目标版本。
func ConstraintCovers(constraint, target string) bool {
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return false
	}
	v, err := semver.NewVersion(target)
	if err != nil {
		return false
	}
	return c.Check(v)
}

// ConstraintParseable 判断依赖约束能否被 semver 解析。
// 返回 false 表示约束属于协议型（file:/link:/workspace:*/git）或非 semver 语法（如 maven 硬区间），
// 无法判定覆盖关系时调用方应按"潜在命中"处理。
func ConstraintParseable(constraint string) bool {
	_, err := semver.NewConstraint(constraint)
	return err == nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func allDigitsIn(segments []string) bool {
	for _, s := range segments {
		if !allDigits(s) {
			return false
		}
	}
	return true
}

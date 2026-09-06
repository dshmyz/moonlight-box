package npm

import (
	"net/http"
	"testing"

	"github.com/dshmyz/moonlight-box/internal/version"
)

func TestResolveDependencies_NonSemverConstraint(t *testing.T) {
	p := NewNpmPlugin(&http.Client{})
	attrs := map[string]string{
		"dependencies": `{"react":"file:../react","lodash":"^4.17.21"}`,
	}

	// 不可解析约束（file:）应按潜在命中返回，不能漏报
	deps := p.ResolveDependencies(attrs, "react", "18.2.0")
	if len(deps) != 1 || deps[0] != "file:../react" {
		t.Errorf("react file: 约束应命中, got %v", deps)
	}

	// 可解析但不覆盖 → 不命中
	deps = p.ResolveDependencies(attrs, "lodash", "4.16.0")
	if len(deps) != 0 {
		t.Errorf("lodash 4.16.0 不应命中 ^4.17.21, got %v", deps)
	}

	// 可解析且覆盖 → 命中
	deps = p.ResolveDependencies(attrs, "lodash", "4.17.21")
	if len(deps) != 1 || deps[0] != "^4.17.21" {
		t.Errorf("lodash 4.17.21 应命中 ^4.17.21, got %v", deps)
	}
}

func TestVersionConstraintParseable(t *testing.T) {
	if !version.ConstraintParseable("^1.2.3") {
		t.Error("^1.2.3 should be parseable")
	}
	if version.ConstraintParseable("file:../react") {
		t.Error("file: 约束不应被 semver 解析")
	}
	if version.ConstraintParseable("[1.0,2.0)") {
		t.Error("maven 硬区间不应被 semver 解析")
	}
}

package maven

import "testing"

func TestMavenDeclaredCovers_RangeAndNonSemver(t *testing.T) {
	cases := []struct {
		declared, risky string
		want            bool
	}{
		{"[1.2.0,1.4.0)", "1.3.0", true}, // 硬区间无法判断 → 潜在命中
		{"[1.2.0,1.4.0)", "2.0.0", true},  // 硬区间无法判断 → 潜在命中（保守）
		{"1.0.0.Final", "1.0.0.Final", true},
		{"5.3.0", "5.3.1", false}, // 可解析且不覆盖 → 不命中
		{"5.3.0", "5.3.0", true},
		{"${project.version}", "1.0.0", true}, // 占位符 → 潜在命中
		{"LATEST", "1.0.0", true},
		{"2.14.1", "2.14.x", true}, // 风险为版本族 → 不精确门控
	}
	for _, c := range cases {
		if got := mavenDeclaredCovers(c.declared, c.risky); got != c.want {
			t.Errorf("mavenDeclaredCovers(%q,%q)=%v want %v", c.declared, c.risky, got, c.want)
		}
	}
}

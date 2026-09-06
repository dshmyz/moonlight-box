package util

import (
	"fmt"
	"time"
)

// CompactDuration 把 duration 规范化为简洁字符串：24h、1h30m、45m；
// 亚分钟精度时保留默认 String()。全项目统一使用，避免多处实现输出不一致。
func CompactDuration(d time.Duration) string {
	if d < time.Minute || d%time.Minute != 0 {
		return d.String()
	}
	h := int(d / time.Hour)
	m := int((d % time.Hour) / time.Minute)
	switch {
	case h == 0:
		return fmt.Sprintf("%dm", m)
	case m == 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dh%dm", h, m)
	}
}

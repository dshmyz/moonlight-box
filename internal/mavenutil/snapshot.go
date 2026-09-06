package mavenutil

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// SnapshotBuild 表示一个 Maven SNAPSHOT 构建的时间戳和构建号。
type SnapshotBuild struct {
	BaseVersion string    // 逻辑版本（去掉 -SNAPSHOT 或时间戳），如 "1.0"
	Timestamp   string    // 如 "20260603.033633"
	BuildNum    int       // 构建序号
	TimestampT  time.Time // 解析后的时间
}

// timestampedVersionRe 匹配时间戳版本形式：1.0-20260603.033633-1
var timestampedVersionRe = regexp.MustCompile(`^(.*)-(\d{8}\.\d{6})-(\d+)$`)

// ParseSnapshotBuild 从 SNAPSHOT 版本/文件名中提取基础版本、时间戳和构建号。
//
// name: Maven artifact name，格式 "group:artifact"（即 model.Artifact.Name）
// version: 版本号，支持两种存储形式：
//   - "1.0-SNAPSHOT"（基础版本）
//   - "1.0-20260603.033633-1"（时间戳版本，代理按时间戳路径缓存时可能使用）
//
// filename: 文件名，如 "my-lib-1.0-20260603.033633-1.jar"
//
// filename 格式: {artifactId}-{baseVersion}-{YYYYMMDD}.{HHMMSS}-{buildNum}[-classifier][.ext]
func ParseSnapshotBuild(name, version, filename string) (SnapshotBuild, bool) {
	var baseVersion string
	switch {
	case strings.HasSuffix(version, "-SNAPSHOT"):
		baseVersion = strings.TrimSuffix(version, "-SNAPSHOT")
	default:
		m := timestampedVersionRe.FindStringSubmatch(version)
		if m == nil {
			return SnapshotBuild{}, false
		}
		baseVersion = m[1]
	}

	// name 存的是 "group:artifact"，提取 artifactId
	artifact := name
	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		artifact = name[idx+1:]
	}

	prefix := artifact + "-" + baseVersion + "-"
	rest := strings.TrimPrefix(filename, prefix)
	if rest == filename {
		return SnapshotBuild{}, false
	}

	parts := strings.SplitN(rest, "-", 3)
	if len(parts) < 2 {
		return SnapshotBuild{}, false
	}

	ts := parts[0] // "20260603.033633"
	buildStr := parts[1]
	if dotIdx := strings.IndexByte(buildStr, '.'); dotIdx >= 0 {
		buildStr = buildStr[:dotIdx]
	}

	buildNum, err := strconv.Atoi(buildStr)
	if err != nil {
		return SnapshotBuild{}, false
	}

	t, err := time.Parse("20060102.150405", ts)
	if err != nil {
		return SnapshotBuild{}, false
	}

	return SnapshotBuild{
		BaseVersion: baseVersion,
		Timestamp:   ts,
		BuildNum:    buildNum,
		TimestampT:  t,
	}, true
}

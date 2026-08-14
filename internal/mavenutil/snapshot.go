package mavenutil

import (
	"strconv"
	"strings"
	"time"
)

// SnapshotBuild 表示一个 Maven SNAPSHOT 构建的时间戳和构建号。
type SnapshotBuild struct {
	Timestamp  string    // 如 "20260603.033633"
	BuildNum   int       // 构建序号
	TimestampT time.Time // 解析后的时间
}

// ParseSnapshotBuild 从 SNAPSHOT 文件名中提取时间戳和构建号。
//
// name: Maven artifact name，格式 "group:artifact"（即 model.Artifact.Name）
// version: 版本号，如 "1.0-SNAPSHOT"
// filename: 文件名，如 "my-lib-1.0-20260603.033633-1.jar"
//
// filename 格式: {artifactId}-{baseVersion}-{YYYYMMDD}.{HHMMSS}-{buildNum}[-classifier][.ext]
func ParseSnapshotBuild(name, version, filename string) (SnapshotBuild, bool) {
	if !strings.HasSuffix(version, "-SNAPSHOT") {
		return SnapshotBuild{}, false
	}

	// name 存的是 "group:artifact"，提取 artifactId
	artifact := name
	if idx := strings.LastIndex(name, ":"); idx >= 0 {
		artifact = name[idx+1:]
	}

	baseVersion := strings.TrimSuffix(version, "-SNAPSHOT")
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
		Timestamp:  ts,
		BuildNum:   buildNum,
		TimestampT: t,
	}, true
}

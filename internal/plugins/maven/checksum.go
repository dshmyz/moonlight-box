package maven

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
)

// checksumAlgo 表示 Maven checksum 算法类型
type checksumAlgo string

const (
	checksumSHA1   checksumAlgo = "sha1"
	checksumMD5    checksumAlgo = "md5"
	checksumSHA256 checksumAlgo = "sha256"
)

// parseChecksumRequest 检测文件名是否为 checksum 文件请求。
// 如果是，返回原始文件名、算法类型和 true；否则返回 false。
//
// Maven checksum 文件命名规则：
//   - my-lib-1.0.0.jar.sha1   → 原始文件: my-lib-1.0.0.jar, 算法: sha1
//   - my-lib-1.0.0.jar.md5    → 原始文件: my-lib-1.0.0.jar, 算法: md5
//   - my-lib-1.0.0.jar.sha256 → 原始文件: my-lib-1.0.0.jar, 算法: sha256
func parseChecksumRequest(filename string) (originalFile string, algo checksumAlgo, ok bool) {
	if strings.HasSuffix(filename, ".sha256") {
		return strings.TrimSuffix(filename, ".sha256"), checksumSHA256, true
	}
	if strings.HasSuffix(filename, ".sha1") {
		return strings.TrimSuffix(filename, ".sha1"), checksumSHA1, true
	}
	if strings.HasSuffix(filename, ".md5") {
		return strings.TrimSuffix(filename, ".md5"), checksumMD5, true
	}
	return "", "", false
}

// computeChecksum 计算 reader 内容的指定算法 checksum，返回小写十六进制字符串。
func computeChecksum(reader io.Reader, algo checksumAlgo) (string, error) {
	var h io.Writer
	switch algo {
	case checksumSHA1:
		h = sha1.New()
	case checksumMD5:
		h = md5.New()
	case checksumSHA256:
		h = sha256.New()
	default:
		h = sha1.New()
	}

	if _, err := io.Copy(h, reader); err != nil {
		return "", err
	}

	hasher := h.(interface{ Sum(b []byte) []byte })
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// formatMavenChecksum 格式化为 Maven 标准 checksum 格式。
// 格式: "<hex_digest>  <filename>\n"
// 两个空格分隔 digest 和 filename，这是 Maven 的标准约定。
func formatMavenChecksum(digest, filename string) string {
	return digest + "  " + filename + "\n"
}

package config

import (
	"fmt"
	"strings"
)

// insecureJWTSecrets 是已知的不安全默认/示例密钥，生产（release）环境禁止使用。
var insecureJWTSecrets = map[string]bool{
	"":                                          true,
	"change-me-in-production":                   true,
	"change-me-in-production-use-random-string": true,
	"test-secret-key-for-development-only":      true,
}

// minJWTSecretLen 是 JWT 密钥的最小长度要求。
const minJWTSecretLen = 32

// Validate 校验配置的生产安全性。
//
// 返回的 fatal 为致命错误（调用方应拒绝启动）；warnings 为非致命提醒。
// 校验策略按 server.mode 区分：release 模式从严（致命），其余模式仅警告，
// 以便本地开发使用默认值时不被阻断。
func (c *Config) Validate() (warnings []string, fatal error) {
	isRelease := strings.EqualFold(c.Server.Mode, "release")

	secret := strings.TrimSpace(c.Auth.JWTSecret)
	if insecureJWTSecrets[secret] || len(secret) < minJWTSecretLen {
		msg := fmt.Sprintf(
			"auth.jwt_secret 不安全（为空、使用默认示例值或长度 < %d）；"+
				"请通过环境变量 MOONLIGHT_AUTH_JWT_SECRET 或配置文件设置一个强随机密钥",
			minJWTSecretLen,
		)
		if isRelease {
			return warnings, fmt.Errorf("%s", msg)
		}
		warnings = append(warnings, msg+"（当前为非 release 模式，仅警告）")
	}

	// release 模式下不应加载测试数据
	if isRelease && c.Seed.LoadTestData {
		warnings = append(warnings, "release 模式下 seed.load_test_data=true，将写入测试包数据，建议设为 false")
	}

	// release 模式下 Gin debug 会暴露堆栈并降低性能（理论上不会走到这里，mode 已是 release，仅防御性提示）
	if isRelease && strings.EqualFold(c.Logging.Level, "debug") {
		warnings = append(warnings, "release 模式下 logging.level=debug，日志量大且可能泄露敏感信息，建议设为 info")
	}

	return warnings, nil
}

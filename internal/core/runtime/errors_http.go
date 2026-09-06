package runtime

import (
	"errors"
	"net/http"
)

// WritePolicyError 把仓库策略/查找类哨兵错误映射为 HTTP 响应，集中维护状态码与文案，
// 供各插件上传/删除处理器复用（避免 7 处内联复制的文案漂移）。
// 已处理（写入了响应）返回 true；未知错误返回 false，由调用方走默认 500 路径。
func WritePolicyError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, ErrOverwriteNotAllowed):
		http.Error(w, "artifact already exists, overwrite not allowed", http.StatusConflict)
	case errors.Is(err, ErrDeleteNotAllowed):
		http.Error(w, "Repository does not allow delete", http.StatusForbidden)
	case errors.Is(err, ErrReadOnly):
		http.Error(w, "Repository is read only", http.StatusMethodNotAllowed)
	case errors.Is(err, ErrNotFound):
		http.Error(w, "Not found", http.StatusNotFound)
	default:
		return false
	}
	return true
}

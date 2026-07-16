package runtime

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// getRealClientIP 获取客户端真实IP地址
// 优先检查 X-Forwarded-For 头，然后检查 X-Real-IP 头，最后使用 RemoteAddr
func getRealClientIP(req *http.Request) string {
	// X-Forwarded-For: client, proxy1, proxy2
	// 取第一个非空IP
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		for _, ip := range ips {
			ip = strings.TrimSpace(ip)
			if ip != "" {
				return ip
			}
		}
	}

	// X-Real-IP
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// RemoteAddr (host:port format)
	addr := req.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

type Nexus3Resolver struct{}

func (r *Nexus3Resolver) Resolve(req *http.Request) (*ResolvedRepository, error) {
	path := req.URL.Path
	if len(path) < 12 || path[:12] != "/repository/" {
		return nil, ErrNotMatched
	}
	remaining := path[12:]
	idx := len(remaining) // default: entire string is repo name
	for i, c := range remaining {
		if c == '/' {
			idx = i
			break
		}
	}
	repoName := remaining[:idx]
	remainingPath := remaining[idx:]
	if remainingPath == "" {
		remainingPath = "/"
	}
	return &ResolvedRepository{
		Repository:    &Repository{Name: repoName},
		RemainingPath: remainingPath,
		RouteStyle:    Nexus3Route,
	}, nil
}

// Nexus2Resolver 解析 Nexus 2 风格路径，支持仓库和 group（virtual）路由
type Nexus2Resolver struct {
	prefix string
}

func NewNexus2RepoResolver() *Nexus2Resolver {
	return &Nexus2Resolver{prefix: "/content/repositories/"}
}

func NewNexus2GroupResolver() *Nexus2Resolver {
	return &Nexus2Resolver{prefix: "/content/groups/"}
}

func (r *Nexus2Resolver) Resolve(req *http.Request) (*ResolvedRepository, error) {
	path := req.URL.Path
	prefixLen := len(r.prefix)
	if len(path) < prefixLen || path[:prefixLen] != r.prefix {
		return nil, ErrNotMatched
	}
	remaining := path[prefixLen:]
	idx := len(remaining)
	for i, c := range remaining {
		if c == '/' {
			idx = i
			break
		}
	}
	repoName := remaining[:idx]
	remainingPath := remaining[idx:]
	if remainingPath == "" {
		remainingPath = "/"
	}
	return &ResolvedRepository{
		Repository:    &Repository{Name: repoName},
		RemainingPath: remainingPath,
		RouteStyle:    Nexus2Route,
	}, nil
}

type CompositeResolver struct {
	Resolvers []RepositoryPathResolver
}

func (r *CompositeResolver) Resolve(req *http.Request) (*ResolvedRepository, error) {
	for _, resolver := range r.Resolvers {
		result, err := resolver.Resolve(req)
		if err == nil {
			return result, nil
		}
	}
	return nil, ErrNotMatched
}

type RepositoryRouter struct {
	Resolver      RepositoryPathResolver
	Manager       RepositoryManager
	Plugins       map[string]ProtocolPlugin
	Blocker       PackageBlocker
	AuditLog      AuditLogger
	DownloadCount DownloadCounter
	ProxyLog      DownloadLogger
}

func (r *RepositoryRouter) logBlock(ctx context.Context, format, name, ip, userAgent string) {
	if r.AuditLog == nil {
		return
	}
	r.AuditLog.Log(ctx, AuditEntry{
		Action:         "block",
		ResourceType:   format,
		ResourceName:   name,
		IPAddress:      ip,
		UserAgent:      userAgent,
		ResponseStatus: http.StatusForbidden,
	})
}

// DownloadCounter 下载计数器接口
type DownloadCounter interface {
	IncrementDownload(repoID uint, format, name, version string)
}

// DownloadLogParams 下载日志参数
type DownloadLogParams struct {
	RepoID      uint
	PackageType string
	PackageName string
	Version     string
	Filename    string
	StatusCode  int
	SizeBytes   int64
	FromCache   bool
	RemoteURL   string
	ClientIP    string
	UserAgent   string
	RequestID   string
}

// DownloadLogger 下载日志接口
type DownloadLogger interface {
	LogDownload(params DownloadLogParams)
}

// statusRecorder 包装 http.ResponseWriter，捕获实际写入的状态码
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.written {
		r.statusCode = code
		r.written = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.statusCode = http.StatusOK
		r.written = true
	}
	return r.ResponseWriter.Write(b)
}

func (r *statusRecorder) StatusCode() int {
	if r.statusCode == 0 {
		return http.StatusOK
	}
	return r.statusCode
}

func (r *RepositoryRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resolved, err := r.Resolver.Resolve(req)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"path":  req.URL.Path,
			"error": err.Error(),
		}).Warn("router: failed to resolve path")
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}
	repo := r.Manager.Get(resolved.Repository.Name)
	if repo == nil {
		logrus.WithFields(logrus.Fields{
			"path":          req.URL.Path,
			"repoName":      resolved.Repository.Name,
			"remainingPath": resolved.RemainingPath,
		}).Warn("router: repository not found in manager")
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	logrus.WithFields(logrus.Fields{
		"path":          req.URL.Path,
		"repoName":      repo.Name,
		"repoFormat":    repo.Format,
		"repoType":      repo.Type,
		"hasRuntime":    repo.Runtime != nil,
		"remainingPath": resolved.RemainingPath,
	}).Debug("router: request resolved")

	// Block rule check — URL 路径匹配，去除前导 /
	blockPath := resolved.RemainingPath
	if len(blockPath) > 0 && blockPath[0] == '/' {
		blockPath = blockPath[1:]
	}
	if r.Blocker != nil && r.Blocker.IsBlocked(repo.Format, blockPath, "*") {
		reason := r.Blocker.BlockReason(repo.Format, blockPath, "*")
		r.logBlock(req.Context(), repo.Format, blockPath, getRealClientIP(req), req.UserAgent())
		http.Error(w, "Blocked: "+reason, http.StatusForbidden)
		return
	}

	clientIP := getRealClientIP(req)
	rec := &statusRecorder{ResponseWriter: w}
	ctx := &RequestContext{
		Writer:         rec,
		Request:        req.WithContext(ContextWithClientIP(req.Context(), clientIP)),
		Repository:     repo,
		RepositoryPath: resolved.RemainingPath,
		RouteStyle:     resolved.RouteStyle,
		Blocker:        r.Blocker,
	}
	r.handleRequest(ctx)

	// 只对 GET 请求记录下载日志，且从 RequestContext 提取真实数据
	if req.Method != http.MethodGet {
		return
	}

	// 解析 repoID
	var repoID uint
	if id, parseErr := strconv.ParseUint(repo.ID, 10, 64); parseErr == nil {
		repoID = uint(id)
	}

	// 从路径中提取包名和版本信息（由 ProtocolPlugin 在 Handle 中填充到 ctx）
	packageName := ctx.PackageName
	version := ctx.Version
	filename := ctx.Filename

	// 获取实际响应状态码：优先使用包装器捕获的真实状态码
	statusCode := rec.StatusCode()

	// 下载计数 — 只在成功时计数
	if r.DownloadCount != nil && statusCode >= 200 && statusCode < 400 {
		r.DownloadCount.IncrementDownload(repoID, repo.Format, packageName, version)
	}

	// 下载日志（替代 AuditLog 记录下载操作）
	if r.ProxyLog != nil {
		r.ProxyLog.LogDownload(DownloadLogParams{
			RepoID:      repoID,
			PackageType: repo.Format,
			PackageName: packageName,
			Version:     version,
			Filename:    filename,
			StatusCode:  statusCode,
			SizeBytes:   ctx.SizeBytes,
			FromCache:   ctx.FromCache,
			RemoteURL:   ctx.RemoteURL,
			ClientIP:    clientIP,
			UserAgent:   req.UserAgent(),
			RequestID:   req.Header.Get("X-Request-ID"),
		})
	}
}

func NewRepositoryRouter(resolver RepositoryPathResolver, manager RepositoryManager) *RepositoryRouter {
	return &RepositoryRouter{
		Resolver: resolver,
		Manager:  manager,
		Plugins:  make(map[string]ProtocolPlugin),
	}
}

func (r *RepositoryRouter) RegisterPlugin(format string, plugin ProtocolPlugin) {
	r.Plugins[format] = plugin
}

func (r *RepositoryRouter) handleRequest(ctx *RequestContext) {
	plugin, ok := r.Plugins[ctx.Repository.Format]
	if !ok {
		logrus.WithFields(logrus.Fields{
			"repoName":   ctx.Repository.Name,
			"repoFormat": ctx.Repository.Format,
			"availablePlugins": func() []string {
				var keys []string
				for k := range r.Plugins {
					keys = append(keys, k)
				}
				return keys
			}(),
		}).Error("router: no plugin registered for format")
		ctx.StatusCode = http.StatusBadRequest
		http.Error(ctx.Writer, "Unsupported format", http.StatusBadRequest)
		return
	}

	runtime := ctx.Repository.Runtime
	if runtime == nil {
		logrus.WithFields(logrus.Fields{
			"repoName":   ctx.Repository.Name,
			"repoFormat": ctx.Repository.Format,
			"repoType":   ctx.Repository.Type,
		}).Error("router: repository runtime is nil")
		ctx.StatusCode = http.StatusInternalServerError
		http.Error(ctx.Writer, "Repository runtime not available", http.StatusInternalServerError)
		return
	}

	logrus.WithFields(logrus.Fields{
		"repoName":      ctx.Repository.Name,
		"repoFormat":    ctx.Repository.Format,
		"remainingPath": ctx.RepositoryPath,
		"method":        ctx.Request.Method,
	}).Debug("router: calling plugin.Handle")

	if err := plugin.Handle(ctx, runtime); err != nil {
		if err == ErrBlocked {
			packageName := ctx.PackageName
			if packageName == "" {
				packageName = ctx.RepositoryPath
			}
			r.logBlock(ctx.Request.Context(), ctx.Repository.Format, packageName, getRealClientIP(ctx.Request), ctx.Request.UserAgent())
			ctx.StatusCode = http.StatusForbidden
			http.Error(ctx.Writer, "Blocked: package is blocked by rule", http.StatusForbidden)
			return
		}
		if err == ErrNotFound {
			ctx.StatusCode = http.StatusNotFound
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
			return
		}
		ctx.StatusCode = http.StatusInternalServerError
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
	}
}

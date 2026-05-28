package runtime

import (
	"net/http"
	"strconv"
	"strings"
)

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

type Nexus2Resolver struct{}

func (r *Nexus2Resolver) Resolve(req *http.Request) (*ResolvedRepository, error) {
	path := req.URL.Path
	if len(path) < 22 || path[:22] != "/content/repositories/" {
		return nil, ErrNotMatched
	}
	remaining := path[22:]
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

type Nexus2GroupResolver struct{}

func (r *Nexus2GroupResolver) Resolve(req *http.Request) (*ResolvedRepository, error) {
	path := req.URL.Path
	if len(path) < 16 || path[:16] != "/content/groups/" {
		return nil, ErrNotMatched
	}
	remaining := path[16:]
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
	ProxyLog      ProxyDownloadLogger
}

// DownloadCounter 下载计数器接口
type DownloadCounter interface {
	IncrementDownload(repoID uint, format, name, version string)
}

// ProxyDownloadLogger 代理下载日志接口
type ProxyDownloadLogger interface {
	LogDownload(repoID uint, packageType, packageName, version, filename string, statusCode int, sizeBytes int64, fromCache bool)
}

func (r *RepositoryRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	resolved, err := r.Resolver.Resolve(req)
	if err != nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}
	repo := r.Manager.Get(resolved.Repository.Name)
	if repo == nil {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Block rule check — URL 路径匹配，去除前导 /
	blockPath := resolved.RemainingPath
	if len(blockPath) > 0 && blockPath[0] == '/' {
		blockPath = blockPath[1:]
	}
	if r.Blocker != nil && r.Blocker.IsBlocked(repo.Format, blockPath, "*") {
		reason := r.Blocker.BlockReason(repo.Format, blockPath, "*")
		// 审计日志：被阻断的请求
		if r.AuditLog != nil {
			r.AuditLog.Log(req.Context(), AuditEntry{
				Action:         "download_blocked",
				ResourceType:   repo.Format,
				ResourceName:   blockPath,
				IPAddress:      req.RemoteAddr,
				UserAgent:      req.UserAgent(),
				ResponseStatus: http.StatusForbidden,
			})
		}
		http.Error(w, "Blocked: "+reason, http.StatusForbidden)
		return
	}

	ctx := &RequestContext{
		Writer:         w,
		Request:        req,
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

	// 从路径中提取包名和版本信息
	packageName, version, filename := extractPackageInfo(repo.Format, blockPath)

	// 获取实际响应状态码
	statusCode := ctx.StatusCode
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	// 下载计数 — 只在成功时计数
	if r.DownloadCount != nil && statusCode >= 200 && statusCode < 400 {
		r.DownloadCount.IncrementDownload(repoID, repo.Format, packageName, version)
	}

	// 代理下载日志
	if r.ProxyLog != nil {
		r.ProxyLog.LogDownload(repoID, repo.Format, packageName, version, filename, statusCode, 0, false)
	}

	// Audit log for downloads
	if r.AuditLog != nil {
		r.AuditLog.Log(req.Context(), AuditEntry{
			Action:         "download",
			ResourceType:   repo.Format,
			ResourceName:   blockPath,
			IPAddress:      req.RemoteAddr,
			UserAgent:      req.UserAgent(),
			ResponseStatus: statusCode,
		})
	}
}

// extractPackageInfo 从路径中提取包名、版本和文件名
func extractPackageInfo(format, path string) (packageName, version, filename string) {
	if path == "" || path == "/" {
		return "", "", ""
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	switch format {
	case "maven":
		// {group...}/{artifact}/{version}/{filename}
		if len(parts) >= 4 {
			filename = parts[len(parts)-1]
			version = parts[len(parts)-2]
			packageName = strings.Join(parts[:len(parts)-3], ".") + ":" + parts[len(parts)-3]
		} else if len(parts) >= 2 {
			packageName = parts[len(parts)-1]
		}
	case "npm":
		if len(parts) >= 2 && strings.HasPrefix(parts[0], "@") {
			packageName = parts[0] + "/" + parts[1] // scoped: @scope/pkg
		} else if len(parts) >= 1 {
			packageName = parts[0]
		}
		if strings.Contains(path, "/-/") {
			slashParts := strings.SplitN(path, "/-/", 2)
			if len(slashParts) == 2 {
				packageName = strings.Trim(slashParts[0], "/")
				filename = slashParts[1]
			}
		}
	case "pypi":
		if len(parts) >= 3 && parts[0] == "simple" {
			packageName = parts[1]
		} else if len(parts) >= 2 && parts[0] == "packages" {
			filename = parts[len(parts)-1]
		}
	case "go":
		if len(parts) >= 1 {
			packageName = parts[0]
		}
	default:
		if len(parts) >= 1 {
			filename = parts[len(parts)-1]
		}
	}
	return
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
		ctx.StatusCode = http.StatusBadRequest
		http.Error(ctx.Writer, "Unsupported format", http.StatusBadRequest)
		return
	}

	runtime := ctx.Repository.Runtime
	if runtime == nil {
		ctx.StatusCode = http.StatusInternalServerError
		http.Error(ctx.Writer, "Repository runtime not available", http.StatusInternalServerError)
		return
	}

	if err := plugin.Handle(ctx, runtime); err != nil {
		if err == ErrBlocked {
			ctx.StatusCode = http.StatusForbidden
			http.Error(ctx.Writer, "Blocked: package is blocked by rule", http.StatusForbidden)
			return
		}
		if err == ErrNotFound {
			ctx.StatusCode = http.StatusNotFound
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			ctx.StatusCode = http.StatusInternalServerError
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
	}
}

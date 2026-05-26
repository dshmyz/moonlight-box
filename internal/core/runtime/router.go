package runtime

import (
	"net/http"
)

type Nexus3Resolver struct{}

func (r *Nexus3Resolver) Resolve(req *http.Request) (*ResolvedRepository, error) {
	path := req.URL.Path
	if len(path) < 12 || path[:12] != "/repository/" {
		return nil, ErrNotMatched
	}
	remaining := path[12:]
	idx := 0
	for i, c := range remaining {
		if c == '/' {
			idx = i
			break
		}
	}
	repoName := remaining[:idx]
	remainingPath := remaining[idx:]
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
	idx := 0
	for i, c := range remaining {
		if c == '/' {
			idx = i
			break
		}
	}
	repoName := remaining[:idx]
	remainingPath := remaining[idx:]
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
	idx := 0
	for i, c := range remaining {
		if c == '/' {
			idx = i
			break
		}
	}
	repoName := remaining[:idx]
	remainingPath := remaining[idx:]
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

	// 下载计数 — 写 repositories.download_count
	if r.DownloadCount != nil && req.Method == http.MethodGet {
		r.DownloadCount.IncrementDownload(0, repo.Format, "", "")
	}

	// 代理下载日志
	if r.ProxyLog != nil && req.Method == http.MethodGet {
		r.ProxyLog.LogDownload(0, repo.Format, resolved.RemainingPath, "", "", http.StatusOK, 0, false)
	}

	// Audit log for downloads
	if r.AuditLog != nil && req.Method == http.MethodGet {
		r.AuditLog.Log(req.Context(), AuditEntry{
			Action:         "download",
			ResourceType:   repo.Format,
			ResourceName:   resolved.RemainingPath,
			IPAddress:      req.RemoteAddr,
			UserAgent:      req.UserAgent(),
			ResponseStatus: http.StatusOK,
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
		http.Error(ctx.Writer, "Unsupported format", http.StatusBadRequest)
		return
	}

	runtime := ctx.Repository.Runtime
	if runtime == nil {
		http.Error(ctx.Writer, "Repository runtime not available", http.StatusInternalServerError)
		return
	}

	if err := plugin.Handle(ctx, runtime); err != nil {
		if err == ErrBlocked {
			http.Error(ctx.Writer, "Blocked: package is blocked by rule", http.StatusForbidden)
			return
		}
		if err == ErrNotFound {
			http.Error(ctx.Writer, "Not found", http.StatusNotFound)
		} else {
			http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		}
	}
}

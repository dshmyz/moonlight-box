package handler

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/response"
	"github.com/moonlight-box/registry/internal/service"
)

type PublicRepoHandler struct {
	repoSvc *service.RepositoryService
}

func NewPublicRepoHandler(repoSvc *service.RepositoryService) *PublicRepoHandler {
	return &PublicRepoHandler{repoSvc: repoSvc}
}

type PublicRepoListItem struct {
	Name        string               `json:"name"`
	DisplayName string               `json:"display_name"`
	Description string               `json:"description"`
	Type        model.RepositoryType `json:"type"`
	PackageType string               `json:"package_type"`
	Enabled     bool                 `json:"enabled"`
	RemoteURL   string               `json:"remote_url,omitempty"`
	RegistryURL string               `json:"registry_url"`
}

func (h *PublicRepoHandler) List(c *gin.Context) {
	filter := map[string]interface{}{
		"public_visible": true,
		"enabled":        true,
	}
	if pkgType := c.Query("package_type"); pkgType != "" {
		filter["package_type"] = pkgType
	}
	if repoType := c.Query("type"); repoType != "" {
		filter["type"] = repoType
	}

	repos, err := h.repoSvc.ListContext(c.Request.Context(), filter)
	if err != nil {
		response.InternalError(c, "Failed to list public repositories")
		return
	}

	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := c.Request.Host
	prefix := strings.TrimRight(c.GetHeader("X-Forwarded-Prefix"), "/")
	if prefix == "" {
		prefix = strings.TrimRight(c.GetHeader("X-Script-Name"), "/")
	}
	baseURL := fmt.Sprintf("%s://%s%s", scheme, host, prefix)

	items := make([]PublicRepoListItem, len(repos))
	for i, repo := range repos {
		items[i] = PublicRepoListItem{
			Name:        repo.Name,
			DisplayName: repo.DisplayName,
			Description: repo.Description,
			Type:        repo.Type,
			PackageType: repo.PackageType,
			Enabled:     repo.Enabled,
			RemoteURL:   repo.RemoteURL,
			RegistryURL: buildRegistryURL(baseURL, &repo),
		}
	}

	response.Success(c, items)
}

type RepoConfigResponse struct {
	Name        string               `json:"name"`
	DisplayName string               `json:"display_name"`
	Description string               `json:"description"`
	Type        model.RepositoryType `json:"type"`
	PackageType string               `json:"package_type"`
	Enabled     bool                 `json:"enabled"`
	RemoteURL   string               `json:"remote_url,omitempty"`
	RegistryURL string               `json:"registry_url"`
	ConfigGuide []ConfigStep         `json:"config_guide"`
}

type ConfigStep struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Command     string `json:"command"`
	Language    string `json:"language"`
}

func (h *PublicRepoHandler) GetRepoConfig(c *gin.Context) {
	name := c.Param("name")

	repo, err := h.repoSvc.Get(name)
	if err != nil {
		response.NotFound(c, "仓库不存在")
		return
	}

	if !repo.Enabled {
		response.NotFound(c, "仓库已禁用")
		return
	}

	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := c.Request.Host
	prefix := strings.TrimRight(c.GetHeader("X-Forwarded-Prefix"), "/")
	if prefix == "" {
		prefix = strings.TrimRight(c.GetHeader("X-Script-Name"), "/")
	}
	baseURL := fmt.Sprintf("%s://%s%s", scheme, host, prefix)

	registryURL := buildRegistryURL(baseURL, repo)
	guide := buildConfigGuide(baseURL, repo)

	resp := RepoConfigResponse{
		Name:        repo.Name,
		DisplayName: repo.DisplayName,
		Description: repo.Description,
		Type:        repo.Type,
		PackageType: repo.PackageType,
		Enabled:     repo.Enabled,
		RemoteURL:   repo.RemoteURL,
		RegistryURL: registryURL,
		ConfigGuide: guide,
	}

	response.Success(c, resp)
}

func buildRegistryURL(baseURL string, repo *model.Repository) string {
	return fmt.Sprintf("%s/repository/%s/", baseURL, repo.Name)
}

func buildConfigGuide(baseURL string, repo *model.Repository) []ConfigStep {
	registryURL := buildRegistryURL(baseURL, repo)

	switch repo.PackageType {
	case "npm":
		return []ConfigStep{
			{
				Title:       "设置 npm registry",
				Description: "将 npm 指向此仓库",
				Command:     fmt.Sprintf("npm config set registry %s", registryURL),
				Language:    "bash",
			},
			{
				Title:       "使用 .npmrc 文件",
				Description: "在项目根目录创建或编辑 .npmrc 文件",
				Command:     fmt.Sprintf("registry=%s", registryURL),
				Language:    "ini",
			},
			{
				Title:       "恢复默认 registry",
				Description: "恢复为 npm 官方源",
				Command:     "npm config set registry https://registry.npmjs.org",
				Language:    "bash",
			},
		}

	case "maven":
		return []ConfigStep{
			{
				Title:       "配置 Maven settings.xml",
				Description: "在 ~/.m2/settings.xml 的 <mirrors> 中添加",
				Command: fmt.Sprintf(`<mirror>
  <id>%s</id>
  <mirrorOf>*</mirrorOf>
  <name>%s</name>
  <url>%s</url>
</mirror>`, repo.Name, repo.DisplayName, registryURL),
				Language: "xml",
			},
			{
				Title:       "或在 pom.xml 中配置仓库",
				Description: "在项目 pom.xml 的 <repositories> 中添加",
				Command: fmt.Sprintf(`<repository>
  <id>%s</id>
  <name>%s</name>
  <url>%s</url>
</repository>`, repo.Name, repo.DisplayName, registryURL),
				Language: "xml",
			},
		}

	case "pypi":
		return []ConfigStep{
			{
				Title:       "使用 pip 配置",
				Description: "设置 pip 全局 index-url",
				Command:     fmt.Sprintf("pip config set global.index-url %s", registryURL),
				Language:    "bash",
			},
			{
				Title:       "使用 pip.conf / pip.ini",
				Description: "在配置文件中添加",
				Command: fmt.Sprintf(`[global]
index-url = %s`, registryURL),
				Language: "ini",
			},
			{
				Title:       "临时使用（单次安装）",
				Description: "安装时指定源",
				Command:     fmt.Sprintf("pip install -i %s <package>", registryURL),
				Language:    "bash",
			},
		}

	case "go":
		return []ConfigStep{
			{
				Title:       "设置 GOPROXY",
				Description: "将 Go module proxy 指向此仓库",
				Command:     fmt.Sprintf("go env -w GOPROXY=%s,direct", registryURL),
				Language:    "bash",
			},
			{
				Title:       "临时使用（单次命令）",
				Description: "在命令中临时指定",
				Command:     fmt.Sprintf("GOPROXY=%s go mod tidy", registryURL),
				Language:    "bash",
			},
		}

	case "nuget":
		return []ConfigStep{
			{
				Title:       "添加 NuGet 源",
				Description: "使用 dotnet CLI 添加包源",
				Command:     fmt.Sprintf("dotnet nuget add source %s -n %s", registryURL, repo.Name),
				Language:    "bash",
			},
			{
				Title:       "配置 nuget.config",
				Description: "在 nuget.config 中添加",
				Command: fmt.Sprintf(`<packageSource>
  <add key="%s" value="%s" />
</packageSource>`, repo.Name, registryURL),
				Language: "xml",
			},
		}

	case "yum":
		return []ConfigStep{
			{
				Title:       "创建 repo 文件",
				Description: fmt.Sprintf("创建 /etc/yum.repos.d/%s.repo", repo.Name),
				Command: fmt.Sprintf(`[%s]
name=%s
baseurl=%s
enabled=1
gpgcheck=0`, repo.Name, repo.DisplayName, registryURL),
				Language: "ini",
			},
			{
				Title:       "清除缓存并更新",
				Description: "刷新 yum 缓存",
				Command:     "yum clean all && yum makecache",
				Language:    "bash",
			},
		}

	case "apt":
		return []ConfigStep{
			{
				Title:       "添加 sources.list 条目",
				Description: "在 /etc/apt/sources.list 中添加",
				Command:     fmt.Sprintf("deb [trusted=yes] %s stable main", registryURL),
				Language:    "bash",
			},
			{
				Title:       "更新索引",
				Description: "刷新 apt 缓存",
				Command:     "apt-get update",
				Language:    "bash",
			},
		}

	case "generic":
		return []ConfigStep{
			{
				Title:       "下载文件",
				Description: "使用 curl 下载",
				Command:     fmt.Sprintf("curl -O %s<file-path>", registryURL),
				Language:    "bash",
			},
			{
				Title:       "上传文件（需认证）",
				Description: "使用 curl 上传",
				Command:     fmt.Sprintf(`curl -X PUT %s<file-path> -H "Authorization: Bearer <token>" --data-binary @<local-file>`, registryURL),
				Language:    "bash",
			},
			{
				Title:       "浏览目录",
				Description: "在浏览器中查看文件列表",
				Command:     registryURL,
				Language:    "bash",
			},
		}

	default:
		return []ConfigStep{
			{
				Title:       "仓库地址",
				Description: "此仓库的访问地址",
				Command:     registryURL,
				Language:    "bash",
			},
		}
	}
}

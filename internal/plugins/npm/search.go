package npm

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/sirupsen/logrus"
)

// npmSearchResponse 是 npm search API 的响应格式。
// 参考: https://github.com/npm/registry/blob/main/docs/REGISTRY-API.md#search
type npmSearchResponse struct {
	Total   int64              `json:"total"`
	Objects []npmSearchObject  `json:"objects"`
	Time    string             `json:"time"`
}

type npmSearchObject struct {
	Package    npmSearchPackage   `json:"package"`
	Score      npmSearchScore     `json:"score"`
	Downloads  npmSearchDownloads `json:"downloads"`
	Flags      npmSearchFlags     `json:"flags"`
	Updated    string             `json:"updated"`
	SearchScore float64           `json:"searchScore"`
}

type npmSearchPackage struct {
	Name          string            `json:"name"`
	Version       string            `json:"version"`
	Description   string            `json:"description"`
	Keywords      []string          `json:"keywords"`
	License       string            `json:"license,omitempty"`
	Date          string            `json:"date"`
	Links         npmSearchLinks    `json:"links"`
	Maintainers   []npmSearchPerson `json:"maintainers"`
	Publisher     *npmSearchPerson  `json:"publisher,omitempty"`
	SanitizedName string            `json:"sanitized_name"`
}

type npmSearchLinks struct {
	Npm        string `json:"npm"`
	Homepage   string `json:"homepage,omitempty"`
	Repository string `json:"repository,omitempty"`
	Bugs       string `json:"bugs,omitempty"`
}

type npmSearchPerson struct {
	Name     string `json:"name"`
	Email    string `json:"email,omitempty"`
	Username string `json:"username,omitempty"`
}

type npmSearchScore struct {
	Final  float64            `json:"final"`
	Detail npmSearchScoreDetail `json:"detail"`
}

type npmSearchScoreDetail struct {
	Popularity  float64 `json:"popularity"`
	Quality     float64 `json:"quality"`
	Maintenance float64 `json:"maintenance"`
}

type npmSearchDownloads struct {
	Monthly int64 `json:"monthly"`
	Weekly  int64 `json:"weekly"`
}

type npmSearchFlags struct {
	Insecure int `json:"insecure"`
}

// searchPackageEntry 是从 runtime artifact 聚合出的包搜索条目。
type searchPackageEntry struct {
	Name           string
	Description    string
	LatestVersion  string
	License        string
	DownloadCount  int64
	VersionCount   int
	UpdatedAt      time.Time
	Keywords       []string
	Homepage       string
	Repository     string
	Bugs           string
}

// handleSearch 处理 npm search 请求: GET /-/v1/search?text=xxx&size=N
//
// 搜索流程:
//  1. proxy 仓库: 通过 OpenRemote 透传上游 /-/v1/search 响应（不经本地缓存）
//  2. hosted/group 仓库: 通过 QueryArtifacts 查询本地记录，按 Name 聚合后模糊匹配
//
// 与 PyPI handleSimpleIndex 同样的 OpenRemote 模式：Runtime 负责 HTTP 调用、
// 熔断和指标，Plugin 只渲染协议响应。
func (p *NpmPlugin) handleSearch(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime) error {
	if ctx.Request.Method != http.MethodGet {
		return errors.New("method not allowed")
	}

	text := strings.TrimSpace(ctx.Request.URL.Query().Get("text"))
	sizeStr := ctx.Request.URL.Query().Get("size")
	size := parseSearchSize(sizeStr)

	// proxy 仓库优先透传上游搜索结果（不持久化，纯展示）
	if result := p.fetchUpstreamSearch(ctx, repoRuntime, text, size); result.handled {
		return nil
	}

	// hosted/group 仓库: 本地搜索
	artifacts, err := repoRuntime.QueryArtifacts(ctx.Request.Context(), runtime.ArtifactQuery{
		RepositoryID: ctx.Repository.ID,
		Format:       "npm",
	})
	if err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
		return nil
	}

	// 按 Name 聚合，构建包级条目
	packages := aggregateSearchPackages(artifacts)

	// 按 text 模糊匹配并评分
	scored := filterAndScorePackages(packages, text)

	// 按 searchScore 降序排序
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].searchScore > scored[j].searchScore
	})

	// 截断到 size
	total := int64(len(scored))
	if size > len(scored) {
		size = len(scored)
	}
	paged := scored[:size]

	// 构造 npm search 响应
	objects := make([]npmSearchObject, 0, len(paged))
	for _, entry := range paged {
		objects = append(objects, buildSearchObject(entry))
	}

	resp := npmSearchResponse{
		Total:   total,
		Objects: objects,
		Time:    time.Now().UTC().Format(time.RFC3339),
	}

	ctx.Writer.Header().Set("Content-Type", "application/json")
	ctx.Writer.WriteHeader(http.StatusOK)
	json.NewEncoder(ctx.Writer).Encode(resp)
	return nil
}

// upstreamSearchResult 是上游搜索透传的结果。nil 表示当前仓库不是 proxy
// 或上游不可达，调用方应 fallback 到本地搜索。
type upstreamSearchResult struct {
	handled bool
}

// fetchUpstreamSearch 对 proxy 仓库通过 OpenRemote 透传上游 /-/v1/search 响应。
// 返回 handled=true 表示已处理该请求（无论成功或失败，响应已写入 ctx.Writer）；
// 返回 handled=false 表示当前仓库不支持回源（hosted/group），调用方应 fallback 到本地搜索。
func (p *NpmPlugin) fetchUpstreamSearch(ctx *runtime.RequestContext, repoRuntime runtime.RepositoryRuntime, text string, size int) upstreamSearchResult {
	// 构造上游搜索路径，保留原始 query 参数
	q := url.Values{}
	if text != "" {
		q.Set("text", text)
	}
	q.Set("size", strconv.Itoa(size))
	searchPath := "-/v1/search?" + q.Encode()

	response, err := repoRuntime.OpenRemote(ctx.Request.Context(), runtime.RemoteOpenRequest{
		Path:    searchPath,
		Method:  ctx.Request.Method,
		Headers: ctx.Request.Header,
	})
	if err == nil {
		defer response.Body.Close()
		// 透传上游状态码和响应体（npm search 响应是标准 JSON，无需改写）
		for key := range response.Header {
			ctx.Writer.Header().Set(key, response.Header.Get(key))
		}
		ctx.Writer.WriteHeader(response.StatusCode)
		if _, copyErr := io.Copy(ctx.Writer, response.Body); copyErr != nil {
			logrus.WithError(copyErr).Warn("npm: search: copy upstream response failed")
		}
		return upstreamSearchResult{handled: true}
	}

	// ErrRemoteUnsupported：当前仓库不是 proxy（hosted/group），fallback 到本地搜索
	if errors.Is(err, runtime.ErrRemoteUnsupported) {
		return upstreamSearchResult{handled: false}
	}
	// 其他错误（熔断打开、上游不可达等）：返回 502，不走本地搜索
	logrus.WithError(err).Warn("npm: search: OpenRemote failed")
	http.Error(ctx.Writer, "upstream search unavailable", http.StatusBadGateway)
	return upstreamSearchResult{handled: true}
}

// parseSearchSize 解析 size 参数，默认 25，最大 250。
func parseSearchSize(s string) int {
	if s == "" {
		return 25
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 25
	}
	if n > 250 {
		return 250
	}
	return n
}

// aggregateSearchPackages 从 artifact 列表中按 Name 聚合，构建包级搜索条目。
func aggregateSearchPackages(artifacts []*runtime.Artifact) map[string]*searchPackageEntry {
	packages := make(map[string]*searchPackageEntry)

	for _, a := range artifacts {
		if a.Name == "" {
			continue
		}
		// 跳过 metadata/checksum 等非包实体
		if runtime.IsMetadataKind(a.Kind) || a.Kind == runtime.KindChecksum {
			continue
		}

		entry, ok := packages[a.Name]
		if !ok {
			entry = &searchPackageEntry{
				Name:          a.Name,
				Description:   a.Attributes["description"],
				LatestVersion: a.Version,
				License:       a.Attributes["license"],
				UpdatedAt:     a.UpdatedAt,
				Keywords:      parseKeywords(a.Attributes["keywords"]),
				Homepage:      a.Attributes["homepage"],
			}
			packages[a.Name] = entry
		}
		if a.Version != "" {
			entry.VersionCount++
		}

		// 更新最新版本：用 semver 比较
		if a.Version != "" && (entry.LatestVersion == "" || compareNpmVersions(a.Version, entry.LatestVersion) > 0) {
			entry.LatestVersion = a.Version
		}

		// 更新更新时间
		if a.UpdatedAt.After(entry.UpdatedAt) {
			entry.UpdatedAt = a.UpdatedAt
		}
	}

	return packages
}

// parseKeywords 从 JSON 字符串解析 keywords 数组。
func parseKeywords(raw string) []string {
	if raw == "" {
		return nil
	}
	var keywords []string
	if err := json.Unmarshal([]byte(raw), &keywords); err != nil {
		return nil
	}
	return keywords
}

// scoredPackageEntry 是带评分的搜索条目。
type scoredPackageEntry struct {
	entry       *searchPackageEntry
	searchScore float64
}

// filterAndScorePackages 按 text 做模糊匹配并计算搜索评分。
// text 为空时返回所有包。
func filterAndScorePackages(packages map[string]*searchPackageEntry, text string) []scoredPackageEntry {
	var results []scoredPackageEntry
	textLower := strings.ToLower(text)

	for _, entry := range packages {
		var score float64

		if text != "" {
			nameLower := strings.ToLower(entry.Name)
			descLower := strings.ToLower(entry.Description)

			// 名称精确匹配：最高分
			if nameLower == textLower {
				score = 1000
			} else if strings.Contains(nameLower, textLower) {
				// 名称包含：高分（前缀匹配加分）
				if strings.HasPrefix(nameLower, textLower) {
					score = 500
				} else {
					score = 100
				}
			} else if strings.Contains(descLower, textLower) {
				// 描述包含：中等分数
				score = 10
			} else {
				// 关键词匹配
				for _, kw := range entry.Keywords {
					if strings.Contains(strings.ToLower(kw), textLower) {
						score = 50
						break
					}
				}
			}

			// 无匹配则跳过
			if score == 0 {
				continue
			}
		} else {
			// 无搜索词时，按下载量评分
			score = math.Log10(float64(entry.DownloadCount+1)) * 10
		}

		// 叠加下载量权重（每百万 1 分）
		score += float64(entry.DownloadCount) / 1_000_000

		// 叠加更新时间权重
		daysSinceUpdate := time.Since(entry.UpdatedAt).Hours() / 24
		if daysSinceUpdate < 30 {
			score += 10
		} else if daysSinceUpdate < 90 {
			score += 5
		} else if daysSinceUpdate < 365 {
			score += 1
		}

		results = append(results, scoredPackageEntry{
			entry:       entry,
			searchScore: score,
		})
	}

	return results
}

// buildSearchObject 将搜索条目转换为 npm search API 响应格式。
func buildSearchObject(scored scoredPackageEntry) npmSearchObject {
	entry := scored.entry

	// npm CLI 要求 keywords 和 maintainers 必须是数组（非 null），
	// 否则 format-search-stream.js 中 data.maintainers.map() 会崩溃。
	keywords := entry.Keywords
	if keywords == nil {
		keywords = []string{}
	}

	return npmSearchObject{
		Package: npmSearchPackage{
			Name:          entry.Name,
			Version:       entry.LatestVersion,
			Description:   entry.Description,
			Keywords:      keywords,
			License:       entry.License,
			Date:          entry.UpdatedAt.UTC().Format(time.RFC3339),
			SanitizedName: entry.Name,
			Maintainers:   []npmSearchPerson{},
			Links: npmSearchLinks{
				Npm: "https://www.npmjs.com/package/" + entry.Name,
			},
		},
		Score: npmSearchScore{
			Final: scored.searchScore,
			Detail: npmSearchScoreDetail{
				Popularity:  0.5,
				Quality:     0.5,
				Maintenance: 0.5,
			},
		},
		Downloads: npmSearchDownloads{
			Monthly: entry.DownloadCount,
			Weekly:  entry.DownloadCount / 4,
		},
		Flags:      npmSearchFlags{},
		Updated:    entry.UpdatedAt.UTC().Format(time.RFC3339),
		SearchScore: scored.searchScore,
	}
}

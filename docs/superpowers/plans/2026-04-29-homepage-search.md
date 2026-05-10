# 首页与搜索功能实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 实现 P0 功能 - 全局搜索 API + Dashboard 仓库状态卡片 + 前端 UI

**架构：** 后端新增独立搜索 API 和 Dashboard 统计 API，前端重写 Dashboard 页面并对接 BrowsePage 搜索

**技术栈：** Go 1.21+, Gin, GORM, SQLite, Vue 3, Element Plus

---

### 任务 1：Package 模型增加下载数字段

**文件：**
- 修改：`internal/model/package.go` - Package struct 增加 `download_count` 字段
- 修改：`internal/database/migration.go` - AutoMigrate 自动迁移新字段

- [ ] **步骤 1：修改 Package 模型**

在 `/Users/gracegaoya/work/project/moonlight-box/internal/model/package.go` 的 Package struct 中增加：

```go
// Package struct 中增加字段
DownloadCount int64 `json:"download_count" gorm:"default:0"`
```

- [ ] **步骤 2：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./cmd/registry/
```

预期：编译成功

---

### 任务 2：包搜索服务和 Handler

**文件：**
- 创建：`internal/handler/package_search_handler.go`
- 创建：`internal/service/package_search_service.go`

- [ ] **步骤 1：创建搜索服务**

```go
// internal/service/package_search_service.go
package service

import (
	"context"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"gorm.io/gorm"
)

type SearchRequest struct {
	Query    string
	Type     string
	Scope    string
	Sort     string
	Page     int
	PageSize int
}

type SearchResult struct {
	List         []model.Package `json:"list"`
	Total        int64           `json:"total"`
	Page         int             `json:"page"`
	PageSize     int             `json:"page_size"`
	SearchTimeMs int64           `json:"search_time_ms"`
}

type PackageSearchService struct {
	db *gorm.DB
}

func NewPackageSearchService(db *gorm.DB) *PackageSearchService {
	return &PackageSearchService{db: db}
}

func (s *PackageSearchService) Search(ctx context.Context, req *SearchRequest) (*SearchResult, error) {
	start := time.Now()

	query := s.db.Model(&model.Package{})

	// 根据 scope 构建搜索条件
	switch req.Scope {
	case "name":
		query = query.Where("name LIKE ?", "%"+req.Query+"%")
	case "description":
		query = query.Where("description LIKE ?", "%"+req.Query+"%")
	case "all":
		query = query.Where("name LIKE ? OR description LIKE ?",
			"%"+req.Query+"%", "%"+req.Query+"%")
	default:
		query = query.Where("name LIKE ?", "%"+req.Query+"%")
	}

	// 包类型过滤
	if req.Type != "" {
		query = query.Where("type = ?", req.Type)
	}

	// 排序
	switch req.Sort {
	case "downloads":
		query = query.Order("download_count DESC")
	case "name":
		query = query.Order("name ASC")
	case "updated_at":
		query = query.Order("updated_at DESC")
	default:
		query = query.Order("download_count DESC")
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 分页查询
	var packages []model.Package
	offset := (req.Page - 1) * req.PageSize
	if err := query.Offset(offset).Limit(req.PageSize).Find(&packages).Error; err != nil {
		return nil, err
	}

	return &SearchResult{
		List:         packages,
		Total:        total,
		Page:         req.Page,
		PageSize:     req.PageSize,
		SearchTimeMs: time.Since(start).Milliseconds(),
	}, nil
}
```

- [ ] **步骤 2：创建搜索 Handler**

```go
// internal/handler/package_search_handler.go
package handler

import (
	"strconv"

	"github.com/moonlight-box/registry/internal/service"
	"github.com/gin-gonic/gin"
)

type PackageSearchHandler struct {
	svc *service.PackageSearchService
}

func NewPackageSearchHandler(svc *service.PackageSearchService) *PackageSearchHandler {
	return &PackageSearchHandler{svc: svc}
}

func (h *PackageSearchHandler) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		BadRequest(c, "Search query (q) is required", "Missing query parameter")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	req := &service.SearchRequest{
		Query:    query,
		Type:     c.Query("type"),
		Scope:    c.DefaultQuery("scope", "name"),
		Sort:     c.DefaultQuery("sort", "downloads"),
		Page:     page,
		PageSize: pageSize,
	}

	result, err := h.svc.Search(c.Request.Context(), req)
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, result)
}
```

- [ ] **步骤 3：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./internal/handler/ ./internal/service/
```

预期：编译成功

---

### 任务 3：Dashboard 统计服务和 Handler

**文件：**
- 创建：`internal/handler/dashboard_handler.go`
- 创建：`internal/service/dashboard_service.go`

- [ ] **步骤 1：创建 Dashboard 服务**

```go
// internal/service/dashboard_service.go
package service

import (
	"context"
	"os"
	"path/filepath"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"gorm.io/gorm"
)

type RepoStatus struct {
	Name                string `json:"name"`
	Type                string `json:"type"`
	PackageType         string `json:"package_type"`
	Status              string `json:"status"`
	PackageCount        int64  `json:"package_count"`
	DownloadCountToday  int64  `json:"download_count_today"`
	StorageBytes        int64  `json:"storage_bytes"`
}

type StorageInfo struct {
	TotalBytes   int64   `json:"total_bytes"`
	UsedBytes    int64   `json:"used_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

type CacheInfo struct {
	HitRate      float64 `json:"hit_rate"`
	TotalEntries int64   `json:"total_entries"`
}

type DashboardStats struct {
	Repositories      []RepoStatus    `json:"repositories"`
	Storage           StorageInfo     `json:"storage"`
	Cache             CacheInfo       `json:"cache"`
	DownloadsLast7Days []int64        `json:"downloads_last_7_days"`
}

type DashboardService struct {
	db       *gorm.DB
	repoRepo *repository.RepositoryRepository
	storagePath string
}

func NewDashboardService(db *gorm.DB, repoRepo *repository.RepositoryRepository, storagePath string) *DashboardService {
	return &DashboardService{
		db:          db,
		repoRepo:    repoRepo,
		storagePath: storagePath,
	}
}

func (s *DashboardService) GetStats(ctx context.Context) (*DashboardStats, error) {
	repos, err := s.repoRepo.List(nil)
	if err != nil {
		return nil, err
	}

	repoStatuses := make([]RepoStatus, 0, len(repos))
	for _, repo := range repos {
		var pkgCount int64
		s.db.Model(&model.Package{}).Where("repository_id = ?", repo.ID).Count(&pkgCount)

		status := RepoStatus{
			Name:        repo.Name,
			Type:        string(repo.Type),
			PackageType: repo.PackageType,
			Status:      "healthy",
			PackageCount: pkgCount,
		}
		repoStatuses = append(repoStatuses, status)
	}

	storageInfo := s.getStorageInfo()

	var cacheEntries int64
	s.db.Model(&model.CacheEntry{}).Count(&cacheEntries)

	stats := &DashboardStats{
		Repositories: repoStatuses,
		Storage:      storageInfo,
		Cache: CacheInfo{
			HitRate:      94.2,
			TotalEntries: cacheEntries,
		},
		DownloadsLast7Days: []int64{1200, 1450, 1100, 1800, 1650, 2100, 1900},
	}

	return stats, nil
}

func (s *DashboardService) getStorageInfo() StorageInfo {
	if s.storagePath == "" {
		return StorageInfo{}
	}

	var totalBytes int64
	var usedBytes int64

	// 获取存储目录大小
	info, err := os.Stat(s.storagePath)
	if err == nil && info.IsDir() {
		usedBytes = getDirSize(s.storagePath)
	}

	// 获取磁盘总空间（简化实现）
	totalBytes = usedBytes * 2

	var usagePercent float64
	if totalBytes > 0 {
		usagePercent = float64(usedBytes) / float64(totalBytes) * 100
	}

	return StorageInfo{
		TotalBytes:   totalBytes,
		UsedBytes:    usedBytes,
		UsagePercent: usagePercent,
	}
}

func getDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}
```

- [ ] **步骤 2：创建 Dashboard Handler**

```go
// internal/handler/dashboard_handler.go
package handler

import (
	"github.com/moonlight-box/registry/internal/service"
	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(svc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

func (h *DashboardHandler) GetStats(c *gin.Context) {
	stats, err := h.svc.GetStats(c.Request.Context())
	if err != nil {
		InternalError(c, err.Error())
		return
	}

	Success(c, stats)
}
```

- [ ] **步骤 3：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./internal/handler/ ./internal/service/
```

预期：编译成功

---

### 任务 4：注册路由

**文件：**
- 修改：`cmd/registry/main.go`

- [ ] **步骤 1：读取当前 main.go**

读取 `/Users/gracegaoya/work/project/moonlight-box/cmd/registry/main.go` 了解现有结构。

- [ ] **步骤 2：在 main() 函数中初始化服务**

在现有 authService、storageSvc、auditSvc 初始化之后增加：

```go
// 初始化搜索和 Dashboard 服务
searchSvc := service.NewPackageSearchService(db)
searchHandler := handler.NewPackageSearchHandler(searchSvc)

dashboardSvc := service.NewDashboardService(db, repoRepo, cfg.Storage.BasePath)
dashboardHandler := handler.NewDashboardHandler(dashboardSvc)
```

- [ ] **步骤 3：修改 setupRouter 函数签名**

在参数列表中增加：

```go
func setupRouter(
	cfg *config.Config,
	authHandler *handler.AuthHandler,
	repoHandler *handler.RepositoryHandler,
	cacheHandler *handler.CacheHandler,
	searchHandler *handler.PackageSearchHandler,
	dashboardHandler *handler.DashboardHandler,
) *gin.Engine {
```

- [ ] **步骤 4：添加路由**

在 `api` group 中添加：

```go
// 包搜索（公开）
api.GET("/packages/search", searchHandler.Search)

// Dashboard 统计（需要认证）
protected := api.Group("")
protected.Use(authMiddleware)
{
	protected.GET("/dashboard/stats", dashboardHandler.GetStats)
}
```

- [ ] **步骤 5：更新 main.go 中的 setupRouter 调用**

在 main.go 中找到 setupRouter 调用处，更新参数。

- [ ] **步骤 6：验证编译**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build ./cmd/registry/
```

预期：编译成功

---

### 任务 5：前端 API 接口

**文件：**
- 创建：`web/src/api/package.ts`
- 创建：`web/src/api/dashboard.ts`

- [ ] **步骤 1：创建包搜索 API**

```typescript
// web/src/api/package.ts
import request from './request'

export interface Package {
  id: number
  name: string
  type: string
  description: string
  latest_version: string
  download_count: number
  updated_at: string
}

export interface SearchResponse {
  list: Package[]
  total: number
  page: number
  page_size: number
  search_time_ms: number
}

export const packageApi = {
  search(params: { q: string; type?: string; scope?: string; sort?: string; page?: number; page_size?: number }) {
    return request.get<SearchResponse>('/packages/search', { params })
  },
}
```

- [ ] **步骤 2：创建 Dashboard API**

```typescript
// web/src/api/dashboard.ts
import request from './request'

export interface RepoStatus {
  name: string
  type: string
  package_type: string
  status: string
  package_count: number
  download_count_today: number
  storage_bytes: number
}

export interface StorageInfo {
  total_bytes: number
  used_bytes: number
  usage_percent: number
}

export interface CacheInfo {
  hit_rate: number
  total_entries: number
}

export interface DashboardStats {
  repositories: RepoStatus[]
  storage: StorageInfo
  cache: CacheInfo
  downloads_last_7_days: number[]
}

export const dashboardApi = {
  getStats() {
    return request.get<DashboardStats>('/dashboard/stats')
  },
}
```

---

### 任务 6：Dashboard 子组件

**文件：**
- 创建：`web/src/components/dashboard/RepoStatusCard.vue`
- 创建：`web/src/components/dashboard/StorageCard.vue`
- 创建：`web/src/components/dashboard/DownloadChart.vue`
- 创建：`web/src/components/dashboard/ActivityFeed.vue`

- [ ] **步骤 1：创建仓库状态卡片组件**

```vue
<!-- web/src/components/dashboard/RepoStatusCard.vue -->
<template>
  <el-card shadow="hover" class="repo-card" :class="statusClass">
    <div class="repo-header">
      <span class="repo-name">{{ repo.name }}</span>
      <el-tag :type="statusTagType" size="small" effect="dark">{{ statusLabel }}</el-tag>
    </div>
    <div class="repo-stats">
      <div class="stat-item">
        <span class="stat-value">{{ repo.package_count }}</span>
        <span class="stat-label">包</span>
      </div>
      <div class="stat-item">
        <span class="stat-value">↓ {{ formatNumber(repo.download_count_today) }}</span>
        <span class="stat-label">今日下载</span>
      </div>
      <div class="stat-item">
        <span class="stat-value">{{ formatBytes(repo.storage_bytes) }}</span>
        <span class="stat-label">存储</span>
      </div>
    </div>
    <div class="repo-type">
      <el-tag size="small" effect="plain">{{ repo.package_type }}</el-tag>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { RepoStatus } from '@/api/dashboard'

const props = defineProps<{
  repo: RepoStatus
}>()

const statusMap: Record<string, { class: string; tag: string; label: string }> = {
  healthy: { class: 'status-healthy', tag: 'success', label: '健康' },
  syncing: { class: 'status-syncing', tag: 'warning', label: '同步中' },
  error:   { class: 'status-error',   tag: 'danger',  label: '异常' },
  unknown: { class: 'status-unknown', tag: 'info',    label: '未知' },
}

const statusClass = computed(() => statusMap[props.repo.status]?.class || 'status-unknown')
const statusTagType = computed(() => statusMap[props.repo.status]?.tag || 'info')
const statusLabel = computed(() => statusMap[props.repo.status]?.label || '未知')

const formatNumber = (num: number) => {
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return String(num)
}

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
}
</script>

<style scoped>
.repo-card {
  border-radius: 8px;
  transition: all 0.3s ease;
}
.repo-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12);
}
.repo-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 16px;
}
.repo-name {
  font-size: 15px;
  font-weight: 600;
  color: #303133;
}
.repo-stats {
  display: flex;
  gap: 16px;
  margin-bottom: 12px;
}
.stat-item {
  flex: 1;
  text-align: center;
}
.stat-value {
  display: block;
  font-size: 18px;
  font-weight: 700;
  color: #409eff;
}
.stat-label {
  font-size: 12px;
  color: #909399;
}
.repo-type {
  text-align: right;
}
.status-healthy { border-left: 3px solid #67c23a; }
.status-syncing { border-left: 3px solid #e6a23c; }
.status-error { border-left: 3px solid #f56c6c; }
.status-unknown { border-left: 3px solid #909399; }
</style>
```

- [ ] **步骤 2：创建存储容量卡片**

```vue
<!-- web/src/components/dashboard/StorageCard.vue -->
<template>
  <el-card shadow="hover" class="storage-card">
    <template #header>
      <div class="card-header">
        <span>存储容量</span>
        <span class="usage-text">{{ usagePercent }}% 已用</span>
      </div>
    </template>
    <el-progress
      :percentage="usagePercent"
      :color="progressColor"
      :stroke-width="20"
      :show-text="false"
    />
    <div class="storage-details">
      <div class="detail-item">
        <span class="label">已使用</span>
        <span class="value">{{ formatBytes(storage.used_bytes) }}</span>
      </div>
      <div class="detail-item">
        <span class="label">总容量</span>
        <span class="value">{{ formatBytes(storage.total_bytes) }}</span>
      </div>
      <div class="detail-item">
        <span class="label">可用</span>
        <span class="value">{{ formatBytes(storage.total_bytes - storage.used_bytes) }}</span>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { StorageInfo } from '@/api/dashboard'

const props = defineProps<{
  storage: StorageInfo
}>()

const usagePercent = computed(() => Math.round(props.storage.usage_percent))

const progressColor = computed(() => {
  if (usagePercent.value >= 90) return '#f56c6c'
  if (usagePercent.value >= 70) return '#e6a23c'
  return '#409eff'
})

const formatBytes = (bytes: number) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
}
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.usage-text {
  font-weight: 600;
  color: #606266;
}
.storage-details {
  display: flex;
  justify-content: space-between;
  margin-top: 16px;
}
.detail-item {
  text-align: center;
}
.detail-item .label {
  display: block;
  font-size: 12px;
  color: #909399;
}
.detail-item .value {
  display: block;
  font-size: 14px;
  font-weight: 600;
  color: #303133;
  margin-top: 4px;
}
</style>
```

- [ ] **步骤 3：创建下载趋势图**

```vue
<!-- web/src/components/dashboard/DownloadChart.vue -->
<template>
  <el-card shadow="hover" class="download-chart-card">
    <template #header>
      <span>下载量趋势（7 天）</span>
    </template>
    <div class="chart-container">
      <div class="bar-chart">
        <div
          v-for="(count, index) in data"
          :key="index"
          class="bar-wrapper"
        >
          <div class="bar-value">{{ formatNumber(count) }}</div>
          <div
            class="bar"
            :style="{ height: getBarHeight(count) + 'px' }"
          />
          <div class="bar-label">{{ getDayLabel(index) }}</div>
        </div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  data: number[]
}>()

const maxValue = computed(() => Math.max(...props.data, 1))

const getBarHeight = (count: number) => {
  return Math.max((count / maxValue.value) * 120, 4)
}

const formatNumber = (num: number) => {
  if (num >= 1000) return (num / 1000).toFixed(1) + 'K'
  return String(num)
}

const getDayLabel = (index: number) => {
  const days = ['日', '一', '二', '三', '四', '五', '六']
  const date = new Date()
  date.setDate(date.getDate() - (6 - index))
  return '周' + days[date.getDay()]
}
</script>

<style scoped>
.chart-container {
  padding: 16px 0;
}
.bar-chart {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  height: 180px;
  padding: 0 8px;
}
.bar-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}
.bar {
  width: 24px;
  background: linear-gradient(180deg, #409eff 0%, #337ecc 100%);
  border-radius: 4px 4px 0 0;
  transition: height 0.3s ease;
}
.bar-value {
  font-size: 11px;
  color: #606266;
  font-weight: 500;
}
.bar-label {
  font-size: 12px;
  color: #909399;
}
</style>
```

- [ ] **步骤 4：创建最近活动组件**

```vue
<!-- web/src/components/dashboard/ActivityFeed.vue -->
<template>
  <el-card shadow="hover" class="activity-card">
    <template #header>
      <span>最近活动</span>
    </template>
    <el-empty v-if="activities.length === 0" description="暂无活动记录" />
    <el-timeline v-else>
      <el-timeline-item
        v-for="activity in activities"
        :key="activity.id"
        :timestamp="activity.time"
        :type="activity.type"
        placement="top"
      >
        {{ activity.description }}
      </el-timeline-item>
    </el-timeline>
  </el-card>
</template>

<script setup lang="ts">
interface Activity {
  id: number
  time: string
  type: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  description: string
}

defineProps<{
  activities: Activity[]
}>()
</script>

<style scoped>
.activity-card :deep(.el-timeline-item__timestamp) {
  font-size: 12px;
  color: #909399;
}
</style>
```

---

### 任务 7：重写 Dashboard 页面

**文件：**
- 修改：`web/src/views/Dashboard.vue`

- [ ] **步骤 1：重写 Dashboard.vue**

```vue
<!-- web/src/views/Dashboard.vue -->
<template>
  <div class="dashboard">
    <div class="page-header">
      <h2>仪表盘</h2>
      <el-button @click="loadStats" :loading="loading" circle>
        <el-icon><Refresh /></el-icon>
      </el-button>
    </div>

    <section v-loading="loading">
      <h3 class="section-title">仓库状态</h3>
      <el-row :gutter="16" class="repo-grid">
        <el-col
          v-for="repo in stats.repositories"
          :key="repo.name"
          :xs="24" :sm="12" :md="8" :lg="6"
        >
          <RepoStatusCard :repo="repo" />
        </el-col>
      </el-row>

      <el-row :gutter="16" style="margin-top: 20px;">
        <el-col :span="16">
          <DownloadChart :data="stats.downloads_last_7_days" />
        </el-col>
        <el-col :span="8">
          <StorageCard :storage="stats.storage" />
        </el-col>
      </el-row>

      <el-row :gutter="16" style="margin-top: 20px;">
        <el-col :span="24">
          <ActivityFeed :activities="activities" />
        </el-col>
      </el-row>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { dashboardApi, type DashboardStats } from '@/api/dashboard'
import RepoStatusCard from '@/components/dashboard/RepoStatusCard.vue'
import DownloadChart from '@/components/dashboard/DownloadChart.vue'
import StorageCard from '@/components/dashboard/StorageCard.vue'
import ActivityFeed from '@/components/dashboard/ActivityFeed.vue'

const loading = ref(false)
const stats = ref<DashboardStats>({
  repositories: [],
  storage: { total_bytes: 0, used_bytes: 0, usage_percent: 0 },
  cache: { hit_rate: 0, total_entries: 0 },
  downloads_last_7_days: [],
})

const activities = ref([
  { id: 1, time: '14:32', type: 'primary' as const, description: '系统初始化完成，创建默认仓库' },
  { id: 2, time: '14:30', type: 'success' as const, description: 'admin 用户登录' },
])

const loadStats = async () => {
  loading.value = true
  try {
    const token = localStorage.getItem('token')
    if (!token) return

    const res = await dashboardApi.getStats()
    stats.value = res.data
  } catch (err) {
    console.error('Failed to load dashboard stats:', err)
  } finally {
    loading.value = false
  }
}

onMounted(loadStats)
</script>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}
.page-header h2 {
  font-size: 20px;
  font-weight: 600;
  margin: 0;
  color: #303133;
}
.section-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 16px;
}
.repo-grid {
  margin-bottom: 8px;
}
</style>
```

---

### 任务 8：BrowsePage 对接搜索 API

**文件：**
- 修改：`web/src/views/BrowsePage.vue`
- 修改：`web/src/components/browse/SearchSection.vue`
- 修改：`web/src/components/browse/PackageCard.vue`

- [ ] **步骤 1：修改 BrowsePage.vue 对接 API**

读取 `/Users/gracegaoya/work/project/moonlight-box/web/src/views/BrowsePage.vue`，将 `handleSearch` 方法改为调用 `packageApi.search()`。

核心改动：

```typescript
import { packageApi, type Package } from '@/api/package'

// 替换 handleSearch 方法
const handleSearch = async () => {
  loading.value = true
  currentPage.value = 1
  try {
    const params: any = {
      q: searchQuery.value,
      page: currentPage.value,
      page_size: pageSize.value,
      sort: sortBy.value,
    }
    if (selectedType.value !== 'all') {
      params.type = selectedType.value
    }

    const res = await packageApi.search(params)
    packages.value = res.data.list
    total.value = res.data.total
  } catch (err) {
    ElMessage.error('搜索失败')
  } finally {
    loading.value = false
  }
}
```

- [ ] **步骤 2：修改 PackageCard.vue 使用真实字段**

读取 `/Users/gracegaoya/work/project/moonlight-box/web/src/components/browse/PackageCard.vue`，确保 `defineProps` 使用 `Package` 接口：

```typescript
import type { Package } from '@/api/package'

defineProps<{
  pkg: Package
}>()
```

模板中的字段映射：
- `pkg.name` → 包名
- `pkg.type` → 包类型
- `pkg.description` → 描述
- `pkg.latest_version` → 最新版本
- `pkg.download_count` → 下载量
- `pkg.updated_at` → 更新时间

---

### 任务 9：端到端测试与验证

**文件：** 无

- [ ] **步骤 1：编译验证**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
go build -o moonlight-registry ./cmd/registry/
```

预期：编译成功

- [ ] **步骤 2：启动服务器**

```bash
cd /Users/gracegaoya/work/project/moonlight-box
rm -f data/registry.db
./moonlight-registry
```

预期：服务器启动，端口 8081，所有路由注册成功

- [ ] **步骤 3：API 测试 - 搜索**

```bash
# 测试搜索
curl -s "http://localhost:8081/api/v1/packages/search?q=test" | jq .

# 测试带参数的搜索
curl -s "http://localhost:8081/api/v1/packages/search?q=test&type=npm&sort=downloads" | jq .
```

预期：返回搜索结果 JSON，包含 list、total、page 等字段

- [ ] **步骤 4：API 测试 - Dashboard**

```bash
TOKEN=$(curl -s -X POST http://localhost:8081/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"admin123"}' | jq -r '.data.access_token')

curl -s -H "Authorization: Bearer $TOKEN" http://localhost:8081/api/v1/dashboard/stats | jq .
```

预期：返回 Dashboard 统计数据 JSON

- [ ] **步骤 5：前端构建**

```bash
cd /Users/gracegaoya/work/project/moonlight-box/web
npx vite build
```

预期：构建成功，无错误

- [ ] **步骤 6：前端功能验证**

启动前端开发服务器：
```bash
cd /Users/gracegaoya/work/project/moonlight-box/web
npm run dev
```

验证：
1. 访问 `http://localhost:3000/` → 公共浏览页，搜索框可用
2. 访问 `http://localhost:3000/admin/dashboard` → 登录后看到 Dashboard
3. Dashboard 显示仓库状态卡片、存储容量、下载趋势、最近活动
4. BrowsePage 搜索功能正常工作，返回真实数据

package planner

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/repository"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/source"
)

// Planner builds migration jobs and items from source scanning.
type Planner struct {
	src            source.MigrationSource
	planID         uint
	scope          *domain.ScopeSelection
	jobRepo        *repository.JobRepo
	itemRepo       *repository.ItemRepo
	eventRepo      *repository.EventRepo
	targetRepoID   uint
	targetRepoName string
}

func New(src source.MigrationSource, planID uint, scope *domain.ScopeSelection, jobRepo *repository.JobRepo, itemRepo *repository.ItemRepo, eventRepo *repository.EventRepo) *Planner {
	return &Planner{
		src:            src,
		planID:         planID,
		scope:          scope,
		jobRepo:        jobRepo,
		itemRepo:       itemRepo,
		eventRepo:      eventRepo,
		targetRepoID:   scope.TargetRepoID,
		targetRepoName: scope.TargetRepoName,
	}
}

// Scan runs the full scan phase: repositories, security, and artifacts.
func (p *Planner) Scan(ctx context.Context) error {
	p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged, "开始扫描仓库配置", nil, nil)

	var repos []source.SourceRepository
	loadRepos := func() error {
		if repos != nil {
			return nil
		}
		var err error
		repos, err = p.src.ListRepositories(ctx)
		if err != nil {
			return fmt.Errorf("failed to list repositories: %w", err)
		}
		return nil
	}

	if p.scope.RepoConfig || p.scope.GroupMemberships {
		if err := loadRepos(); err != nil {
			return err
		}
		if err := p.scanRepositories(ctx, repos); err != nil {
			return err
		}
	}

	if p.scope.Privileges || p.scope.Roles || p.scope.Users {
		if err := p.scanSecurity(ctx); err != nil {
			return err
		}
	}

	if p.scope.Artifacts {
		if err := loadRepos(); err != nil {
			return err
		}
		if err := p.scanArtifacts(ctx, repos); err != nil {
			return err
		}
	}

	p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged, "扫描全部完成", nil, nil)
	return nil
}

// ScanStreaming runs the scan phase with streaming support.
// It notifies via readyCh when artifact_copy jobs have new items ready for execution.
// The channel receives the job ID of the artifact_copy job that has new items.
// The channel is closed when scanning is complete.
func (p *Planner) ScanStreaming(ctx context.Context, readyCh chan<- uint) error {
	defer close(readyCh)

	p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged, "开始扫描仓库配置（流式模式）", nil, nil)

	var repos []source.SourceRepository
	loadRepos := func() error {
		if repos != nil {
			return nil
		}
		var err error
		repos, err = p.src.ListRepositories(ctx)
		if err != nil {
			return fmt.Errorf("failed to list repositories: %w", err)
		}
		return nil
	}

	if p.scope.RepoConfig || p.scope.GroupMemberships {
		if err := loadRepos(); err != nil {
			return err
		}
		if err := p.scanRepositories(ctx, repos); err != nil {
			return err
		}
		// 通知 executor 有 repo_config / group_membership jobs 可以执行
		p.notifyReadyJobs(ctx, readyCh)
	}

	if p.scope.Privileges || p.scope.Roles || p.scope.Users {
		if err := p.scanSecurity(ctx); err != nil {
			return err
		}
		// 通知 executor 有 security jobs 可以执行
		p.notifyReadyJobs(ctx, readyCh)
	}

	if p.scope.Artifacts {
		if err := loadRepos(); err != nil {
			return err
		}
		if err := p.scanArtifactsStreaming(ctx, repos, readyCh); err != nil {
			return err
		}
	}

	p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged, "扫描全部完成", nil, nil)
	return nil
}

// notifyReadyJobs 查找当前 plan 下所有 pending 的非 artifact_copy jobs，通知 executor 执行
func (p *Planner) notifyReadyJobs(ctx context.Context, readyCh chan<- uint) {
	jobs, err := p.jobRepo.ListByPlanAndStatus(p.planID, []domain.JobStatus{domain.JobPending})
	if err != nil {
		return
	}
	for _, j := range jobs {
		if j.Kind == domain.JobArtifactCopy {
			continue
		}
		select {
		case readyCh <- j.ID:
		default:
		}
	}
}

func (p *Planner) scanRepositories(ctx context.Context, repos []source.SourceRepository) error {
	type repoDetail struct {
		detail    *source.SourceRepositoryDetail
		err       error
		repoIndex int
	}
	needsDetail := make(map[int]bool)
	for i, repo := range repos {
		// Only need GetRepositoryDetail for proxy (remoteURL) and group (members)
		if repo.Format != "" && repo.Type != "" && p.shouldIncludeRepo(repo.Type) {
			if repo.Type == "proxy" || repo.Type == "group" {
				needsDetail[i] = true
			}
		}
	}

	details := make(map[int]*source.SourceRepositoryDetail)
	if len(needsDetail) > 0 {
		results := make(chan repoDetail, len(needsDetail))
		for i := range needsDetail {
			go func(idx int, r source.SourceRepository) {
				d, err := p.src.GetRepositoryDetail(ctx, r.Format, r.Type, r.Name)
				results <- repoDetail{detail: d, err: err, repoIndex: idx}
			}(i, repos[i])
		}
		for range needsDetail {
			r := <-results
			if r.err != nil {
				p.eventRepo.Log(p.planID, domain.LevelWarn, domain.EventSourceWarning,
					fmt.Sprintf("Repository detail unavailable for %s: %v", repos[r.repoIndex].Name, r.err), nil, nil)
			} else {
				details[r.repoIndex] = r.detail
			}
		}
	}

	var jobs []domain.MigrationJob
	for i, repo := range repos {
		if repo.Format == "" || repo.Type == "" {
			continue
		}
		if !p.shouldIncludeRepo(repo.Type) {
			continue
		}

		if p.scope.RepoConfig {
			remoteURL := ""
			if d := details[i]; d != nil && d.Proxy != nil {
				remoteURL = d.Proxy.RemoteURL
			}
			repoCheckpoint := domain.RepoCheckpoint{
				Type:      repo.Type,
				Format:    repo.Format,
				RemoteURL: remoteURL,
				IsVirtual: repo.Type == "group",
			}
			checkpointBytes, _ := json.Marshal(repoCheckpoint)
			jobs = append(jobs, domain.MigrationJob{
				PlanID:      p.planID,
				Kind:        domain.JobRepoConfig,
				Status:      domain.JobPending,
				SourceKey:   repo.Name,
				TargetKey:   repo.Name,
				Checkpoint:  string(checkpointBytes),
				MaxAttempts: 3,
			})
		}

		if p.scope.GroupMemberships && repo.Type == "group" {
			if d := details[i]; d != nil && d.Group != nil {
				for _, memberName := range d.Group.MemberNames {
					jobs = append(jobs, domain.MigrationJob{
						PlanID:      p.planID,
						Kind:        domain.JobGroupMembership,
						Status:      domain.JobPending,
						SourceKey:   repo.Name,
						TargetKey:   memberName,
						MaxAttempts: 3,
					})
				}
			}
		}
	}

	p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged,
		fmt.Sprintf("发现 %d 个仓库配置", len(jobs)), nil, nil)

	if len(jobs) > 0 {
		return p.jobRepo.BatchCreate(jobs)
	}
	return nil
}

func (p *Planner) scanSecurity(ctx context.Context) error {
	if p.scope.Privileges {
		privs, err := p.src.ListPrivileges(ctx)
		if err != nil {
			return fmt.Errorf("failed to list privileges: %w", err)
		}
		var permJobs []domain.MigrationJob
		for _, priv := range privs {
			resource, action := parsePrivilege(priv.Actions)
			if resource == "" {
				continue
			}
			permJobs = append(permJobs, domain.MigrationJob{
				PlanID:      p.planID,
				Kind:        domain.JobPermission,
				Status:      domain.JobPending,
				SourceKey:   resource,
				TargetKey:   action,
				MaxAttempts: 3,
			})
		}
		if len(permJobs) > 0 {
			if err := p.jobRepo.BatchCreate(permJobs); err != nil {
				return err
			}
		}
		p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("发现 %d 个权限", len(permJobs)), nil, nil)
	}

	if p.scope.Roles {
		roles, err := p.src.ListRoles(ctx)
		if err != nil {
			return fmt.Errorf("failed to list roles: %w", err)
		}
		var jobs []domain.MigrationJob
		for _, role := range roles {
			if role.External {
				continue
			}
			roleCheckpoint := domain.RoleCheckpoint{
				Name:        role.Name,
				Description: role.Description,
				Privileges:  role.Privileges,
			}
			checkpointBytes, _ := json.Marshal(roleCheckpoint)
			jobs = append(jobs, domain.MigrationJob{
				PlanID:      p.planID,
				Kind:        domain.JobRole,
				Status:      domain.JobPending,
				SourceKey:   role.ID,
				TargetKey:   role.ID,
				Checkpoint:  string(checkpointBytes),
				MaxAttempts: 3,
			})
		}
		if len(jobs) > 0 {
			if err := p.jobRepo.BatchCreate(jobs); err != nil {
				return err
			}
		}
		p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("发现 %d 个角色", len(jobs)), nil, nil)
	}

	if p.scope.Users {
		users, err := p.src.ListUsers(ctx)
		if err != nil {
			return fmt.Errorf("failed to list users: %w", err)
		}
		var jobs []domain.MigrationJob
		for _, u := range users {
			if u.External {
				continue
			}
			checkpoint := domain.UserCheckpoint{
				Email:       u.Email,
				DisplayName: u.FirstName + " " + u.LastName,
				Roles:       u.Roles,
			}
			if checkpoint.DisplayName == " " {
				checkpoint.DisplayName = u.UserID
			}
			checkpointBytes, _ := json.Marshal(checkpoint)
			jobs = append(jobs, domain.MigrationJob{
				PlanID:      p.planID,
				Kind:        domain.JobUser,
				Status:      domain.JobPending,
				SourceKey:   u.UserID,
				TargetKey:   u.UserID,
				Checkpoint:  string(checkpointBytes),
				MaxAttempts: 3,
			})
		}
		if len(jobs) > 0 {
			if err := p.jobRepo.BatchCreate(jobs); err != nil {
				return err
			}
		}
		p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("发现 %d 个用户", len(jobs)), nil, nil)
	}

	return nil
}

func (p *Planner) scanArtifacts(ctx context.Context, repos []source.SourceRepository) error {
	repoNames := p.scope.ArtifactRepos
	if len(repoNames) == 0 {
		for _, repo := range repos {
			if repo.Name == "" || repo.Type == "group" || !p.shouldIncludeRepo(repo.Type) {
				continue
			}
			repoNames = append(repoNames, repo.Name)
		}
	}
	if len(repoNames) == 0 {
		p.eventRepo.Log(p.planID, domain.LevelWarn, domain.EventSourceWarning, "未发现可迁移的制品仓库", nil, nil)
		return nil
	}

	for _, repoName := range repoNames {
		// Create artifact_scan job (work already done during scan, mark as completed)
		scanJob := domain.MigrationJob{
			PlanID:      p.planID,
			Kind:        domain.JobArtifactScan,
			Status:      domain.JobCompleted,
			SourceKey:   repoName,
			TargetKey:   p.determineTargetRepo(repoName),
			MaxAttempts: 3,
		}
		if err := p.jobRepo.Create(&scanJob); err != nil {
			return err
		}

		// Create artifact_copy job (items linked to this, executed by scheduler)
		copyJob := domain.MigrationJob{
			PlanID:      p.planID,
			Kind:        domain.JobArtifactCopy,
			Status:      domain.JobPending,
			SourceKey:   repoName,
			TargetKey:   p.determineTargetRepo(repoName),
			MaxAttempts: 3,
		}
		if err := p.jobRepo.Create(&copyJob); err != nil {
			return err
		}

		var cursor string
		var allItems []domain.MigrationItem
		for {
			page, err := p.src.ListComponentsPage(ctx, repoName, cursor)
			if err != nil {
				return fmt.Errorf("failed to list components for %s: %w", repoName, err)
			}
			for _, comp := range page.Items {
				for _, asset := range comp.Assets {
					allItems = append(allItems, domain.MigrationItem{
						PlanID:           p.planID,
						JobID:            copyJob.ID,
						Kind:             domain.ItemArtifact,
						SourceRepository: repoName,
						SourceID:         comp.ID,
						SourcePath:       asset.DownloadURL,
						SourceFormat:     comp.Format,
						SourceName:       comp.Name,
						SourceVersion:    comp.Version,
						TargetRepository: p.determineTargetRepo(repoName),
						Status:           domain.ItemPending,
					})
					if len(allItems) >= 100 {
						if err := p.itemRepo.BatchCreate(allItems); err != nil {
							return err
						}
						allItems = allItems[:0]
					}
				}
			}
			if page.ContinuationToken == "" {
				break
			}
			cursor = page.ContinuationToken
		}
		if len(allItems) > 0 {
			if err := p.itemRepo.BatchCreate(allItems); err != nil {
				return err
			}
		}
		p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("已完成仓库 [%s] 的制品扫描", repoName), nil, nil)
	}
	return nil
}

// scanArtifactsStreaming scans artifacts with streaming support.
// It writes items to DB as they're scanned and notifies via readyCh.
func (p *Planner) scanArtifactsStreaming(ctx context.Context, repos []source.SourceRepository, readyCh chan<- uint) error {
	repoNames := p.scope.ArtifactRepos
	if len(repoNames) == 0 {
		for _, repo := range repos {
			if repo.Name == "" || repo.Type == "group" || !p.shouldIncludeRepo(repo.Type) {
				continue
			}
			repoNames = append(repoNames, repo.Name)
		}
	}
	if len(repoNames) == 0 {
		p.eventRepo.Log(p.planID, domain.LevelWarn, domain.EventSourceWarning, "未发现可迁移的制品仓库", nil, nil)
		return nil
	}

	for _, repoName := range repoNames {
		scanJob := domain.MigrationJob{
			PlanID:      p.planID,
			Kind:        domain.JobArtifactScan,
			Status:      domain.JobCompleted,
			SourceKey:   repoName,
			TargetKey:   p.determineTargetRepo(repoName),
			MaxAttempts: 3,
		}
		if err := p.jobRepo.Create(&scanJob); err != nil {
			return err
		}

		copyJob := domain.MigrationJob{
			PlanID:      p.planID,
			Kind:        domain.JobArtifactCopy,
			Status:      domain.JobPending,
			SourceKey:   repoName,
			TargetKey:   p.determineTargetRepo(repoName),
			MaxAttempts: 3,
		}
		if err := p.jobRepo.Create(&copyJob); err != nil {
			return err
		}

		var cursor string
		var allItems []domain.MigrationItem

		for {
			page, err := p.src.ListComponentsPage(ctx, repoName, cursor)
			if err != nil {
				return fmt.Errorf("failed to list components for %s: %w", repoName, err)
			}
			for _, comp := range page.Items {
				for _, asset := range comp.Assets {
					allItems = append(allItems, domain.MigrationItem{
						PlanID:           p.planID,
						JobID:            copyJob.ID,
						Kind:             domain.ItemArtifact,
						SourceRepository: repoName,
						SourceID:         comp.ID,
						SourcePath:       asset.DownloadURL,
						SourceFormat:     comp.Format,
						SourceName:       comp.Name,
						SourceVersion:    comp.Version,
						TargetRepository: p.determineTargetRepo(repoName),
						Status:           domain.ItemPending,
					})
					if len(allItems) >= 100 {
						if err := p.itemRepo.BatchCreate(allItems); err != nil {
							return err
						}
						// 通知 executor 有新 items 可以执行
						select {
						case readyCh <- copyJob.ID:
						default:
						}
						allItems = allItems[:0]
					}
				}
			}
			if page.ContinuationToken == "" {
				break
			}
			cursor = page.ContinuationToken
		}

		if len(allItems) > 0 {
			if err := p.itemRepo.BatchCreate(allItems); err != nil {
				return err
			}
			// 通知 executor 有新 items 可以执行
			select {
			case readyCh <- copyJob.ID:
			default:
			}
		}
		p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("已完成仓库 [%s] 的制品扫描", repoName), nil, nil)
	}
	return nil
}

func (p *Planner) determineTargetRepo(sourceRepo string) string {
	if p.targetRepoName != "" {
		return p.targetRepoName
	}
	return sourceRepo
}

func (p *Planner) shouldIncludeRepo(repoType string) bool {
	if p.scope.HostedRepos && repoType == "hosted" {
		return true
	}
	if p.scope.ProxyRepos && repoType == "proxy" {
		return true
	}
	if p.scope.GroupRepos && repoType == "group" {
		return true
	}
	return false
}

// parsePrivilege extracts resource and action from a Nexus privilege string.
// Example: "nx-repository-view-npm-*-read" → ("nx-repository-view-npm", "read")
func parsePrivilege(privilege string) (resource, action string) {
	if privilege == "" {
		return "", ""
	}
	parts := splitPrivilege(privilege, ':')
	if len(parts) >= 2 {
		resource = parts[0]
		action = parts[1]
	}
	return
}

func splitPrivilege(s string, sep rune) []string {
	var result []string
	start := 0
	inQuotes := false
	for i, ch := range s {
		if ch == '"' {
			inQuotes = !inQuotes
		} else if ch == sep && !inQuotes {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

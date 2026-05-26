package planner

import (
	"context"
	"fmt"

	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/repository"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/source"
)

// Planner builds migration jobs and items from source scanning.
type Planner struct {
	src          source.MigrationSource
	planID       uint
	scope        *domain.ScopeSelection
	jobRepo      *repository.JobRepo
	itemRepo     *repository.ItemRepo
	eventRepo    *repository.EventRepo
	targetRepoID uint
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
	p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged, "Scanning started", nil, nil)

	if p.scope.RepoConfig || p.scope.GroupMemberships {
		if err := p.scanRepositories(ctx); err != nil {
			return err
		}
	}

	if p.scope.Privileges || p.scope.Roles || p.scope.Users {
		if err := p.scanSecurity(ctx); err != nil {
			return err
		}
	}

	if p.scope.Artifacts && len(p.scope.ArtifactRepos) > 0 {
		if err := p.scanArtifacts(ctx); err != nil {
			return err
		}
	}

	p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged, "Scanning completed", nil, nil)
	return nil
}

func (p *Planner) scanRepositories(ctx context.Context) error {
	repos, err := p.src.ListRepositories(ctx)
	if err != nil {
		return fmt.Errorf("failed to list repositories: %w", err)
	}

	var jobs []domain.MigrationJob
	for _, repo := range repos {
		if repo.Format == "" || repo.Type == "" {
			continue
		}
		if !p.shouldIncludeRepo(repo.Type) {
			continue
		}

		detail, detailErr := p.src.GetRepositoryDetail(ctx, repo.Format, repo.Type, repo.Name)
		if detailErr != nil {
			p.eventRepo.Log(p.planID, domain.LevelWarn, domain.EventSourceWarning,
				fmt.Sprintf("Repository detail unavailable for %s (format=%s type=%s): %v", repo.Name, repo.Format, repo.Type, detailErr), nil, nil)
		}

		if p.scope.RepoConfig {
			jobs = append(jobs, domain.MigrationJob{
				PlanID:      p.planID,
				Kind:        domain.JobRepoConfig,
				Status:      domain.JobPending,
				SourceKey:   repo.Name,
				TargetKey:   repo.Name,
				MaxAttempts: 3,
			})
		}

		// Group membership
		if p.scope.GroupMemberships && repo.Type == "group" && detailErr == nil && detail != nil && detail.Group != nil {
			for _, memberName := range detail.Group.MemberNames {
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

	p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged,
		fmt.Sprintf("Found %d repositories", len(jobs)), nil, nil)

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
			fmt.Sprintf("Found %d privileges", len(permJobs)), nil, nil)
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
			jobs = append(jobs, domain.MigrationJob{
				PlanID:   p.planID,
				Kind:     domain.JobRole,
				Status:   domain.JobPending,
				SourceKey: role.ID,
				TargetKey: role.ID,
				MaxAttempts: 3,
			})
		}
		if len(jobs) > 0 {
			if err := p.jobRepo.BatchCreate(jobs); err != nil {
				return err
			}
		}
		p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("Found %d roles", len(jobs)), nil, nil)
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
			jobs = append(jobs, domain.MigrationJob{
				PlanID:   p.planID,
				Kind:     domain.JobUser,
				Status:   domain.JobPending,
				SourceKey: u.UserID,
				TargetKey: u.UserID,
				MaxAttempts: 3,
			})
		}
		if len(jobs) > 0 {
			if err := p.jobRepo.BatchCreate(jobs); err != nil {
				return err
			}
		}
		p.eventRepo.Log(p.planID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("Found %d users", len(jobs)), nil, nil)
	}

	return nil
}

func (p *Planner) scanArtifacts(ctx context.Context) error {
	for _, repoName := range p.scope.ArtifactRepos {
		// Create artifact_scan job
		scanJob := domain.MigrationJob{
			PlanID:   p.planID,
			Kind:     domain.JobArtifactScan,
			Status:   domain.JobPending,
			SourceKey: repoName,
			TargetKey: p.determineTargetRepo(repoName),
			MaxAttempts: 3,
		}
		if err := p.jobRepo.Create(&scanJob); err != nil {
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
				allItems = append(allItems, domain.MigrationItem{
					PlanID:           p.planID,
					JobID:            scanJob.ID,
					Kind:             domain.ItemArtifact,
					SourceRepository: repoName,
					SourceID:         comp.ID,
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
			fmt.Sprintf("Scanned repository %s", repoName), nil, nil)
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

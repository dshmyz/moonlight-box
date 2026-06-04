package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/core/runtime"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/domain"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/repository"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/source"
	"github.com/dshmyz/moonlight-box/internal/migration/v2/source/nexus"
	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/service"
	"github.com/dshmyz/moonlight-box/internal/util"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ExecutorManager dispatches jobs to the appropriate executor.
type ExecutorManager struct {
	db          *gorm.DB
	src         source.MigrationSource
	storageSvc  *service.StorageService
	itemRepo    *repository.ItemRepo
	eventRepo   *repository.EventRepo
	concurrency int
	artifactSvc *service.ArtifactService
}

func NewExecutorManager(db *gorm.DB, src source.MigrationSource, storageSvc *service.StorageService, itemRepo *repository.ItemRepo, eventRepo *repository.EventRepo, concurrency int, artifactSvc *service.ArtifactService) *ExecutorManager {
	return &ExecutorManager{
		db:          db,
		src:         src,
		storageSvc:  storageSvc,
		itemRepo:    itemRepo,
		eventRepo:   eventRepo,
		concurrency: concurrency,
		artifactSvc: artifactSvc,
	}
}

// SetSource sets the migration source for plan-specific execution.
func (m *ExecutorManager) SetSource(src source.MigrationSource) {
	m.src = src
}

// ExecuteJob dispatches a job to the correct executor based on its kind.
func (m *ExecutorManager) ExecuteJob(ctx context.Context, job *domain.MigrationJob) error {
	m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
		fmt.Sprintf("Executing job: %s/%s", job.Kind, job.SourceKey), &job.ID, nil)

	job.Status = domain.JobRunning
	if err := m.db.Save(job).Error; err != nil {
		return err
	}

	var err error
	switch job.Kind {
	case domain.JobRepoConfig:
		err = m.executeRepoConfig(ctx, job)
	case domain.JobGroupMembership:
		err = m.executeGroupMembership(ctx, job)
	case domain.JobPermission:
		err = m.executePermission(ctx, job)
	case domain.JobRole:
		err = m.executeRole(ctx, job)
	case domain.JobUser:
		err = m.executeUser(ctx, job)
	case domain.JobArtifactCopy:
		err = m.executeArtifactCopy(ctx, job)
	case domain.JobVerify:
		err = m.executeVerify(ctx, job)
	default:
		err = fmt.Errorf("unsupported job kind: %s", job.Kind)
	}

	if err != nil {
		job.Status = domain.JobFailed
		job.ErrorMessage = err.Error()
		m.eventRepo.Log(job.PlanID, domain.LevelError, domain.EventJobFailed,
			fmt.Sprintf("Job failed: %s/%s: %v", job.Kind, job.SourceKey, err), &job.ID, nil)
	} else {
		job.Status = domain.JobCompleted
		m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("Job completed: %s/%s", job.Kind, job.SourceKey), &job.ID, nil)
	}

	return m.db.Save(job).Error
}

func (m *ExecutorManager) executeRepoConfig(ctx context.Context, job *domain.MigrationJob) error {
	var count int64
	if err := m.db.Raw("SELECT count(*) FROM repositories WHERE name = ?", job.SourceKey).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("跳过已存在的仓库: %s", job.SourceKey), &job.ID, nil)
		return nil
	}

	repoType := "local"
	packageType := "generic"
	remoteURL := ""
	isVirtual := false

	if job.Checkpoint != "" {
		var cp domain.RepoCheckpoint
		if err := json.Unmarshal([]byte(job.Checkpoint), &cp); err == nil {
			repoType = cp.Type
			packageType = cp.Format
			remoteURL = cp.RemoteURL
			isVirtual = cp.IsVirtual
		}
	}

	repoType = nexus.MapRepositoryType(repoType)
	packageType = nexus.MapRepositoryFormat(packageType)

	repo := &model.Repository{
		Name:        job.SourceKey,
		Type:        model.RepositoryType(repoType),
		PackageType: packageType,
		Enabled:     true,
	}
	if repoType == "proxy" && remoteURL != "" {
		repo.Config = &model.RepositoryConfig{
			RemoteURL:       remoteURL,
			CacheEnabled:    true,
			CacheTTLSeconds: 86400,
		}
	}
	if isVirtual {
		repo.StorageBackendID = nil
	}
	if err := m.db.Create(repo).Error; err != nil {
		return fmt.Errorf("failed to create repository %s: %w", job.SourceKey, err)
	}
	m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
		fmt.Sprintf("Created repository: %s (%s/%s)", job.SourceKey, repoType, packageType), &job.ID, nil)
	return nil
}

func (m *ExecutorManager) executeGroupMembership(ctx context.Context, job *domain.MigrationJob) error {
	virtualName := job.SourceKey
	memberName := job.TargetKey

	var virtualRepo model.Repository
	if err := m.db.Where("name = ?", virtualName).First(&virtualRepo).Error; err != nil {
		job.ErrorCode = domain.ErrTargetGroupMemberMissing
		return fmt.Errorf("virtual repository %s not found", virtualName)
	}
	var memberRepo model.Repository
	if err := m.db.Where("name = ?", memberName).First(&memberRepo).Error; err != nil {
		job.ErrorCode = domain.ErrTargetGroupMemberMissing
		return fmt.Errorf("member repository %s not found", memberName)
	}

	var count int64
	if err := m.db.Model(&model.RepositoryMember{}).
		Where("repository_id = ? AND member_id = ?", virtualRepo.ID, memberRepo.ID).
		Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("跳过已存在的组成员关系: %s -> %s", virtualName, memberName), &job.ID, nil)
		return nil
	}

	member := model.RepositoryMember{
		RepositoryID: virtualRepo.ID,
		MemberID:     memberRepo.ID,
		Position:     0,
	}
	if err := m.db.Create(&member).Error; err != nil {
		return err
	}
	m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
		fmt.Sprintf("添加组成员: %s -> %s", virtualName, memberName), &job.ID, nil)
	return nil
}

func (m *ExecutorManager) executePermission(ctx context.Context, job *domain.MigrationJob) error {
	var count int64
	if err := m.db.Raw("SELECT count(*) FROM permissions WHERE resource = ? AND action = ?", job.SourceKey, job.TargetKey).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("跳过已存在的权限: %s:%s", job.SourceKey, job.TargetKey), &job.ID, nil)
		return nil
	}
	perm := &model.Permission{
		Resource: job.SourceKey,
		Action:   job.TargetKey,
	}
	if err := m.db.Where(model.Permission{Resource: job.SourceKey, Action: job.TargetKey}).FirstOrCreate(perm).Error; err != nil {
		return err
	}
	m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
		fmt.Sprintf("创建权限: %s:%s", job.SourceKey, job.TargetKey), &job.ID, nil)
	return nil
}

func (m *ExecutorManager) executeRole(ctx context.Context, job *domain.MigrationJob) error {
	var count int64
	if err := m.db.Raw("SELECT count(*) FROM roles WHERE name = ?", job.SourceKey).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("跳过已存在的角色: %s", job.SourceKey), &job.ID, nil)
		return nil
	}

	name := job.SourceKey
	description := "Migrated from source"

	if job.Checkpoint != "" {
		var cp domain.RoleCheckpoint
		if err := json.Unmarshal([]byte(job.Checkpoint), &cp); err == nil {
			if cp.Name != "" {
				name = cp.Name
			}
			if cp.Description != "" {
				description = cp.Description
			}
		}
	}

	role := &model.Role{
		Name:        name,
		Description: description,
	}
	if err := m.db.Create(role).Error; err != nil {
		return err
	}
	m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
		fmt.Sprintf("创建角色: %s", name), &job.ID, nil)
	return nil
}

func (m *ExecutorManager) executeUser(ctx context.Context, job *domain.MigrationJob) error {
	var count int64
	if err := m.db.Raw("SELECT count(*) FROM users WHERE username = ?", job.SourceKey).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("跳过已存在的用户: %s", job.SourceKey), &job.ID, nil)
		return nil
	}

	email := job.SourceKey + "@migrated.local"
	displayName := job.SourceKey

	// Use checkpoint data stored during the scanning phase
	if job.Checkpoint != "" {
		var cp domain.UserCheckpoint
		if err := json.Unmarshal([]byte(job.Checkpoint), &cp); err == nil {
			if cp.Email != "" {
				email = cp.Email
			}
			if cp.DisplayName != "" {
				displayName = cp.DisplayName
			}
		}
	}

	// Check email conflict and resolve
	var emailCount int64
	m.db.Raw("SELECT count(*) FROM users WHERE email = ?", email).Scan(&emailCount)
	if emailCount > 0 {
		// Extract email parts and create a valid migrated email
		parts := strings.Split(email, "@")
		if len(parts) == 2 {
			email = fmt.Sprintf("%s-migrated@%s", parts[0], parts[1])
		} else {
			email = fmt.Sprintf("%s-migrated@migrated.local", job.SourceKey)
		}
	}

	hashedPassword, _ := util.HashPassword(job.SourceKey)
	user := &model.User{
		Username:     job.SourceKey,
		PasswordHash: hashedPassword,
		Email:        email,
		DisplayName:  displayName,
		IsActive:     true,
	}
	if err := m.db.Create(user).Error; err != nil {
		return err
	}

	// Assign default role (developer) to migrated users
	var devRole model.Role
	if err := m.db.Where("name = ?", "developer").First(&devRole).Error; err == nil {
		userRole := model.UserRole{
			UserID:     user.ID,
			RoleID:     devRole.ID,
			AssignedBy: user.ID,
		}
		m.db.Create(&userRole)
	}

	m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
		fmt.Sprintf("创建用户: %s (%s)", job.SourceKey, email), &job.ID, nil)
	return nil
}

func (m *ExecutorManager) executeArtifactCopy(ctx context.Context, job *domain.MigrationJob) error {
	if m.src == nil {
		return fmt.Errorf("no source configured for artifact download")
	}

	items, err := m.itemRepo.ListByJob(job.ID)
	if err != nil {
		return err
	}

	// 统计跳过的 item
	var skippedCount int
	for _, item := range items {
		if item.Status != domain.ItemPending {
			skippedCount++
		}
	}
	if skippedCount > 0 {
		m.eventRepo.Log(job.PlanID, domain.LevelInfo, domain.EventStatusChanged,
			fmt.Sprintf("跳过 %d 个已处理的制品", skippedCount), &job.ID, nil)
	}

	sem := make(chan struct{}, m.concurrency)
	errCh := make(chan error, len(items))

	for i := range items {
		item := items[i]
		if item.Status != domain.ItemPending {
			continue
		}
		sem <- struct{}{}
		go func(it domain.MigrationItem) {
			defer func() { <-sem }()
			if execErr := m.executeItem(ctx, job.PlanID, &it); execErr != nil {
				errCh <- execErr
			}
		}(item)
	}

	for i := 0; i < m.concurrency; i++ {
		sem <- struct{}{}
	}
	close(errCh)

	for e := range errCh {
		if e != nil {
			return e
		}
	}
	return nil
}

func (m *ExecutorManager) executeItem(ctx context.Context, planID uint, item *domain.MigrationItem) error {
	item.Status = domain.ItemRunning
	m.db.Save(item)

	ns, ok := m.src.(*nexus.NexusSource)
	if !ok {
		return m.failItem(planID, item, domain.ErrArtifactDownloadFailed, "source is not a Nexus source")
	}

	assetStream, err := ns.DownloadAsset(ctx, item.SourcePath)
	if err != nil {
		return m.failItem(planID, item, domain.ErrArtifactDownloadFailed, err.Error())
	}
	defer assetStream.Reader.Close()

	if m.storageSvc == nil {
		return m.failItem(planID, item, domain.ErrArtifactStorageFailed, "storage service not configured")
	}

	hash := sha256.New()
	teeReader := io.TeeReader(assetStream.Reader, hash)

	targetRepo := item.TargetRepository
	if targetRepo == "" {
		targetRepo = item.SourceRepository
	}
	storageVersion := item.SourceVersion
	if item.SourceVersion == "" {
		storageVersion = "unknown"
	}
	storageVersion += "/" + filepath.Base(item.SourcePath)

	size := assetStream.Size
	storageKey, err := m.storageSvc.StorePackage(ctx, item.SourceFormat, item.SourceName, storageVersion, teeReader, size)
	if err != nil {
		return m.failItem(planID, item, domain.ErrArtifactStorageFailed, err.Error())
	}

	digest := hex.EncodeToString(hash.Sum(nil))

	// 先查询目标仓库 ID（blob 和 artifact 都需要）
	var repo model.Repository
	if err := m.db.Where("name = ?", targetRepo).First(&repo).Error; err != nil {
		return m.failItem(planID, item, domain.ErrArtifactStorageFailed, fmt.Sprintf("target repository not found: %s", targetRepo))
	}

	// Step 1: 创建 blob 记录
	// 注意：blob 和 artifact 不在同一个事务中，如果 Step 2 失败会产生孤儿 blob。
	// 这是有意的 tradeoff——让 ArtifactService 统一管理 artifact 写入 + packages 同步，
	// 而非拆分其内部事务。孤儿 blob 可通过定期清理任务回收。
	var blobID uint
	err = m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		blob := &model.Blob{
			Algorithm:   "sha256",
			Digest:      digest,
			Size:        size,
			StoragePath: storageKey,
		}
		if err := tx.Create(blob).Error; err != nil {
			return err
		}
		blobID = blob.ID
		return nil
	})
	if err != nil {
		return m.failItem(planID, item, domain.ErrArtifactStorageFailed, err.Error())
	}

	// Step 2: 通过 ArtifactService 创建 artifact + blob 关联 + 同步 packages 表
	runtimeArtifact := &runtime.Artifact{
		RepositoryID: fmt.Sprintf("%d", repo.ID),
		Format:       item.SourceFormat,
		Kind:         "primary",
		Coordinates: map[string]string{
			"name":    item.SourceName,
			"version": item.SourceVersion,
		},
		BlobRefs: []runtime.BlobRef{
			{BlobID: blobID},
		},
	}
	if err := m.artifactSvc.Save(ctx, runtimeArtifact); err != nil {
		return m.failItem(planID, item, domain.ErrArtifactStorageFailed, err.Error())
	}

	item.Status = domain.ItemCompleted
	item.ChecksumJSON = fmt.Sprintf(`{"sha256":"%s"}`, digest)
	item.SizeBytes = size
	if err := m.db.Save(item).Error; err != nil {
		return err
	}

	// 记录成功日志
	m.eventRepo.Log(planID, domain.LevelInfo, domain.EventItemCompleted,
		fmt.Sprintf("制品迁移成功: %s/%s (%s)", item.SourceRepository, item.SourcePath, item.SourceName), nil, &item.ID)
	return nil
}

func (m *ExecutorManager) failItem(planID uint, item *domain.MigrationItem, code domain.ErrorCode, msg string) error {
	item.Status = domain.ItemFailed
	item.ErrorCode = code
	item.ErrorMessage = msg
	m.db.Save(item)

	// 记录失败日志
	m.eventRepo.Log(planID, domain.LevelError, domain.EventJobFailed,
		fmt.Sprintf("制品迁移失败: %s/%s - %s: %s", item.SourceRepository, item.SourcePath, code, msg), nil, &item.ID)

	logrus.WithFields(logrus.Fields{
		"plan_id":     planID,
		"item_id":     item.ID,
		"source_path": item.SourcePath,
		"error_code":  code,
		"error":       msg,
	}).Error("Migration item failed")

	return fmt.Errorf("%s: %s", code, msg)
}

func (m *ExecutorManager) executeVerify(ctx context.Context, job *domain.MigrationJob) error {
	stats, err := m.itemRepo.CountByPlanAndStatus(job.PlanID, domain.ItemFailed)
	if err != nil {
		return err
	}
	if stats > 0 {
		logrus.WithFields(logrus.Fields{"plan_id": job.PlanID, "failed": stats}).Warn("Verification found failed items")
	}
	return nil
}

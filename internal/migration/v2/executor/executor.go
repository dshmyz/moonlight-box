package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"

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
}

func NewExecutorManager(db *gorm.DB, src source.MigrationSource, storageSvc *service.StorageService, itemRepo *repository.ItemRepo, eventRepo *repository.EventRepo, concurrency int) *ExecutorManager {
	return &ExecutorManager{
		db:          db,
		src:         src,
		storageSvc:  storageSvc,
		itemRepo:    itemRepo,
		eventRepo:   eventRepo,
		concurrency: concurrency,
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
		return nil
	}

	repoType := "local"
	packageType := "generic"
	remoteURL := ""

	if m.src != nil {
		repos, err := m.src.ListRepositories(ctx)
		if err == nil {
			for _, r := range repos {
				if r.Name == job.SourceKey {
					packageType = r.Format
					repoType = nexus.MapRepositoryType(r.Type)
					// Get detail for proxy repos to get remote URL
					if r.Type == "proxy" {
						detail, detailErr := m.src.GetRepositoryDetail(ctx, r.Format, r.Type, r.Name)
						if detailErr == nil && detail.Proxy != nil {
							remoteURL = detail.Proxy.RemoteURL
						} else if r.URL != "" {
							remoteURL = r.URL
						}
					}
					break
				}
			}
		}
	}

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
	if repoType == "virtual" {
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
		return nil
	}

	member := model.RepositoryMember{
		RepositoryID: virtualRepo.ID,
		MemberID:     memberRepo.ID,
		Position:     0,
	}
	return m.db.Create(&member).Error
}

func (m *ExecutorManager) executePermission(ctx context.Context, job *domain.MigrationJob) error {
	var count int64
	if err := m.db.Raw("SELECT count(*) FROM permissions WHERE resource = ? AND action = ?", job.SourceKey, job.TargetKey).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	perm := &model.Permission{
		Resource: job.SourceKey,
		Action:   job.TargetKey,
	}
	return m.db.Where(model.Permission{Resource: job.SourceKey, Action: job.TargetKey}).FirstOrCreate(perm).Error
}

func (m *ExecutorManager) executeRole(ctx context.Context, job *domain.MigrationJob) error {
	var count int64
	if err := m.db.Raw("SELECT count(*) FROM roles WHERE name = ?", job.SourceKey).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	role := &model.Role{
		Name:        job.SourceKey,
		Description: "Migrated from source",
	}
	return m.db.Create(role).Error
}

func (m *ExecutorManager) executeUser(ctx context.Context, job *domain.MigrationJob) error {
	var count int64
	if err := m.db.Raw("SELECT count(*) FROM users WHERE username = ?", job.SourceKey).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	email := job.SourceKey + "@migrated.local"
	displayName := job.SourceKey

	// Try to get actual user data from source
	if m.src != nil {
		users, err := m.src.ListUsers(ctx)
		if err == nil {
			for _, u := range users {
				if u.UserID == job.SourceKey {
					if u.Email != "" {
						email = u.Email
					}
					if u.FirstName != "" || u.LastName != "" {
						displayName = u.FirstName + " " + u.LastName
					}
					break
				}
			}
		}
	}

	// Check email conflict and resolve
	var emailCount int64
	m.db.Raw("SELECT count(*) FROM users WHERE email = ?", email).Scan(&emailCount)
	if emailCount > 0 {
		email = fmt.Sprintf("%s-migrated-%s", job.SourceKey, email)
	}

	hashedPassword, _ := util.HashPassword(util.GenerateRandomString(16))
	user := &model.User{
		Username:     job.SourceKey,
		PasswordHash: hashedPassword,
		Email:        email,
		DisplayName:  displayName,
		IsActive:     true,
	}
	return m.db.Create(user).Error
}

func (m *ExecutorManager) executeArtifactCopy(ctx context.Context, job *domain.MigrationJob) error {
	if m.src == nil {
		return fmt.Errorf("no source configured for artifact download")
	}

	items, err := m.itemRepo.ListByJob(job.ID)
	if err != nil {
		return err
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
			if execErr := m.executeItem(ctx, &it); execErr != nil {
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

func (m *ExecutorManager) executeItem(ctx context.Context, item *domain.MigrationItem) error {
	item.Status = domain.ItemRunning
	m.db.Save(item)

	ns, ok := m.src.(*nexus.NexusSource)
	if !ok {
		return m.failItem(item, domain.ErrArtifactDownloadFailed, "source is not a Nexus source")
	}

	assetStream, err := ns.DownloadAsset(ctx, item.SourcePath)
	if err != nil {
		return m.failItem(item, domain.ErrArtifactDownloadFailed, err.Error())
	}
	defer assetStream.Reader.Close()

	if m.storageSvc == nil {
		return m.failItem(item, domain.ErrArtifactStorageFailed, "storage service not configured")
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
		return m.failItem(item, domain.ErrArtifactStorageFailed, err.Error())
	}

	digest := hex.EncodeToString(hash.Sum(nil))

	// Store blob + artifact atomically
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

		// Find target repo ID
		var repo model.Repository
		if err := tx.Where("name = ?", targetRepo).First(&repo).Error; err != nil {
			return fmt.Errorf("target repository not found: %s", targetRepo)
		}

		coords := model.JSONB{
			"name":    item.SourceName,
			"version": item.SourceVersion,
		}

		artifact := &model.Artifact{
			RepositoryID: repo.ID,
			Format:       item.SourceFormat,
			Kind:         "primary",
			Coordinates:  coords,
		}
		if err := tx.Create(artifact).Error; err != nil {
			return err
		}

		ab := &model.ArtifactBlob{
			ArtifactID: artifact.ID,
			BlobID:     blob.ID,
			Position:   0,
			Role:       "primary",
		}
		return tx.Create(ab).Error
	})
	if err != nil {
		return m.failItem(item, domain.ErrArtifactStorageFailed, err.Error())
	}

	item.Status = domain.ItemCompleted
	item.ChecksumJSON = fmt.Sprintf(`{"sha256":"%s"}`, digest)
	item.SizeBytes = size
	return m.db.Save(item).Error
}

func (m *ExecutorManager) failItem(item *domain.MigrationItem, code domain.ErrorCode, msg string) error {
	item.Status = domain.ItemFailed
	item.ErrorCode = code
	item.ErrorMessage = msg
	m.db.Save(item)
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

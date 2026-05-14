package migration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/sirupsen/logrus"
)

type MigrationWorkerV2 struct {
	service     *MigrationService
	storageSvc  *service.StorageService
	pkgRepo     *repository.PackageRepository
	repoRepo    *repository.RepositoryRepository
	itemRepo    *repository.MigrationItemRepository
	concurrency int
	maxRetries  int
	batchSize   int
}

type migrationTarget struct {
	repoID    uint
	repoName  string
	backendID uint
}

func NewMigrationWorkerV2(
	migrationSvc *MigrationService,
	storageSvc *service.StorageService,
	pkgRepo *repository.PackageRepository,
	repoRepo *repository.RepositoryRepository,
	itemRepo *repository.MigrationItemRepository,
	concurrency int,
	maxRetries int,
	batchSize int,
) *MigrationWorkerV2 {
	return &MigrationWorkerV2{
		service:     migrationSvc,
		storageSvc:  storageSvc,
		pkgRepo:     pkgRepo,
		repoRepo:    repoRepo,
		itemRepo:    itemRepo,
		concurrency: concurrency,
		maxRetries:  maxRetries,
		batchSize:   batchSize,
	}
}

func (w *MigrationWorkerV2) SetService(service *MigrationService) {
	w.service = service
}

func (w *MigrationWorkerV2) Execute(ctx context.Context, task *model.MigrationTask) error {
	return w.execute(ctx, task, false)
}

func (w *MigrationWorkerV2) RetryFailed(ctx context.Context, task *model.MigrationTask) error {
	return w.execute(ctx, task, true)
}

func (w *MigrationWorkerV2) loadFailedItems(ctx context.Context, taskID uint, maxRetries int, queue *ComponentQueue) error {
	go w.streamPendingItems(ctx, taskID, maxRetries, queue)
	return nil
}

// streamPendingItems 分批加载待处理项并推送到队列，避免一次性加载太多数据
// 使用游标分页（id > lastID）避免 offset 分页在 consumers 并发更新状态时跳过记录
func (w *MigrationWorkerV2) streamPendingItems(ctx context.Context, taskID uint, maxRetries int, queue *ComponentQueue) {
	defer queue.Close()

	const batchSize = 50
	var lastID uint
	totalLoaded := 0

	for {
		select {
		case <-ctx.Done():
			logrus.WithFields(logrus.Fields{
				"module":  "migration",
				"task_id": taskID,
			}).Warn("Stream pending items cancelled")
			return
		default:
		}

		items, err := w.itemRepo.GetPendingItemsAfterID(taskID, maxRetries, batchSize, lastID)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"module":  "migration",
				"task_id": taskID,
				"error":   err,
			}).Error("Failed to load pending items")
			return
		}

		if len(items) == 0 {
			break
		}

		for _, item := range items {
			queue.Push(item)
		}

		totalLoaded += len(items)
		lastID = items[len(items)-1].ID

		logrus.WithFields(logrus.Fields{
			"module":     "migration",
			"task_id":    taskID,
			"batch":      len(items),
			"total":      totalLoaded,
			"queue_size": queue.Len(),
		}).Debug("Streamed batch of pending items")
	}

	logrus.WithFields(logrus.Fields{
		"module":  "migration",
		"task_id": taskID,
		"total":   totalLoaded,
	}).Info("Streamed all pending items to queue")
}

func (w *MigrationWorkerV2) execute(ctx context.Context, task *model.MigrationTask, retryOnly bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w.service.RegisterContext(task.ID, ctx, cancel)

	if retryOnly {
		logrus.WithFields(logrus.Fields{
			"module":      "migration",
			"task_id":     task.ID,
			"concurrency": w.concurrency,
			"max_retries": w.maxRetries,
		}).Info("Retry failed migration items started (v2)")
	} else {
		logrus.WithFields(logrus.Fields{
			"module":      "migration",
			"task_id":     task.ID,
			"source_url":  task.SourceURL,
			"phase":       task.Phase,
			"concurrency": w.concurrency,
			"max_retries": w.maxRetries,
			"batch_size":  w.batchSize,
		}).Info("Migration task started (v2)")
	}

	client := NewNexusClient(task.SourceURL, task.Username, mustGetPasswordV2(task))
	w.service.RegisterNexusClient(task.ID, client)

	target := w.resolveMigrationTarget(task)

	now := time.Now()
	startedAt := &now
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":     model.MigrationRunning,
		"started_at": startedAt,
	})

	queue := NewComponentQueue(w.batchSize * 10)
	progressUpdater := NewProgressUpdater(task.ID, w.service.db, w.itemRepo, 2*time.Second)

	if retryOnly {
		if err := w.loadFailedItems(ctx, task.ID, task.MaxRetries, queue); err != nil {
			logrus.WithFields(logrus.Fields{
				"module":  "migration",
				"task_id": task.ID,
				"error":   err,
			}).Error("Failed to load failed items")
			return w.failTask(task.ID, fmt.Sprintf("failed to load failed items: %v", err))
		}
		queue.Close()
	} else {
		var selectedRepos []string
		if err := json.Unmarshal([]byte(task.SelectedRepos), &selectedRepos); err != nil {
			logrus.WithFields(logrus.Fields{
				"module":  "migration",
				"task_id": task.ID,
				"error":   err,
			}).Error("Failed to parse selected repos")
			return w.failTask(task.ID, fmt.Sprintf("failed to parse selected repos: %v", err))
		}

		if task.Phase == model.PhaseScanning {
			w.service.AddLog(task.ID, "正在扫描组件...")
			totalScanned := 0
			for _, repoName := range selectedRepos {
				count, err := w.scanRepository(ctx, task.ID, client, repoName)
				if err != nil {
					w.service.AddLog(task.ID, fmt.Sprintf("扫描 %s 失败: %v", repoName, err))
					return w.failTask(task.ID, fmt.Sprintf("扫描仓库 %s 失败: %v", repoName, err))
				}
				totalScanned += count
			}

			w.service.AddLog(task.ID, fmt.Sprintf("扫描完成，共发现 %d 个组件", totalScanned))
			w.service.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
				"total_items": totalScanned,
				"phase":       model.PhaseScanned,
			})
		} else {
			w.service.AddLog(task.ID, "跳过扫描阶段，直接开始迁移")
		}

		w.service.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Update("phase", model.PhaseMigrating)

		// 分批加载待处理项，每次 50 个，避免一次性加载太多数据到内存
		go w.streamPendingItems(ctx, task.ID, task.MaxRetries, queue)
	}

	go progressUpdater.Start(ctx)

	var consumerWg sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		consumerWg.Add(1)
		go func(workerID int) {
			defer consumerWg.Done()
			w.consumeComponents(ctx, task.ID, client, queue, progressUpdater, workerID, target)
		}(i)
	}

	consumerWg.Wait()
	progressUpdater.Stop()

	w.completeTask(task.ID)
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Update("phase", model.PhaseDone)

	if retryOnly {
		logrus.WithFields(logrus.Fields{
			"module":  "migration",
			"task_id": task.ID,
		}).Info("Retry failed migration items completed (v2)")
		w.service.AddLog(task.ID, "重试失败项目完成")
	} else {
		logrus.WithFields(logrus.Fields{
			"module":  "migration",
			"task_id": task.ID,
		}).Info("Migration task completed (v2)")
		w.service.AddLog(task.ID, "迁移任务完成")
	}
	return nil
}

func (w *MigrationWorkerV2) resolveMigrationTarget(task *model.MigrationTask) migrationTarget {
	target := migrationTarget{
		repoID:   task.TargetRepositoryID,
		repoName: task.TargetRepository,
	}

	if task.TargetRepositoryID == 0 || w.repoRepo == nil {
		if target.repoName == "" {
			logrus.WithFields(logrus.Fields{
				"module":  "migration",
				"task_id": task.ID,
			}).Warn("Migration task has no target repository; storage keys will use default repository namespace")
		}
		return target
	}

	targetRepo, err := w.repoRepo.FindByID(task.TargetRepositoryID)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":           "migration",
			"task_id":          task.ID,
			"target_repo_id":   task.TargetRepositoryID,
			"target_repo_name": task.TargetRepository,
			"error":            err,
		}).Warn("Failed to get target repository, falling back to task target repository name")
		return target
	}

	target.repoName = targetRepo.Name
	if targetRepo.StorageBackendID != nil {
		target.backendID = *targetRepo.StorageBackendID
		logrus.WithFields(logrus.Fields{
			"module":         "migration",
			"task_id":        task.ID,
			"target_repo_id": task.TargetRepositoryID,
			"target_repo":    target.repoName,
			"backend_id":     target.backendID,
		}).Info("Using target repository's storage backend (v2)")
	} else {
		logrus.WithFields(logrus.Fields{
			"module":           "migration",
			"task_id":          task.ID,
			"target_repo_name": target.repoName,
		}).Info("Target repository has no storage backend configured, using default (v2)")
	}

	return target
}

func (w *MigrationWorkerV2) scanRepository(ctx context.Context, taskID uint, client *NexusClient, repoName string) (int, error) {
	w.service.AddLog(taskID, fmt.Sprintf("正在扫描: %s", repoName))
	logrus.WithFields(logrus.Fields{
		"module":    "migration",
		"task_id":   taskID,
		"repo_name": repoName,
	}).Info("Scanning repository")

	var continuationToken string
	totalComponents := 0
	var allItems []model.MigrationItem

	for {
		select {
		case <-ctx.Done():
			return totalComponents, fmt.Errorf("扫描被取消")
		default:
		}

		components, nextToken, err := client.ListComponentsPage(ctx, repoName, continuationToken)
		if err != nil {
			return totalComponents, err
		}

		for _, comp := range components {
			item := model.MigrationItem{
				TaskID:         taskID,
				Repository:     repoName,
				ComponentID:    comp.ID,
				ComponentName:  comp.Name,
				ComponentGroup: comp.Group,
				Version:        comp.Version,
				Format:         comp.Format,
				Status:         model.MigrationItemPending,
			}
			allItems = append(allItems, item)
			totalComponents++

			if len(allItems) >= w.batchSize {
				if err := w.itemRepo.BatchCreate(allItems); err != nil {
					logrus.WithError(err).Error("Failed to batch create migration items")
				}
				allItems = allItems[:0]
			}
		}

		if nextToken == "" {
			break
		}
		continuationToken = nextToken
	}

	if len(allItems) > 0 {
		if err := w.itemRepo.BatchCreate(allItems); err != nil {
			logrus.WithError(err).Error("Failed to batch create remaining migration items")
		}
	}

	logrus.WithFields(logrus.Fields{
		"module":    "migration",
		"task_id":   taskID,
		"repo_name": repoName,
		"total":     totalComponents,
	}).Info("Repository scan completed")

	return totalComponents, nil
}

func (w *MigrationWorkerV2) produceComponents(ctx context.Context, taskID uint, client *NexusClient, repoName string, queue *ComponentQueue) {
	w.service.AddLog(taskID, fmt.Sprintf("开始扫描仓库: %s", repoName))
	logrus.WithFields(logrus.Fields{
		"module":    "migration",
		"task_id":   taskID,
		"repo_name": repoName,
	}).Info("Starting repository scan")

	var continuationToken string
	totalComponents := 0
	var allItems []model.MigrationItem

	for {
		select {
		case <-ctx.Done():
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   taskID,
				"repo_name": repoName,
			}).Warn("Producer cancelled")
			return
		default:
		}

		components, nextToken, err := client.ListComponentsPage(ctx, repoName, continuationToken)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   taskID,
				"repo_name": repoName,
				"error":     err,
			}).Error("Failed to list components page")
			w.service.AddLog(taskID, fmt.Sprintf("获取 %s 组件列表失败: %v", repoName, err))
			return
		}

		for _, comp := range components {
			item := model.MigrationItem{
				TaskID:         taskID,
				Repository:     repoName,
				ComponentID:    comp.ID,
				ComponentName:  comp.Name,
				ComponentGroup: comp.Group,
				Version:        comp.Version,
				Format:         comp.Format,
				Status:         model.MigrationItemPending,
			}
			allItems = append(allItems, item)
			totalComponents++

			if len(allItems) >= w.batchSize {
				if err := w.itemRepo.BatchCreate(allItems); err != nil {
					logrus.WithFields(logrus.Fields{
						"module":    "migration",
						"task_id":   taskID,
						"repo_name": repoName,
						"error":     err,
					}).Error("Failed to batch create migration items")
				} else {
					for _, item := range allItems {
						if !queue.TryPush(item) {
							logrus.WithFields(logrus.Fields{
								"module":     "migration",
								"task_id":    taskID,
								"queue_size": queue.Len(),
							}).Warn("Queue full, waiting...")
							queue.Push(item)
						}
					}
				}
				allItems = allItems[:0]
			}
		}

		w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).
			Update("total_items", w.service.db.Raw("total_items + ?", len(components)))

		if nextToken == "" {
			break
		}
		continuationToken = nextToken
	}

	if len(allItems) > 0 {
		if err := w.itemRepo.BatchCreate(allItems); err != nil {
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   taskID,
				"repo_name": repoName,
				"error":     err,
			}).Error("Failed to batch create remaining migration items")
		} else {
			for _, item := range allItems {
				queue.Push(item)
			}
		}
	}

	w.service.AddLog(taskID, fmt.Sprintf("仓库 %s 共发现 %d 个组件", repoName, totalComponents))
	logrus.WithFields(logrus.Fields{
		"module":     "migration",
		"task_id":    taskID,
		"repo_name":  repoName,
		"components": totalComponents,
	}).Info("Repository scan completed")
}

func (w *MigrationWorkerV2) consumeComponents(
	ctx context.Context,
	taskID uint,
	client *NexusClient,
	queue *ComponentQueue,
	progressUpdater *ProgressUpdater,
	workerID int,
	target migrationTarget,
) {
	for {
		select {
		case <-ctx.Done():
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   taskID,
				"worker_id": workerID,
			}).Info("Consumer stopped")
			return
		default:
		}

		item, ok := queue.Pop()
		if !ok {
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   taskID,
				"worker_id": workerID,
			}).Debug("Queue closed, consumer exiting")
			return
		}

		if item.RetryCount >= w.maxRetries {
			logrus.WithFields(logrus.Fields{
				"module":       "migration",
				"task_id":      taskID,
				"component_id": item.ComponentID,
				"retry_count":  item.RetryCount,
			}).Warn("Max retries exceeded, marking as failed")
			w.itemRepo.UpdateStatus(item.ID, model.MigrationItemFailed, "max retries exceeded")
			progressUpdater.IncrementFailed()
			continue
		}

		progressUpdater.UpdateItemStatus(item.ID, model.MigrationItemProcessing, "")

		comp := NexusComponent{
			ID:         item.ComponentID,
			Name:       item.ComponentName,
			Group:      item.ComponentGroup,
			Version:    item.Version,
			Format:     item.Format,
			Repository: item.Repository,
		}

		if err := w.migrateComponentWithRetry(ctx, taskID, client, comp, item.ID, item.RetryCount, target); err != nil {
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   taskID,
				"component": item.ComponentName,
				"version":   item.Version,
				"worker_id": workerID,
				"error":     err,
			}).Warn("Failed to migrate component")
			w.service.AddLog(taskID, fmt.Sprintf("迁移 %s (v%s) 失败: %v", item.ComponentName, item.Version, err))
			progressUpdater.UpdateItemStatus(item.ID, model.MigrationItemFailed, err.Error())
			progressUpdater.IncrementFailed()
		} else {
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   taskID,
				"component": item.ComponentName,
				"version":   item.Version,
				"worker_id": workerID,
			}).Info("Component migrated successfully")
			progressUpdater.UpdateItemStatus(item.ID, model.MigrationItemCompleted, "")
			progressUpdater.IncrementProcessed()
		}
	}
}

func (w *MigrationWorkerV2) migrateComponentWithRetry(ctx context.Context, taskID uint, client *NexusClient, comp NexusComponent, itemID uint, retryCount int, target migrationTarget) error {
	targetComp, err := client.GetComponentByID(ctx, comp.ID)
	if err != nil {
		return fmt.Errorf("failed to get component details: %w", err)
	}

	return w.migrateComponent(ctx, taskID, client, *targetComp, target)
}

func (w *MigrationWorkerV2) migrateComponent(ctx context.Context, taskID uint, client *NexusClient, comp NexusComponent, target migrationTarget) error {
	for _, asset := range comp.Assets {
		if asset.DownloadURL == "" {
			continue
		}

		reader, contentType, contentLength, err := client.DownloadAsset(ctx, asset.DownloadURL)
		if err != nil {
			return fmt.Errorf("download asset failed: %w", err)
		}

		size := contentLength
		if size < 0 && asset.FileSize > 0 {
			size = asset.FileSize
		}

		assetReader := io.Reader(reader)
		cleanup := func() { reader.Close() }
		if size < 0 {
			tmpFile, tmpSize, tmpErr := spoolReaderToTempFile(reader)
			reader.Close()
			if tmpErr != nil {
				return fmt.Errorf("failed to spool asset with unknown size: %w", tmpErr)
			}
			assetReader = tmpFile
			size = tmpSize
			cleanup = func() {
				tmpFile.Close()
				os.Remove(tmpFile.Name())
			}
		}

		err = w.storeAsset(taskID, comp, asset, assetReader, size, contentType, target)
		cleanup()
		if err != nil {
			return fmt.Errorf("store asset failed: %w", err)
		}
	}

	return nil
}

func spoolReaderToTempFile(reader io.Reader) (*os.File, int64, error) {
	tmpFile, err := os.CreateTemp("", "moonlight-migration-*")
	if err != nil {
		return nil, 0, err
	}

	size, copyErr := io.Copy(tmpFile, reader)
	if copyErr != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, 0, copyErr
	}
	if _, seekErr := tmpFile.Seek(0, 0); seekErr != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, 0, seekErr
	}

	return tmpFile, size, nil
}

func (w *MigrationWorkerV2) storeAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64, _ string, target migrationTarget) error {
	switch comp.Format {
	case "maven2":
		return w.storeMavenAsset(taskID, comp, asset, reader, size, target)
	case "npm":
		return w.storeNpmAsset(taskID, comp, asset, reader, size, target)
	case "pypi":
		return w.storePypiAsset(taskID, comp, asset, reader, size, target)
	case "go":
		return w.storeGoAsset(taskID, comp, asset, reader, size, target)
	case "raw":
		return w.storeGenericAsset(taskID, comp, asset, reader, size, target)
	default:
		return w.storeGenericAsset(taskID, comp, asset, reader, size, target)
	}
}

func (w *MigrationWorkerV2) storeMavenAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64, target migrationTarget) error {
	packaging := getPackaging(asset.Path)
	storageName := groupArtifactToStorageName(comp.Group, comp.Name)
	storageVersion := comp.Version + "/" + filepath.Base(asset.Path)

	storageKey, err := w.storageSvc.StorePackageWithBackend(context.Background(), target.repoName, "maven", storageName, storageVersion, reader, size, target.backendID)
	if err != nil {
		return err
	}

	metadata := map[string]any{
		"groupId":    comp.Group,
		"artifactId": comp.Name,
		"version":    comp.Version,
		"packaging":  packaging,
		"filename":   filepath.Base(asset.Path),
	}

	repoType := model.RepoTypeLocal

	_, _, _, err = w.pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:           comp.Group + ":" + comp.Name,
		Type:           model.PackageTypeMaven,
		RepositoryID:   target.repoID,
		RepositoryType: repoType,
		Description:    comp.Name,
	}, &model.PackageVersion{
		Version:  comp.Version,
		Status:   model.StatusPublished,
		Metadata: marshalMetadata(metadata),
	}, &model.PackageFile{
		Filename:    filepath.Base(asset.Path),
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   size,
	})

	return err
}

func (w *MigrationWorkerV2) storeNpmAsset(taskID uint, comp NexusComponent, _ NexusAsset, reader io.Reader, size int64, target migrationTarget) error {
	name := comp.Name
	if comp.Group != "" {
		name = comp.Group + "/" + comp.Name
	}
	version := comp.Version
	if version == "" {
		version = "1.0.0"
	}

	storageVersion := version + "/package.tgz"
	storageKey, err := w.storageSvc.StorePackageWithBackend(context.Background(), target.repoName, "npm", name, storageVersion, reader, size, target.backendID)
	if err != nil {
		return err
	}

	repoType := model.RepoTypeLocal

	_, _, _, err = w.pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:           name,
		Type:           model.PackageTypeNPM,
		RepositoryID:   target.repoID,
		RepositoryType: repoType,
		Description:    name,
	}, &model.PackageVersion{
		Version: version,
		Status:  model.StatusPublished,
	}, &model.PackageFile{
		Filename:    "package.tgz",
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   size,
	})

	return err
}

func (w *MigrationWorkerV2) storePypiAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64, target migrationTarget) error {
	name := comp.Name
	if comp.Group != "" {
		name = comp.Group + "/" + comp.Name
	}
	version := comp.Version
	if version == "" {
		version = "1.0.0"
	}

	storageVersion := version + "/" + filepath.Base(asset.Path)
	storageKey, err := w.storageSvc.StorePackageWithBackend(context.Background(), target.repoName, "pypi", name, storageVersion, reader, size, target.backendID)
	if err != nil {
		return err
	}

	repoType := model.RepoTypeLocal

	_, _, _, err = w.pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:           name,
		Type:           model.PackageTypePyPI,
		RepositoryID:   target.repoID,
		RepositoryType: repoType,
		Description:    name,
	}, &model.PackageVersion{
		Version: version,
		Status:  model.StatusPublished,
	}, &model.PackageFile{
		Filename:    filepath.Base(asset.Path),
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   size,
	})

	return err
}

func (w *MigrationWorkerV2) storeGoAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64, target migrationTarget) error {
	name := comp.Name
	if comp.Group != "" {
		name = comp.Group + "/" + comp.Name
	}
	version := comp.Version
	if version == "" {
		version = "v1.0.0"
	}

	storageVersion := version + "/" + filepath.Base(asset.Path)
	storageKey, err := w.storageSvc.StorePackageWithBackend(context.Background(), target.repoName, "go", name, storageVersion, reader, size, target.backendID)
	if err != nil {
		return err
	}

	repoType := model.RepoTypeLocal

	_, _, _, err = w.pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:           name,
		Type:           model.PackageTypeGo,
		RepositoryID:   target.repoID,
		RepositoryType: repoType,
		Description:    name,
	}, &model.PackageVersion{
		Version: version,
		Status:  model.StatusPublished,
	}, &model.PackageFile{
		Filename:    filepath.Base(asset.Path),
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   size,
	})

	return err
}

func (w *MigrationWorkerV2) storeGenericAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64, target migrationTarget) error {
	path := asset.Path
	if path == "" {
		path = comp.Name + "/" + filepath.Base(asset.DownloadURL)
	}

	storageVersion := filepath.Base(path)
	storageKey, err := w.storageSvc.StorePackageWithBackend(context.Background(), target.repoName, "generic", filepath.Dir(path), storageVersion, reader, size, target.backendID)
	if err != nil {
		return err
	}

	repoType := model.RepoTypeLocal

	_, _, _, err = w.pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:           path,
		Type:           model.PackageTypeGeneric,
		RepositoryID:   target.repoID,
		RepositoryType: repoType,
		Description:    path,
	}, &model.PackageVersion{
		Version: "1.0.0",
		Status:  model.StatusPublished,
	}, &model.PackageFile{
		Filename:    filepath.Base(path),
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   size,
	})

	return err
}

func (w *MigrationWorkerV2) failTask(taskID uint, errMsg string) error {
	return w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":        model.MigrationFailed,
		"error_message": errMsg,
		"completed_at":  time.Now(),
	}).Error
}

func (w *MigrationWorkerV2) completeTask(taskID uint) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":       model.MigrationCompleted,
		"completed_at": time.Now(),
	})
}

func mustGetPasswordV2(task *model.MigrationTask) string {
	password, _ := task.GetPassword()
	return password
}

func getPackaging(path string) string {
	ext := filepath.Ext(path)
	switch ext {
	case ".jar":
		return "jar"
	case ".war":
		return "war"
	case ".pom":
		return "pom"
	case ".ear":
		return "ear"
	case ".zip":
		return "zip"
	default:
		return strings.TrimPrefix(ext, ".")
	}
}

func groupArtifactToStorageName(group, artifact string) string {
	return strings.ReplaceAll(group, ".", "/") + "/" + artifact
}

func marshalMetadata(meta map[string]interface{}) string {
	if meta == nil {
		return ""
	}
	data, _ := json.Marshal(meta)
	return string(data)
}

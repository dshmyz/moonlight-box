package migration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/sirupsen/logrus"
)

type MigrationWorkerV2 struct {
	service         *MigrationService
	storageSvc      *service.StorageService
	pkgRepo         *repository.PackageRepository
	repoRepo        *repository.RepositoryRepository
	itemRepo        *repository.MigrationItemRepository
	concurrency     int
	maxRetries      int
	batchSize       int
	targetBackendID uint
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

func (w *MigrationWorkerV2) Execute(ctx context.Context, task *model.MigrationTask) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w.service.RegisterContext(task.ID, ctx, cancel)

	logrus.WithFields(logrus.Fields{
		"module":      "migration",
		"task_id":     task.ID,
		"source_url":  task.SourceURL,
		"concurrency": w.concurrency,
		"max_retries": w.maxRetries,
		"batch_size":  w.batchSize,
	}).Info("Migration task started (v2)")

	client := NewNexusClient(task.SourceURL, task.Username, mustGetPasswordV2(task))
	w.service.RegisterNexusClient(task.ID, client)

	if task.TargetRepositoryID > 0 && w.repoRepo != nil {
		targetRepo, err := w.repoRepo.FindByID(task.TargetRepositoryID)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"module":           "migration",
				"task_id":          task.ID,
				"target_repo_id":   task.TargetRepositoryID,
				"target_repo_name": task.TargetRepository,
				"error":            err,
			}).Warn("Failed to get target repository, using default storage backend")
		} else {
			if targetRepo.StorageBackendID != nil {
				w.targetBackendID = *targetRepo.StorageBackendID
				logrus.WithFields(logrus.Fields{
					"module":         "migration",
					"task_id":        task.ID,
					"target_repo_id": task.TargetRepositoryID,
					"backend_id":     w.targetBackendID,
				}).Info("Using target repository's storage backend (v2)")
			} else {
				logrus.WithFields(logrus.Fields{
					"module":           "migration",
					"task_id":          task.ID,
					"target_repo_name": task.TargetRepository,
				}).Info("Target repository has no storage backend configured, using default (v2)")
			}
		}
	}

	now := time.Now()
	startedAt := &now
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", task.ID).Updates(map[string]interface{}{
		"status":     model.MigrationRunning,
		"started_at": startedAt,
	})

	var selectedRepos []string
	if err := json.Unmarshal([]byte(task.SelectedRepos), &selectedRepos); err != nil {
		logrus.WithFields(logrus.Fields{
			"module":  "migration",
			"task_id": task.ID,
			"error":   err,
		}).Error("Failed to parse selected repos")
		return w.failTask(task.ID, fmt.Sprintf("failed to parse selected repos: %v", err))
	}

	queue := NewComponentQueue(w.batchSize * 10)
	progressUpdater := NewProgressUpdater(task.ID, w.service.db, 2*time.Second)

	var producerWg sync.WaitGroup
	producerCtx, producerCancel := context.WithCancel(context.Background())
	defer producerCancel()

	for _, repoName := range selectedRepos {
		producerWg.Add(1)
		go func(repo string) {
			defer producerWg.Done()
			w.produceComponents(producerCtx, task.ID, client, repo, queue)
		}(repoName)
	}

	go func() {
		producerWg.Wait()
		queue.Close()
		logrus.WithFields(logrus.Fields{
			"module":  "migration",
			"task_id": task.ID,
		}).Info("All producers finished")
	}()

	go progressUpdater.Start(ctx)

	var consumerWg sync.WaitGroup
	for i := 0; i < w.concurrency; i++ {
		consumerWg.Add(1)
		go func(workerID int) {
			defer consumerWg.Done()
			w.consumeComponents(ctx, task.ID, client, queue, progressUpdater, workerID)
		}(i)
	}

	consumerWg.Wait()
	progressUpdater.Stop()

	w.completeTask(task.ID)
	logrus.WithFields(logrus.Fields{
		"module":  "migration",
		"task_id": task.ID,
	}).Info("Migration task completed (v2)")
	w.service.AddLog(task.ID, "迁移任务完成")
	return nil
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

		w.itemRepo.UpdateStatus(item.ID, model.MigrationItemProcessing, "")

		comp := NexusComponent{
			ID:      item.ComponentID,
			Name:    item.ComponentName,
			Group:   item.ComponentGroup,
			Version: item.Version,
			Format:  item.Format,
		}

		if err := w.migrateComponentWithRetry(ctx, taskID, client, comp, item.ID, item.RetryCount); err != nil {
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   taskID,
				"component": item.ComponentName,
				"version":   item.Version,
				"worker_id": workerID,
				"error":     err,
			}).Warn("Failed to migrate component")
			w.service.AddLog(taskID, fmt.Sprintf("迁移 %s (v%s) 失败: %v", item.ComponentName, item.Version, err))
			w.itemRepo.UpdateStatus(item.ID, model.MigrationItemFailed, err.Error())
			progressUpdater.IncrementFailed()
		} else {
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   taskID,
				"component": item.ComponentName,
				"version":   item.Version,
				"worker_id": workerID,
			}).Info("Component migrated successfully")
			w.itemRepo.UpdateStatus(item.ID, model.MigrationItemCompleted, "")
			progressUpdater.IncrementProcessed()
		}
	}
}

func (w *MigrationWorkerV2) migrateComponentWithRetry(ctx context.Context, taskID uint, client *NexusClient, comp NexusComponent, itemID uint, retryCount int) error {
	components, _, err := client.ListComponentsPage(ctx, comp.Group, "")
	if err != nil {
		return fmt.Errorf("failed to get component details: %w", err)
	}

	var targetComp *NexusComponent
	for i := range components {
		if components[i].ID == comp.ID {
			targetComp = &components[i]
			break
		}
	}

	if targetComp == nil {
		return fmt.Errorf("component not found in repository")
	}

	return w.migrateComponent(taskID, client, *targetComp)
}

func (w *MigrationWorkerV2) migrateComponent(taskID uint, client *NexusClient, comp NexusComponent) error {
	for _, asset := range comp.Assets {
		if asset.DownloadURL == "" {
			continue
		}

		reader, contentType, _, err := client.DownloadAsset(context.Background(), asset.DownloadURL)
		if err != nil {
			return fmt.Errorf("download asset failed: %w", err)
		}

		body, readErr := io.ReadAll(reader)
		reader.Close()
		if readErr != nil {
			return fmt.Errorf("failed to read asset content: %w", readErr)
		}

		size := int64(len(body))
		err = w.storeAsset(taskID, comp, asset, bytes.NewReader(body), size, contentType)
		if err != nil {
			return fmt.Errorf("store asset failed: %w", err)
		}
	}

	return nil
}

func (w *MigrationWorkerV2) storeAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64, _ string) error {
	switch comp.Format {
	case "maven2":
		return w.storeMavenAsset(taskID, comp, asset, reader, size)
	case "npm":
		return w.storeNpmAsset(taskID, comp, asset, reader, size)
	case "pypi":
		return w.storePypiAsset(taskID, comp, asset, reader, size)
	case "go":
		return w.storeGoAsset(taskID, comp, asset, reader, size)
	case "raw":
		return w.storeGenericAsset(taskID, comp, asset, reader, size)
	default:
		return w.storeGenericAsset(taskID, comp, asset, reader, size)
	}
}

func (w *MigrationWorkerV2) storeMavenAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64) error {
	packaging := getPackaging(asset.Path)
	storageName := groupArtifactToStorageName(comp.Group, comp.Name)
	storageVersion := comp.Version + "/" + filepath.Base(asset.Path)

	storageKey, err := w.storageSvc.StorePackageWithBackend(context.Background(), "maven", storageName, storageVersion, reader, size, w.targetBackendID)
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

	task, _ := w.service.GetTask(taskID)
	repoType := model.RepoTypeLocal
	if task != nil && task.TargetRepositoryID > 0 {
		repoType = model.RepoTypeLocal
	}

	_, _, _, err = w.pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:           comp.Group + ":" + comp.Name,
		Type:           model.PackageTypeMaven,
		RepositoryID:   task.TargetRepositoryID,
		RepositoryType: repoType,
		Description:    comp.Name,
	}, &model.PackageVersion{
		Version:     comp.Version,
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
		Metadata:    marshalMetadata(metadata),
	}, &model.PackageFile{
		Filename:    filepath.Base(asset.Path),
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   size,
	})

	return err
}

func (w *MigrationWorkerV2) storeNpmAsset(taskID uint, comp NexusComponent, _ NexusAsset, reader io.Reader, size int64) error {
	name := comp.Name
	if comp.Group != "" {
		name = comp.Group + "/" + comp.Name
	}
	version := comp.Version
	if version == "" {
		version = "1.0.0"
	}

	storageVersion := version + "/package.tgz"
	storageKey, err := w.storageSvc.StorePackageWithBackend(context.Background(), "npm", name, storageVersion, reader, size, w.targetBackendID)
	if err != nil {
		return err
	}

	task, _ := w.service.GetTask(taskID)
	repoType := model.RepoTypeLocal

	_, _, _, err = w.pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:           name,
		Type:           model.PackageTypeNPM,
		RepositoryID:   task.TargetRepositoryID,
		RepositoryType: repoType,
		Description:    comp.Name,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
	}, &model.PackageFile{
		Filename:    "package.tgz",
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   size,
	})

	return err
}

func (w *MigrationWorkerV2) storePypiAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64) error {
	name := comp.Name
	if comp.Group != "" {
		name = comp.Group
	}
	version := comp.Version
	if version == "" {
		version = "1.0.0"
	}

	storageVersion := version + "/" + filepath.Base(asset.Path)
	storageKey, err := w.storageSvc.StorePackageWithBackend(context.Background(), "pypi", name, storageVersion, reader, size, w.targetBackendID)
	if err != nil {
		return err
	}

	task, _ := w.service.GetTask(taskID)
	repoType := model.RepoTypeLocal

	_, _, _, err = w.pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:           name,
		Type:           model.PackageTypePyPI,
		RepositoryID:   task.TargetRepositoryID,
		RepositoryType: repoType,
		Description:    comp.Name,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
	}, &model.PackageFile{
		Filename:    filepath.Base(asset.Path),
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   size,
	})

	return err
}

func (w *MigrationWorkerV2) storeGoAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64) error {
	name := comp.Name
	if comp.Group != "" {
		name = comp.Group + "/" + comp.Name
	}
	version := comp.Version
	if version == "" {
		version = "v1.0.0"
	}

	storageVersion := version + "/" + filepath.Base(asset.Path)
	storageKey, err := w.storageSvc.StorePackageWithBackend(context.Background(), "go", name, storageVersion, reader, size, w.targetBackendID)
	if err != nil {
		return err
	}

	task, _ := w.service.GetTask(taskID)
	repoType := model.RepoTypeLocal

	_, _, _, err = w.pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:           name,
		Type:           model.PackageTypeGo,
		RepositoryID:   task.TargetRepositoryID,
		RepositoryType: repoType,
		Description:    comp.Name,
	}, &model.PackageVersion{
		Version:     version,
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
	}, &model.PackageFile{
		Filename:    filepath.Base(asset.Path),
		FileType:    model.FileTypePrimary,
		StoragePath: storageKey,
		SizeBytes:   size,
	})

	return err
}

func (w *MigrationWorkerV2) storeGenericAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64) error {
	path := asset.Path
	if path == "" {
		path = comp.Name + "/" + filepath.Base(asset.DownloadURL)
	}

	storageKey := "generic/" + filepath.Clean(path)
	backend := w.storageSvc.GetDefaultBackend()
	if err := backend.Put(context.Background(), storageKey, reader, size); err != nil {
		return err
	}

	task, _ := w.service.GetTask(taskID)
	repoType := model.RepoTypeLocal

	_, _, _, err := w.pkgRepo.StorePackageFile(context.Background(), &model.Package{
		Name:           path,
		Type:           model.PackageTypeGeneric,
		RepositoryID:   task.TargetRepositoryID,
		RepositoryType: repoType,
		Description:    comp.Name,
	}, &model.PackageVersion{
		Version:     "1.0.0",
		Status:      model.StatusPublished,
		StoragePath: filepath.Dir(storageKey),
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

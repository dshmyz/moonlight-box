package migration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/moonlight-box/registry/internal/service"
	"github.com/sirupsen/logrus"
)

type MigrationWorker struct {
	service     *MigrationService
	storageSvc  *service.StorageService
	pkgRepo     *repository.PackageRepository
	concurrency int
}

func NewMigrationWorker(migrationSvc *MigrationService, storageSvc *service.StorageService, pkgRepo *repository.PackageRepository, concurrency int) *MigrationWorker {
	return &MigrationWorker{
		service:     migrationSvc,
		storageSvc:  storageSvc,
		pkgRepo:     pkgRepo,
		concurrency: concurrency,
	}
}

func (w *MigrationWorker) Execute(ctx context.Context, task *model.MigrationTask) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	w.service.RegisterContext(task.ID, ctx, cancel)

	logrus.WithFields(logrus.Fields{
		"module":      "migration",
		"task_id":     task.ID,
		"source_url":  task.SourceURL,
		"concurrency": w.concurrency,
	}).Info("Migration task started")

	client := NewNexusClient(task.SourceURL, task.Username, mustGetPassword(task))
	w.service.RegisterNexusClient(task.ID, client)

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

	semaphore := make(chan struct{}, w.concurrency)
	var wg sync.WaitGroup

	for _, repoName := range selectedRepos {
		select {
		case <-ctx.Done():
			logrus.WithFields(logrus.Fields{
				"module":  "migration",
				"task_id": task.ID,
			}).Warn("Migration task cancelled")
			return w.cancelTask(task.ID)
		default:
		}

		w.service.AddLog(task.ID, fmt.Sprintf("开始迁移仓库: %s", repoName))
		logrus.WithFields(logrus.Fields{
			"module":    "migration",
			"task_id":   task.ID,
			"repo_name": repoName,
		}).Info("Starting repository migration")

		components, err := client.ListComponents(ctx, repoName)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"module":    "migration",
				"task_id":   task.ID,
				"repo_name": repoName,
				"error":     err,
			}).Error("Failed to list components")
			w.service.AddLog(task.ID, fmt.Sprintf("获取 %s 组件列表失败: %v", repoName, err))
			continue
		}

		logrus.WithFields(logrus.Fields{
			"module":     "migration",
			"task_id":    task.ID,
			"repo_name":  repoName,
			"components": len(components),
		}).Info("Components retrieved")

		w.service.AddLog(task.ID, fmt.Sprintf("仓库 %s 共有 %d 个组件", repoName, len(components)))

		if len(components) == 0 {
			w.service.AddLog(task.ID, fmt.Sprintf("仓库 %s 没有组件，跳过", repoName))
			continue
		}

		w.updateTotal(task.ID, len(components))

		for _, comp := range components {
			wg.Add(1)
			semaphore <- struct{}{}

			go func(c NexusComponent) {
				defer func() { <-semaphore; wg.Done() }()

				select {
				case <-ctx.Done():
					return
				default:
				}

				w.service.AddLog(task.ID, fmt.Sprintf("开始迁移组件: %s (版本: %s)", c.Name, c.Version))

				if err := w.migrateComponent(task.ID, client, c); err != nil {
					logrus.WithFields(logrus.Fields{
						"module":    "migration",
						"task_id":   task.ID,
						"component": c.Name,
						"version":   c.Version,
						"format":    c.Format,
						"error":     err,
					}).Warn("Failed to migrate component")
					w.service.AddLog(task.ID, fmt.Sprintf("迁移 %s (v%s) 失败: %v", c.Name, c.Version, err))
					w.incrementFailed(task.ID)
				} else {
					logrus.WithFields(logrus.Fields{
						"module":    "migration",
						"task_id":   task.ID,
						"component": c.Name,
						"version":   c.Version,
					}).Info("Component migrated successfully")
					w.incrementProcessed(task.ID)
				}
			}(comp)
		}
	}

	wg.Wait()

	w.completeTask(task.ID)
	logrus.WithFields(logrus.Fields{
		"module":  "migration",
		"task_id": task.ID,
	}).Info("Migration task completed")
	w.service.AddLog(task.ID, "迁移任务完成")
	return nil
}

func (w *MigrationWorker) migrateComponent(taskID uint, client *NexusClient, comp NexusComponent) error {
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

func (w *MigrationWorker) storeAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64, _ string) error {
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

func (w *MigrationWorker) storeMavenAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64) error {
	packaging := getPackaging(asset.Path)
	storageName := groupArtifactToStorageName(comp.Group, comp.Name)
	storageVersion := comp.Version + "/" + filepath.Base(asset.Path)

	storageKey, err := w.storageSvc.StorePackage(context.Background(), "maven", storageName, storageVersion, reader, size)
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

func (w *MigrationWorker) storeNpmAsset(taskID uint, comp NexusComponent, _ NexusAsset, reader io.Reader, size int64) error {
	name := comp.Name
	if comp.Group != "" {
		name = comp.Group + "/" + comp.Name
	}
	version := comp.Version
	if version == "" {
		version = "1.0.0"
	}

	storageVersion := version + "/package.tgz"
	storageKey, err := w.storageSvc.StorePackage(context.Background(), "npm", name, storageVersion, reader, size)
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

func (w *MigrationWorker) storePypiAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64) error {
	name := comp.Name
	if comp.Group != "" {
		name = comp.Group
	}
	version := comp.Version
	if version == "" {
		version = "1.0.0"
	}

	storageVersion := version + "/" + filepath.Base(asset.Path)
	storageKey, err := w.storageSvc.StorePackage(context.Background(), "pypi", name, storageVersion, reader, size)
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

func (w *MigrationWorker) storeGoAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64) error {
	name := comp.Name
	if comp.Group != "" {
		name = comp.Group + "/" + comp.Name
	}
	version := comp.Version
	if version == "" {
		version = "v1.0.0"
	}

	storageVersion := version + "/" + filepath.Base(asset.Path)
	storageKey, err := w.storageSvc.StorePackage(context.Background(), "go", name, storageVersion, reader, size)
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

func (w *MigrationWorker) storeGenericAsset(taskID uint, comp NexusComponent, asset NexusAsset, reader io.Reader, size int64) error {
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

func (w *MigrationWorker) updateTotal(taskID uint, total int) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Update("total_items", total)
}

func (w *MigrationWorker) incrementProcessed(taskID uint) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).
		Update("processed_items", w.service.db.Raw("processed_items + 1"))
}

func (w *MigrationWorker) incrementFailed(taskID uint) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).
		Update("failed_items", w.service.db.Raw("failed_items + 1"))
}

func (w *MigrationWorker) failTask(taskID uint, errMsg string) error {
	return w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":        model.MigrationFailed,
		"error_message": errMsg,
		"completed_at":  time.Now(),
	}).Error
}

func (w *MigrationWorker) cancelTask(taskID uint) error {
	return w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":       model.MigrationCancelled,
		"completed_at": time.Now(),
	}).Error
}

func (w *MigrationWorker) completeTask(taskID uint) {
	w.service.db.Model(&model.MigrationTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":       model.MigrationCompleted,
		"completed_at": time.Now(),
	})
}

func mustGetPassword(task *model.MigrationTask) string {
	password, _ := task.GetPassword()
	return password
}

package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dshmyz/moonlight-box/internal/model"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/sirupsen/logrus"
)

// BackupTarget 定义一个需要备份的文件
type BackupTarget struct {
	// ArchivePath 在 tar 包中的相对路径（如 "db/registry.db"）
	ArchivePath string
	// LocalPath 在磁盘上的绝对路径
	LocalPath string
}

type BackupService struct {
	backupRepo   *repository.BackupRepository
	storageSvc   *StorageService
	backupPrefix string // 备份文件在存储中的 key 前缀，如 "backups"
	targets      []BackupTarget
	wg           sync.WaitGroup
}

// NewBackupService 创建备份服务
// storageSvc 用于读写备份文件（支持 Local/S3 等任意存储后端）
// backupPrefix 是备份文件在存储中的 key 前缀（如 "backups"）
// targets 指定需要备份的文件列表
func NewBackupService(backupRepo *repository.BackupRepository, storageSvc *StorageService, backupPrefix string, targets []BackupTarget) *BackupService {
	return &BackupService{
		backupRepo:   backupRepo,
		storageSvc:   storageSvc,
		backupPrefix: backupPrefix,
		targets:      targets,
	}
}

func (s *BackupService) backupKey(name string) string {
	return filepath.Join(s.backupPrefix, name+".tar.gz")
}

func (s *BackupService) CreateBackup(name string, backupType model.BackupType, description string, createdBy uint) (*model.Backup, error) {
	backup := &model.Backup{
		Name:        name,
		Type:        backupType,
		Status:      model.BackupStatusPending,
		Description: description,
		CreatedBy:   createdBy,
		FilePath:    s.backupKey(name),
	}

	if err := s.backupRepo.Create(backup); err != nil {
		logrus.WithFields(logrus.Fields{
			"module":    "backup",
			"backup_id": backup.ID,
			"error":     err,
		}).Error("Failed to create backup record")
		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"module":      "backup",
		"backup_id":   backup.ID,
		"backup_name": name,
		"type":        backupType,
	}).Info("Backup created, starting execution")

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.executeBackup(backup)
	}()

	return backup, nil
}

func (s *BackupService) executeBackup(backup *model.Backup) {
	startTime := time.Now()
	logrus.WithFields(logrus.Fields{
		"module":    "backup",
		"backup_id": backup.ID,
	}).Info("Backup execution started")

	backup.Status = model.BackupStatusRunning
	backup.StartedAt = &startTime
	if err := s.backupRepo.Update(backup); err != nil {
		logrus.WithError(err).WithField("backup_id", backup.ID).Error("Failed to update backup status to running")
	}

	// 在内存中构建 tar.gz
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	var totalSize int64

	for _, target := range s.targets {
		info, err := os.Stat(target.LocalPath)
		if err != nil {
			s.markBackupFailed(backup, fmt.Sprintf("failed to stat %s: %v", target.LocalPath, err))
			return
		}

		header, err := tar.FileInfoHeader(info, target.ArchivePath)
		if err != nil {
			s.markBackupFailed(backup, fmt.Sprintf("failed to create tar header for %s: %v", target.LocalPath, err))
			return
		}
		header.Name = target.ArchivePath

		if err := tw.WriteHeader(header); err != nil {
			s.markBackupFailed(backup, fmt.Sprintf("failed to write tar header for %s: %v", target.LocalPath, err))
			return
		}

		srcFile, err := os.Open(target.LocalPath)
		if err != nil {
			s.markBackupFailed(backup, fmt.Sprintf("failed to open %s: %v", target.LocalPath, err))
			return
		}

		written, err := io.Copy(tw, srcFile)
		srcFile.Close()
		if err != nil {
			s.markBackupFailed(backup, fmt.Sprintf("failed to copy %s: %v", target.LocalPath, err))
			return
		}

		totalSize += written
		logrus.WithFields(logrus.Fields{
			"module":       "backup",
			"archive_path": target.ArchivePath,
			"size_bytes":   written,
		}).Debug("Backed up file")
	}

	// 关闭 tar 和 gzip writer 以 flush 数据
	if err := tw.Close(); err != nil {
		s.markBackupFailed(backup, fmt.Sprintf("failed to close tar writer: %v", err))
		return
	}
	if err := gzw.Close(); err != nil {
		s.markBackupFailed(backup, fmt.Sprintf("failed to close gzip writer: %v", err))
		return
	}

	// 通过存储后端写入备份文件（支持 Local/S3 等）
	backend := s.storageSvc.GetDefaultBackend()
	archiveSize := int64(buf.Len())
	if err := backend.Put(context.Background(), backup.FilePath, &buf, archiveSize); err != nil {
		s.markBackupFailed(backup, fmt.Sprintf("failed to store backup: %v", err))
		return
	}

	backup.Status = model.BackupStatusCompleted
	backup.SizeBytes = totalSize
	now := time.Now()
	backup.CompletedAt = &now
	if err := s.backupRepo.Update(backup); err != nil {
		logrus.WithError(err).WithField("backup_id", backup.ID).Error("Failed to update backup status to completed")
	}

	duration := time.Since(startTime)
	logrus.WithFields(logrus.Fields{
		"module":       "backup",
		"backup_id":    backup.ID,
		"size_bytes":   totalSize,
		"archive_size": archiveSize,
		"duration_ms":  duration.Milliseconds(),
		"storage_type": backend.Name(),
	}).Info("Backup completed successfully")
}

func (s *BackupService) markBackupFailed(backup *model.Backup, errMsg string) {
	backup.Status = model.BackupStatusFailed
	backup.Error = errMsg
	now := time.Now()
	backup.CompletedAt = &now
	if err := s.backupRepo.Update(backup); err != nil {
		logrus.WithError(err).WithField("backup_id", backup.ID).Error("Failed to update backup status to failed")
	}

	logrus.WithFields(logrus.Fields{
		"module":    "backup",
		"backup_id": backup.ID,
		"error":     errMsg,
	}).Error("Backup execution failed")
}

// Shutdown 等待所有正在执行的备份任务完成
func (s *BackupService) Shutdown() {
	s.wg.Wait()
}

func (s *BackupService) RestoreBackup(backupID uint) error {
	backup, err := s.backupRepo.GetByID(backupID)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":    "backup",
			"backup_id": backupID,
			"error":     err,
		}).Error("Restore backup failed: backup not found")
		return fmt.Errorf("backup not found: %w", err)
	}

	if backup.Status != model.BackupStatusCompleted {
		logrus.WithFields(logrus.Fields{
			"module":         "backup",
			"backup_id":      backupID,
			"current_status": backup.Status,
		}).Warn("Restore backup failed: backup not completed")
		return fmt.Errorf("backup is not completed, current status: %s", backup.Status)
	}

	logrus.WithFields(logrus.Fields{
		"module":      "backup",
		"backup_id":   backupID,
		"backup_name": backup.Name,
	}).Info("Backup restoration started")

	// 构建 archivePath → localPath 的映射
	restoreMap := make(map[string]string)
	for _, t := range s.targets {
		restoreMap[t.ArchivePath] = t.LocalPath
	}

	// 从存储后端读取备份文件
	backend := s.storageSvc.GetDefaultBackend()
	reader, err := backend.Get(context.Background(), backup.FilePath)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":    "backup",
			"backup_id": backupID,
			"error":     err,
		}).Error("Restore backup failed: cannot read backup file from storage")
		return fmt.Errorf("failed to read backup file from storage: %w", err)
	}
	defer reader.Close()

	gzr, err := gzip.NewReader(reader)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":    "backup",
			"backup_id": backupID,
			"error":     err,
		}).Error("Restore backup failed: cannot create gzip reader")
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"module":    "backup",
				"backup_id": backupID,
				"error":     err,
			}).Error("Restore backup failed: cannot read tar header")
			return fmt.Errorf("failed to read tar header: %w", err)
		}

		targetPath, ok := restoreMap[header.Name]
		if !ok {
			logrus.WithFields(logrus.Fields{
				"module":       "backup",
				"archive_path": header.Name,
			}).Warn("Skipping unknown file in backup archive")
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			logrus.WithFields(logrus.Fields{
				"module":    "backup",
				"backup_id": backupID,
				"path":      targetPath,
				"error":     err,
			}).Error("Restore backup failed: cannot create directory")
			return fmt.Errorf("failed to create directory: %w", err)
		}

		outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"module":    "backup",
				"backup_id": backupID,
				"path":      targetPath,
				"error":     err,
			}).Error("Restore backup failed: cannot create file")
			return fmt.Errorf("failed to create file: %w", err)
		}

		if _, err := io.Copy(outFile, tr); err != nil {
			outFile.Close()
			logrus.WithFields(logrus.Fields{
				"module":    "backup",
				"backup_id": backupID,
				"path":      targetPath,
				"error":     err,
			}).Error("Restore backup failed: cannot write file")
			return fmt.Errorf("failed to write file: %w", err)
		}
		outFile.Close()
	}

	logrus.WithFields(logrus.Fields{
		"module":      "backup",
		"backup_id":   backupID,
		"backup_name": backup.Name,
	}).Info("Backup restoration completed successfully")

	return nil
}

func (s *BackupService) GetBackup(id uint) (*model.Backup, error) {
	return s.backupRepo.GetByID(id)
}

func (s *BackupService) ListBackups(page, pageSize int) ([]model.Backup, int64, error) {
	return s.backupRepo.List(page, pageSize)
}

func (s *BackupService) DeleteBackup(id uint) error {
	backup, err := s.backupRepo.GetByID(id)
	if err != nil {
		return err
	}

	backend := s.storageSvc.GetDefaultBackend()
	if err := backend.Delete(context.Background(), backup.FilePath); err != nil {
		logrus.WithFields(logrus.Fields{
			"module":    "backup",
			"backup_id": id,
			"error":     err,
		}).Warn("Failed to delete backup file from storage")
	}

	return s.backupRepo.Delete(id)
}
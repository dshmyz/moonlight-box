package service

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/moonlight-box/registry/internal/model"
	"github.com/moonlight-box/registry/internal/repository"
	"github.com/sirupsen/logrus"
)

type BackupService struct {
	backupRepo  *repository.BackupRepository
	storagePath string
	backupPath  string
}

func NewBackupService(backupRepo *repository.BackupRepository, storagePath, backupPath string) *BackupService {
	return &BackupService{
		backupRepo:  backupRepo,
		storagePath: storagePath,
		backupPath:  backupPath,
	}
}

func (s *BackupService) CreateBackup(name string, backupType model.BackupType, description string, createdBy uint) (*model.Backup, error) {
	if err := os.MkdirAll(s.backupPath, 0755); err != nil {
		logrus.WithFields(logrus.Fields{
			"module": "backup",
			"error":  err,
		}).Error("Failed to create backup directory")
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	backup := &model.Backup{
		Name:        name,
		Type:        backupType,
		Status:      model.BackupStatusPending,
		Description: description,
		CreatedBy:   createdBy,
		FilePath:    filepath.Join(s.backupPath, name+".tar.gz"),
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
		"module":     "backup",
		"backup_id":  backup.ID,
		"backup_name": name,
		"type":       backupType,
	}).Info("Backup created, starting execution")

	go s.executeBackup(backup)

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
	s.backupRepo.Update(backup)

	file, err := os.Create(backup.FilePath)
	if err != nil {
		s.markBackupFailed(backup, fmt.Sprintf("failed to create backup file: %v", err))
		return
	}
	defer file.Close()

	gzw := gzip.NewWriter(file)
	defer gzw.Close()

	tw := tar.NewWriter(gzw)
	defer tw.Close()

	var totalSize int64

	err = filepath.Walk(s.storagePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(s.storagePath, path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, relPath)
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		written, err := io.Copy(tw, file)
		if err != nil {
			return err
		}

		totalSize += written
		return nil
	})

	if err != nil {
		s.markBackupFailed(backup, fmt.Sprintf("failed to create backup: %v", err))
		return
	}

	backup.Status = model.BackupStatusCompleted
	backup.SizeBytes = totalSize
	now := time.Now()
	backup.CompletedAt = &now
	s.backupRepo.Update(backup)

	duration := time.Since(startTime)
	logrus.WithFields(logrus.Fields{
		"module":      "backup",
		"backup_id":   backup.ID,
		"size_bytes":  totalSize,
		"duration_ms": duration.Milliseconds(),
	}).Info("Backup completed successfully")
}

func (s *BackupService) markBackupFailed(backup *model.Backup, errMsg string) {
	backup.Status = model.BackupStatusFailed
	backup.Error = errMsg
	now := time.Now()
	backup.CompletedAt = &now
	s.backupRepo.Update(backup)

	logrus.WithFields(logrus.Fields{
		"module":    "backup",
		"backup_id": backup.ID,
		"error":     errMsg,
	}).Error("Backup execution failed")
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
			"module":       "backup",
			"backup_id":    backupID,
			"current_status": backup.Status,
		}).Warn("Restore backup failed: backup not completed")
		return fmt.Errorf("backup is not completed, current status: %s", backup.Status)
	}

	logrus.WithFields(logrus.Fields{
		"module":    "backup",
		"backup_id": backupID,
		"backup_name": backup.Name,
	}).Info("Backup restoration started")

	file, err := os.Open(backup.FilePath)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"module":    "backup",
			"backup_id": backupID,
			"error":     err,
		}).Error("Restore backup failed: cannot open backup file")
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
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

		targetPath := filepath.Join(s.storagePath, header.Name)

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			logrus.WithFields(logrus.Fields{
				"module":    "backup",
				"backup_id": backupID,
				"path":      targetPath,
				"error":     err,
			}).Error("Restore backup failed: cannot create directory")
			return fmt.Errorf("failed to create directory: %w", err)
		}

		file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"module":    "backup",
				"backup_id": backupID,
				"path":      targetPath,
				"error":     err,
			}).Error("Restore backup failed: cannot create file")
			return fmt.Errorf("failed to create file: %w", err)
		}

		if _, err := io.Copy(file, tr); err != nil {
			file.Close()
			logrus.WithFields(logrus.Fields{
				"module":    "backup",
				"backup_id": backupID,
				"path":      targetPath,
				"error":     err,
			}).Error("Restore backup failed: cannot write file")
			return fmt.Errorf("failed to write file: %w", err)
		}
		file.Close()
	}

	logrus.WithFields(logrus.Fields{
		"module":    "backup",
		"backup_id": backupID,
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

	if err := os.Remove(backup.FilePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete backup file: %w", err)
	}

	return s.backupRepo.Delete(id)
}

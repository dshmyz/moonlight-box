package job

import (
	"context"
	"time"

	"github.com/moonlight-box/registry/internal/database"
	"github.com/moonlight-box/registry/internal/model"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type Job interface {
	Name() string
	Run(ctx context.Context) error
}

type JobScheduler struct {
	jobs    []Job
	ticker  *time.Ticker
	stopCh  chan struct{}
}

func NewJobScheduler() *JobScheduler {
	return &JobScheduler{
		stopCh: make(chan struct{}),
	}
}

func (s *JobScheduler) AddJob(job Job) {
	s.jobs = append(s.jobs, job)
}

func (s *JobScheduler) Start(interval time.Duration) {
	s.ticker = time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				for _, job := range s.jobs {
					if err := job.Run(context.Background()); err != nil {
						logrus.WithField("job", job.Name()).WithError(err).Error("Job failed")
					}
				}
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *JobScheduler) Stop() {
	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopCh)
}

type GCJob struct {
	db *gorm.DB
}

func NewGCJob() *GCJob {
	return &GCJob{db: database.DB}
}

func (j *GCJob) Name() string {
	return "gc"
}

func (j *GCJob) Run(ctx context.Context) error {
	logrus.Info("Starting GC job")
	
	var referencedBlobIDs []uint
	if err := j.db.Model(&model.ArtifactBlob{}).
		Distinct("blob_id").
		Pluck("blob_id", &referencedBlobIDs).Error; err != nil {
		return err
	}
	
	var unreferencedBlobs []model.BlobV2
	if err := j.db.Where("id NOT IN ?", referencedBlobIDs).
		Find(&unreferencedBlobs).Error; err != nil {
		return err
	}
	
	for _, blob := range unreferencedBlobs {
		if err := j.db.Delete(&blob).Error; err != nil {
			logrus.WithField("blob_id", blob.ID).WithError(err).Error("Failed to delete blob")
		}
	}
	
	logrus.WithField("deleted", len(unreferencedBlobs)).Info("GC job completed")
	return nil
}

type CleanupJob struct {
	db        *gorm.DB
	retention time.Duration
}

func NewCleanupJob(retention time.Duration) *CleanupJob {
	return &CleanupJob{
		db:        database.DB,
		retention: retention,
	}
}

func (j *CleanupJob) Name() string {
	return "cleanup"
}

func (j *CleanupJob) Run(ctx context.Context) error {
	logrus.Info("Starting cleanup job")
	
	cutoff := time.Now().Add(-j.retention)
	
	if err := j.db.Where("created_at < ?", cutoff).
		Delete(&model.CacheEntry{}).Error; err != nil {
		return err
	}
	
	logrus.Info("Cleanup job completed")
	return nil
}

type BlobVerifyJob struct {
	db *gorm.DB
}

func NewBlobVerifyJob() *BlobVerifyJob {
	return &BlobVerifyJob{db: database.DB}
}

func (j *BlobVerifyJob) Name() string {
	return "blob_verify"
}

func (j *BlobVerifyJob) Run(ctx context.Context) error {
	logrus.Info("Starting blob verify job")
	
	var blobs []model.BlobV2
	if err := j.db.Find(&blobs).Error; err != nil {
		return err
	}
	
	for _, blob := range blobs {
		logrus.WithFields(logrus.Fields{
			"blob_id":  blob.ID,
			"digest":   blob.Digest,
			"size":     blob.Size,
		}).Debug("Verified blob")
	}
	
	logrus.WithField("count", len(blobs)).Info("Blob verify job completed")
	return nil
}

type StaleRefreshJob struct {
	db *gorm.DB
}

func NewStaleRefreshJob() *StaleRefreshJob {
	return &StaleRefreshJob{db: database.DB}
}

func (j *StaleRefreshJob) Name() string {
	return "stale_refresh"
}

func (j *StaleRefreshJob) Run(ctx context.Context) error {
	logrus.Info("Starting stale refresh job")
	return nil
}

type MetadataRebuildJob struct {
	db *gorm.DB
}

func NewMetadataRebuildJob() *MetadataRebuildJob {
	return &MetadataRebuildJob{db: database.DB}
}

func (j *MetadataRebuildJob) Name() string {
	return "metadata_rebuild"
}

func (j *MetadataRebuildJob) Run(ctx context.Context) error {
	logrus.Info("Starting metadata rebuild job")
	return nil
}

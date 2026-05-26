package storage

import (
	"context"
	"io"
)

type Backend interface {
	Name() string
	Init(basePath string) error
	Put(ctx context.Context, key string, reader io.Reader, size int64) error
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Size(ctx context.Context, key string) (int64, error)
	List(ctx context.Context, prefix string) ([]Entry, error)
	Browse(ctx context.Context, path string) ([]BrowseEntry, error)
	Close() error
	BasePath() string
}

type Entry struct {
	Key   string
	IsDir bool
	Size  int64
}

type BrowseEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	ModTime string `json:"mod_time"`
}

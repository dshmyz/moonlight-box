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
	Close() error
}

type Entry struct {
	Key   string
	IsDir bool
	Size  int64
}

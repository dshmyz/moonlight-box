package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dshmyz/moonlight-box/internal/util"
)

type LocalStorage struct {
	basePath string
	maxSize  int64
}

func NewLocalStorage(basePath string, maxSizeMB int64) (*LocalStorage, error) {
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(absPath, 0755); err != nil {
		return nil, err
	}

	return &LocalStorage{
		basePath: absPath,
		maxSize:  maxSizeMB * 1024 * 1024,
	}, nil
}

func (s *LocalStorage) Name() string {
	return "local"
}

func (s *LocalStorage) Init(basePath string) error {
	return os.MkdirAll(basePath, 0755)
}

var errPathTraversal = errors.New("path traversal detected")

func (s *LocalStorage) resolvePath(key string) string {
	key = filepath.Clean(key)
	key = strings.TrimPrefix(key, "/")
	return filepath.Join(s.basePath, key)
}

func (s *LocalStorage) resolvePathSafe(key string) (string, error) {
	fullPath := s.resolvePath(key)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}
	absBase, err := filepath.Abs(s.basePath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve base path: %w", err)
	}
	if !strings.HasPrefix(absPath, absBase+string(os.PathSeparator)) && absPath != absBase {
		return "", errPathTraversal
	}
	return absPath, nil
}

func (s *LocalStorage) Put(ctx context.Context, key string, reader io.Reader, size int64) error {
	fullPath, err := s.resolvePathSafe(key)
	if err != nil {
		return err
	}
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return err
	}
	defer file.Close()

	written, err := io.Copy(file, reader)
	if err != nil {
		os.Remove(fullPath)
		return err
	}

	if size > 0 && written != size {
		os.Remove(fullPath)
		return io.ErrShortWrite
	}

	return nil
}

func (s *LocalStorage) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	fullPath, err := s.resolvePathSafe(key)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, util.ErrPackageNotFound
		}
		return nil, err
	}

	if info.IsDir() {
		return nil, fmt.Errorf("cannot read directory as file: %s", key)
	}

	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, util.ErrPackageNotFound
		}
		return nil, err
	}

	return file, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	fullPath, err := s.resolvePathSafe(key)
	if err != nil {
		return err
	}

	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return nil // 已删除视为成功
		}
		return err
	}

	// 尝试清理空目录
	dir := filepath.Dir(fullPath)
	s.removeEmptyDirs(dir)

	return nil
}

func (s *LocalStorage) Move(ctx context.Context, oldKey, newKey string) error {
	oldPath, err := s.resolvePathSafe(oldKey)
	if err != nil {
		return err
	}
	newPath, err := s.resolvePathSafe(newKey)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
		return err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	s.removeEmptyDirs(filepath.Dir(oldPath))
	return nil
}

func (s *LocalStorage) removeEmptyDirs(dir string) {
	if dir == s.basePath || !strings.HasPrefix(dir, s.basePath) {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) > 0 {
		return
	}

	os.Remove(dir)
	s.removeEmptyDirs(filepath.Dir(dir))
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	fullPath, err := s.resolvePathSafe(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(fullPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *LocalStorage) Size(ctx context.Context, key string) (int64, error) {
	fullPath, err := s.resolvePathSafe(key)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, util.ErrPackageNotFound
		}
		return 0, err
	}
	if info.IsDir() {
		return 0, fmt.Errorf("cannot get size of directory: %s", key)
	}
	return info.Size(), nil
}

func (s *LocalStorage) List(ctx context.Context, prefix string) ([]Entry, error) {
	dirPath, err := s.resolvePathSafe(prefix)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Entry{}, nil
		}
		return nil, err
	}

	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		result = append(result, Entry{
			Key:   filepath.Join(prefix, entry.Name()),
			IsDir: entry.IsDir(),
			Size:  info.Size(),
		})
	}

	return result, nil
}

func (s *LocalStorage) Close() error {
	return nil
}

func (s *LocalStorage) BasePath() string {
	return s.basePath
}

func (s *LocalStorage) Browse(ctx context.Context, path string) ([]BrowseEntry, error) {
	cleanPath := strings.TrimPrefix(path, "/")
	cleanPath = strings.TrimSuffix(cleanPath, "/")

	fullPath, err := s.resolvePathSafe(cleanPath)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []BrowseEntry{}, nil
		}
		return nil, err
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", path)
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, err
	}

	result := make([]BrowseEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		entryPath := filepath.Join(cleanPath, entry.Name())
		if cleanPath == "" || cleanPath == "/" {
			entryPath = entry.Name()
		}

		result = append(result, BrowseEntry{
			Name:    entry.Name(),
			Path:    entryPath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	return result, nil
}

func (s *LocalStorage) ResolvePathSafe(key string) (string, error) {
	return s.resolvePathSafe(key)
}

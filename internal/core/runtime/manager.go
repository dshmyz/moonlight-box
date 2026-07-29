package runtime

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// RepositoryFactory 根据仓库名称从 DB 加载仓库并创建 Runtime。
// 由 main.go 在启动时注入，供 Get() 懒加载使用。
type RepositoryFactory func(name string) (*Repository, error)

type loadResult struct {
	repo *Repository
	err  error
}

type cachedRepo struct {
	repo      *Repository
	expiresAt time.Time
}

type DefaultRepositoryManager struct {
	repos   sync.Map
	factory RepositoryFactory
	// loading 防止同一仓库并发加载（singleflight）
	loading sync.Map
	ttl     time.Duration
}

func NewDefaultRepositoryManager() *DefaultRepositoryManager {
	return &DefaultRepositoryManager{
		ttl: 10 * time.Minute,
	}
}

// SetFactory 设置懒加载工厂函数。启动时注入后，Get() 在 cache miss 时自动创建 Runtime。
func (m *DefaultRepositoryManager) SetFactory(f RepositoryFactory) {
	m.factory = f
}

// SetTTL 设置缓存过期时间。过期后下次 Get 会自动从 DB 重新加载（兜底机制）。
func (m *DefaultRepositoryManager) SetTTL(ttl time.Duration) {
	m.ttl = ttl
}

// Get 获取仓库。优先从内存缓存读取；缓存过期或未命中时自动从 DB 加载。
func (m *DefaultRepositoryManager) Get(name string) *Repository {
	if v, ok := m.repos.Load(name); ok {
		c := v.(*cachedRepo)
		if time.Now().Before(c.expiresAt) {
			return c.repo
		}
		// TTL 过期，删除后走懒加载
		m.repos.Delete(name)
	}
	if m.factory == nil {
		return nil
	}
	return m.loadAndCache(name)
}

// loadAndCache 通过 singleflight 从 DB 加载并缓存
func (m *DefaultRepositoryManager) loadAndCache(name string) *Repository {
	chI, loaded := m.loading.LoadOrStore(name, make(chan *loadResult, 1))
	ch, ok := chI.(chan *loadResult)
	if !ok {
		logrus.WithField("repo", name).Error("manager: unexpected loading channel type")
		return nil
	}
	if !loaded {
		// 当前 goroutine 负责加载
		repo, err := m.factory(name)
		result := &loadResult{repo: repo, err: err}
		if err == nil {
			m.repos.Store(name, &cachedRepo{
				repo:      repo,
				expiresAt: time.Now().Add(m.ttl),
			})
		} else {
			logrus.WithError(err).WithField("repo", name).Warn("manager: lazy-load failed")
		}
		ch <- result
		m.loading.Delete(name)
		return repo
	}
	result := <-ch
	return result.repo
}

func (m *DefaultRepositoryManager) Set(repo *Repository) {
	m.repos.Store(repo.Name, &cachedRepo{
		repo:      repo,
		expiresAt: time.Now().Add(m.ttl),
	})
}

// Invalidate 使指定仓库的缓存失效，下次 Get 时会重新从 DB 加载。
func (m *DefaultRepositoryManager) Invalidate(name string) {
	m.repos.Delete(name)
}

func (m *DefaultRepositoryManager) Delete(id string) {
	m.repos.Delete(id)
}

func (m *DefaultRepositoryManager) Reload() error {
	m.repos.Range(func(key, _ interface{}) bool {
		m.repos.Delete(key)
		return true
	})
	return nil
}

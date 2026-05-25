package runtime

import "sync"

type DefaultRepositoryManager struct {
	repos sync.Map
}

func NewDefaultRepositoryManager() *DefaultRepositoryManager {
	return &DefaultRepositoryManager{}
}

func (m *DefaultRepositoryManager) Get(id string) *Repository {
	if v, ok := m.repos.Load(id); ok {
		return v.(*Repository)
	}
	return nil
}

func (m *DefaultRepositoryManager) Set(repo *Repository) {
	m.repos.Store(repo.Name, repo)
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

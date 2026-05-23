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

func (m *DefaultRepositoryManager) Reload() error {
	return nil
}

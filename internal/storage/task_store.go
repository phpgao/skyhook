package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/phpgao/skyhook/internal/model"
)

// TaskStore defines the persistence interface for task states
type TaskStore interface {
	Save(task *model.TaskRecord) error
	Get(gid string) (*model.TaskRecord, bool)
	UpdateStatus(gid string, status model.TaskStatus, errMsg string) error
	ListUnfinished() ([]*model.TaskRecord, error)
	ListAll() ([]*model.TaskRecord, error)
	Delete(gid string) error
	Close() error
}

// JSONFileTaskStore provides a zero-dependency, crash-resilient atomic file-backed TaskStore
type JSONFileTaskStore struct {
	filePath string
	mu       sync.RWMutex
	tasks    map[string]*model.TaskRecord
}

// NewJSONFileTaskStore creates or loads a JSONFileTaskStore at the given path
func NewJSONFileTaskStore(filePath string) (*JSONFileTaskStore, error) {
	if filePath == "" {
		filePath = "/tmp/skyhook_tasks.json"
	}

	store := &JSONFileTaskStore{
		filePath: filePath,
		tasks:    make(map[string]*model.TaskRecord),
	}

	if err := store.load(); err != nil {
		return nil, fmt.Errorf("failed to initialize task store from %s: %w", filePath, err)
	}

	return store, nil
}

func (s *JSONFileTaskStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	data, err := os.ReadFile(s.filePath)
	if os.IsNotExist(err) {
		// File does not exist yet; start clean
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read store file %s: %w", s.filePath, err)
	}

	if len(data) == 0 {
		return nil
	}

	var list []*model.TaskRecord
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("corrupted state file %s: %w", s.filePath, err)
	}

	for _, t := range list {
		if t != nil && t.GID != "" {
			s.tasks[t.GID] = t
		}
	}

	return nil
}

func (s *JSONFileTaskStore) flushLocked() error {
	var list []*model.TaskRecord
	for _, t := range s.tasks {
		list = append(list, t)
	}

	// Sort chronologically by CreatedAt
	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.Before(list[j].CreatedAt)
	})

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize tasks: %w", err)
	}

	tmpFile := s.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write tmp store file %s: %w", tmpFile, err)
	}

	// Atomic rename ensures file is never corrupted during sudden power loss or process kill
	if err := os.Rename(tmpFile, s.filePath); err != nil {
		return fmt.Errorf("failed to commit store file %s: %w", s.filePath, err)
	}

	return nil
}

func (s *JSONFileTaskStore) Save(task *model.TaskRecord) error {
	if task == nil || task.GID == "" {
		return fmt.Errorf("cannot save nil or empty GID task")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	cpy := *task
	cpy.UpdatedAt = time.Now()
	s.tasks[task.GID] = &cpy

	return s.flushLocked()
}

func (s *JSONFileTaskStore) Get(gid string) (*model.TaskRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	t, ok := s.tasks[gid]
	if !ok {
		return nil, false
	}
	cpy := *t
	return &cpy, true
}

func (s *JSONFileTaskStore) UpdateStatus(gid string, status model.TaskStatus, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	t, ok := s.tasks[gid]
	if !ok {
		return fmt.Errorf("task %s not found in store", gid)
	}

	t.Status = status
	t.ErrorMsg = errMsg
	t.UpdatedAt = time.Now()

	return s.flushLocked()
}

func (s *JSONFileTaskStore) ListUnfinished() ([]*model.TaskRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var res []*model.TaskRecord
	for _, t := range s.tasks {
		if t.Status == model.StatusPending || t.Status == model.StatusDownloading || t.Status == model.StatusProcessing {
			cpy := *t
			res = append(res, &cpy)
		}
	}
	return res, nil
}

func (s *JSONFileTaskStore) ListAll() ([]*model.TaskRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var res []*model.TaskRecord
	for _, t := range s.tasks {
		cpy := *t
		res = append(res, &cpy)
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].CreatedAt.Before(res[j].CreatedAt)
	})
	return res, nil
}

func (s *JSONFileTaskStore) Delete(gid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.tasks, gid)
	return s.flushLocked()
}

func (s *JSONFileTaskStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushLocked()
}

// MemoryTaskStore is an in-memory implementation of TaskStore for fast testing
type MemoryTaskStore struct {
	mu    sync.RWMutex
	tasks map[string]*model.TaskRecord
}

// NewMemoryTaskStore creates an in-memory TaskStore
func NewMemoryTaskStore() *MemoryTaskStore {
	return &MemoryTaskStore{
		tasks: make(map[string]*model.TaskRecord),
	}
}

func (m *MemoryTaskStore) Save(task *model.TaskRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cpy := *task
	cpy.UpdatedAt = time.Now()
	m.tasks[task.GID] = &cpy
	return nil
}

func (m *MemoryTaskStore) Get(gid string) (*model.TaskRecord, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[gid]
	if !ok {
		return nil, false
	}
	cpy := *t
	return &cpy, true
}

func (m *MemoryTaskStore) UpdateStatus(gid string, status model.TaskStatus, errMsg string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[gid]
	if !ok {
		return fmt.Errorf("task %s not found", gid)
	}
	t.Status = status
	t.ErrorMsg = errMsg
	t.UpdatedAt = time.Now()
	return nil
}

func (m *MemoryTaskStore) ListUnfinished() ([]*model.TaskRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*model.TaskRecord
	for _, t := range m.tasks {
		if t.Status == model.StatusPending || t.Status == model.StatusDownloading || t.Status == model.StatusProcessing {
			cpy := *t
			res = append(res, &cpy)
		}
	}
	return res, nil
}

func (m *MemoryTaskStore) ListAll() ([]*model.TaskRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var res []*model.TaskRecord
	for _, t := range m.tasks {
		cpy := *t
		res = append(res, &cpy)
	}
	return res, nil
}

func (m *MemoryTaskStore) Delete(gid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.tasks, gid)
	return nil
}

func (m *MemoryTaskStore) Close() error {
	return nil
}

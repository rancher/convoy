package util

import (
	"fmt"
	"sync"
)

type Index struct {
	data map[string]string
	lock *sync.RWMutex
}

func NewIndex() *Index {
	return &Index{data: make(map[string]string), lock: &sync.RWMutex{}}
}

func (idx *Index) Add(key, value string) error {
	if key == "" {
		return fmt.Errorf("BUG: Invalid empty index key")
	}
	if value == "" {
		return fmt.Errorf("BUG: Invalid empty index value")
	}

	idx.lock.Lock()
	defer idx.lock.Unlock()

	if oldValue, exists := idx.data[key]; exists {
		if oldValue != value {
			return fmt.Errorf("BUG: Conflict when updating index, %v was mapped to %v, but %v want to be mapped too", key, oldValue, value)
		}
		return nil
	}
	idx.data[key] = value
	return nil
}

func (idx *Index) Remove(key string) error {
	if key == "" {
		return fmt.Errorf("BUG: Invalid empty index key")
	}

	idx.lock.Lock()
	defer idx.lock.Unlock()

	if _, exists := idx.data[key]; !exists {
		return fmt.Errorf("BUG: About to remove non-existed key %v from index", key)
	}
	delete(idx.data, key)
	return nil
}

func (idx *Index) Get(key string) string {
	idx.lock.RLock()
	defer idx.lock.RUnlock()

	return idx.data[key]
}

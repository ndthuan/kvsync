package kvsync

import (
	"fmt"
	"reflect"
	"sync"
)

// InMemoryStore is an in-memory implementation of KVStore
type InMemoryStore struct {
	Store map[string]any
	mutex sync.Mutex
}

func copyFields(val interface{}, dest interface{}) error {
	vVal := reflect.ValueOf(val)
	vDest := reflect.ValueOf(dest)

	vDest = vDest.Elem()

	for i := 0; i < vDest.NumField(); i++ {
		vDest.Field(i).Set(vVal.Field(i))
	}

	return nil
}

func (m *InMemoryStore) Fetch(key string, dest any) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	val, ok := m.Store[key]
	if !ok {
		return fmt.Errorf("key %s not found", key)
	}

	return copyFields(val, dest)
}

func (m *InMemoryStore) Put(key string, value any) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Store[key] = value

	return nil
}

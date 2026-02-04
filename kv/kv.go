package kv

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// KVStore defines the interface for a key-value store
type KVStore interface {
	Get(key string) ([]byte, error)
	Put(key string, value []byte) error
	Delete(key string) error
	Close() error
}

// InMemoryKVStore is an in-memory key-value store
type InMemoryKVStore struct {
	data map[string][]byte
	mu   sync.RWMutex
}

func NewInMemoryKVStore() *InMemoryKVStore {
	return &InMemoryKVStore{
		data: make(map[string][]byte),
	}
}

func (kv *InMemoryKVStore) Get(key string) ([]byte, error) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()
	
	value, exists := kv.data[key]
	if !exists {
		return nil, fmt.Errorf("key '%s' not found", key)
	}
	
	return value, nil
}

func (kv *InMemoryKVStore) Put(key string, value []byte) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	
	kv.data[key] = value
	return nil
}

func (kv *InMemoryKVStore) Delete(key string) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	
	delete(kv.data, key)
	return nil
}

func (kv *InMemoryKVStore) Close() error {
	// Nothing to close for in-memory store
	return nil
}

// FileBasedKVStore is a file-based key-value store
type FileBasedKVStore struct {
	filePath string
	data     map[string][]byte
	mu       sync.RWMutex
}

func NewFileBasedKVStore(filePath string) *FileBasedKVStore {
	store := &FileBasedKVStore{
		filePath: filePath,
		data:     make(map[string][]byte),
	}
	
	// Try to load existing data
	store.loadFromFile()
	
	return store
}

func (kv *FileBasedKVStore) loadFromFile() error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	
	data, err := os.ReadFile(kv.filePath)
	if err != nil {
		// If file doesn't exist, that's fine - we'll create it later
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	
	return json.Unmarshal(data, &kv.data)
}

func (kv *FileBasedKVStore) saveToFile() error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	
	data, err := json.MarshalIndent(kv.data, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(kv.filePath, data, 0644)
}

func (kv *FileBasedKVStore) Get(key string) ([]byte, error) {
	kv.mu.RLock()
	defer kv.mu.RUnlock()

	value, exists := kv.data[key]
	if !exists {
		return nil, fmt.Errorf("key '%s' not found", key)
	}

	return value, nil
}

func (kv *FileBasedKVStore) Put(key string, value []byte) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	kv.data[key] = value

	return kv.saveToFileUnsafe() // Call saveToFile without lock since we already have the write lock
}

func (kv *FileBasedKVStore) Delete(key string) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()

	delete(kv.data, key)

	return kv.saveToFileUnsafe() // Call saveToFile without lock since we already have the write lock
}

// saveToFileUnsafe saves the data to file without acquiring the mutex (should be called when already locked)
func (kv *FileBasedKVStore) saveToFileUnsafe() error {
	data, err := json.MarshalIndent(kv.data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(kv.filePath, data, 0644)
}

func (kv *FileBasedKVStore) Close() error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	return kv.saveToFileUnsafe()
}
package main

import (
	"fmt"
	"os"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

const O_DIRECT = 0x4000 // Linux syscall index

type CacheEntry struct {
	path string
	data []byte
	size int64
	mu   sync.RWMutex
}

func (ce *CacheEntry) Bytes() []byte {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.data
}

func (ce *CacheEntry) Size() int64 {
	return ce.size
}

func (ce *CacheEntry) Path() string {
	return ce.path
}

type FileCache struct {
	entries   map[string]*CacheEntry
	mu        sync.RWMutex
	totalSize int64
	maxSize   int64
	blockSize int64
}

func NewFileCache(maxSize int64) (*FileCache, error) {
	// Ottieni block size reale dal filesystem
	stat := unix.Statfs_t{}
	if err := unix.Statfs("/", &stat); err != nil {
		return nil, err
	}

	return &FileCache{
		entries:   make(map[string]*CacheEntry),
		maxSize:   maxSize,
		blockSize: stat.Bsize,
	}, nil
}

func (fc *FileCache) Load(path string) (*CacheEntry, error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if entry, exists := fc.entries[path]; exists {
		return entry, nil
	}

	fd, err := os.OpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("open failed: %w", err)
	}
	defer fd.Close()

	stat, err := fd.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat failed: %w", err)
	}

	fileSize := stat.Size()
	if fc.totalSize+fileSize > fc.maxSize {
		return nil, fmt.Errorf("cache full")
	}

	// Alloca buffer roundato a block size per O_DIRECT
	roundedSize := ((fileSize + fc.blockSize - 1) / fc.blockSize) * fc.blockSize
	buffer := make([]byte, roundedSize)

	// Leggi con syscall (O_DIRECT)
	n, err := syscall.Read(int(fd.Fd()), buffer)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}

	// Copia in heap pulito (dimensione reale)
	heapData := make([]byte, fileSize)
	copy(heapData, buffer[:n])

	entry := &CacheEntry{
		path: path,
		data: heapData,
		size: fileSize,
	}

	fc.entries[path] = entry
	fc.totalSize += fileSize

	return entry, nil
}

func (fc *FileCache) Contains(path string) bool {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	_, ok := fc.entries[path]
	return ok
}

func (fc *FileCache) Evict(path string) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	entry, ok := fc.entries[path]
	if !ok {
		return fmt.Errorf("not found")
	}

	fc.totalSize -= entry.size
	delete(fc.entries, path)
	return nil
}

func (fc *FileCache) CachedSize() int64 {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.totalSize
}

package main

import (
	"fmt"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

const O_DIRECT = 0x4000 // Linux syscall index

type CacheEntry struct {
	path       string
	data       []byte
	size       int64
	addedAt    time.Time
	modifiedAt time.Time
	ready      chan struct{}
	err        error
}

func (ce *CacheEntry) Bytes() []byte {
	return ce.data
}

func (ce *CacheEntry) Size() int64 {
	return ce.size
}

func (ce *CacheEntry) Path() string {
	return ce.path
}

func (ce *CacheEntry) AddedAt() time.Time {
	return ce.addedAt
}

func (ce *CacheEntry) ModifiedAt() time.Time {
	return ce.modifiedAt
}

type LoadResult struct {
	Entry *CacheEntry
	Hit   bool
}

type FileCache struct {
	entries   map[string]*CacheEntry
	mu        sync.RWMutex
	totalSize int64
	maxSize   int64
	blockSize int64
	ttl       time.Duration
}

func NewFileCache(maxSize int64, ttl time.Duration) (*FileCache, error) {
	// Ottieni block size reale dal filesystem
	stat := unix.Statfs_t{}
	if err := unix.Statfs("/", &stat); err != nil {
		return nil, err
	}

	return &FileCache{
		entries:   make(map[string]*CacheEntry),
		maxSize:   maxSize,
		blockSize: stat.Bsize,
		ttl:       ttl,
	}, nil
}

// waitReady aspetta che il caricamento dell'entry sia completato
func waitReady(entry *CacheEntry) (*CacheEntry, error) {
	<-entry.ready
	return entry, entry.err
}

func (fc *FileCache) Load(path string) (LoadResult, error) {
	fc.mu.Lock()

	entry, exists := fc.entries[path]
	if exists && (fc.ttl == 0 || time.Since(entry.addedAt) < fc.ttl) {
		// Entry valida: rilascia subito, poi aspetta l'eventuale caricamento
		fc.mu.Unlock()
		e, err := waitReady(entry)
		if err != nil {
			return LoadResult{}, err
		}
		return LoadResult{Entry: e, Hit: true}, nil
	}

	if exists {
		// Entry scaduta: rimuovila prima di crearne una nuova
		fc.totalSize -= entry.size
		delete(fc.entries, path)
	}

	entry = &CacheEntry{path: path, addedAt: time.Now(), ready: make(chan struct{})}

	fc.entries[path] = entry
	fc.mu.Unlock()

	if err := fc.readFromDisk(entry); err != nil {
		entry.err = err
		fc.mu.Lock()
		if fc.entries[path] == entry {
			delete(fc.entries, path)
		}
		fc.mu.Unlock()

		close(entry.ready)
		return LoadResult{}, err
	}

	close(entry.ready)
	return LoadResult{Entry: entry, Hit: false}, nil
}

func alignedSlice(size, align int64) []byte {
	buf := make([]byte, size+align)
	offset := align - (int64(uintptr(unsafe.Pointer(&buf[0]))) % align)
	if offset == align {
		offset = 0
	}
	return buf[offset : offset+size]
}

func (fc *FileCache) readFromDisk(entry *CacheEntry) error {
	fd, err := syscall.Open(entry.path, syscall.O_RDONLY|O_DIRECT, 0)
	if err != nil {
		return fmt.Errorf("open failed: %w", err)
	}
	defer syscall.Close(fd)

	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return fmt.Errorf("stat failed: %w", err)
	}
	fileSize := st.Size

	fc.mu.Lock()
	if fc.totalSize+fileSize > fc.maxSize {
		fc.mu.Unlock()
		return fmt.Errorf("cache full")
	}
	fc.totalSize += fileSize
	fc.mu.Unlock()

	roundedSize := ((fileSize + fc.blockSize - 1) / fc.blockSize) * fc.blockSize
	buf := alignedSlice(roundedSize, fc.blockSize)

	n, err := syscall.Read(fd, buf)
	if err != nil {
		fc.mu.Lock()
		fc.totalSize -= fileSize
		fc.mu.Unlock()
		return fmt.Errorf("read failed: %w", err)
	}

	entry.data = buf[:n]
	entry.size = fileSize
	entry.modifiedAt = time.Unix(st.Mtim.Sec, st.Mtim.Nsec)

	return nil
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

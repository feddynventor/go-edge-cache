package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	ErrForbidden   = errors.New("forbidden")   // path traversal
	ErrUnavailable = errors.New("unavailable") // errori generici cache/IO
)

// Validators

func resolvePath(baseDir, urlPath string) (string, error) {
	// filepath.Join normalizza "..", ".", doppi slash
	fullPath := filepath.Join(baseDir, urlPath)

	// Valida che il risultato sia dentro baseDir
	basePath, _ := filepath.Abs(baseDir)
	cleanPath, _ := filepath.Abs(fullPath)

	if !strings.HasPrefix(cleanPath, basePath+string(filepath.Separator)) {
		return "", ErrForbidden
	}

	return cleanPath, nil
}

// Use dependencies

func fetchFile(fc *FileCache, contentDir, urlPath string) (*CacheEntry, error) {
	fullPath, err := resolvePath(contentDir, urlPath)
	if err != nil {
		return nil, err
	}
	entry, err := fc.Load(fullPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return entry, nil
}

// export

func serveResponse(w http.ResponseWriter, r *http.Request, entry *CacheEntry, err error) {
	if err != nil {
		switch {
		case errors.Is(err, ErrForbidden):
			http.Error(w, "Forbidden", http.StatusForbidden)
		case errors.Is(err, os.ErrNotExist):
			http.NotFound(w, r)
		default:
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	data := entry.Bytes()
	header := data
	if len(header) > 512 {
		header = header[:512]
	}
	w.Header().Set("Content-Type", http.DetectContentType(header))
	w.Header().Set("Content-Length", strconv.FormatInt(entry.Size(), 10))
	w.Write(data)
}

func fileHandler(fc *FileCache, contentDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entry, err := fetchFile(fc, contentDir, r.URL.Path)
		serveResponse(w, r, entry, err)
	}
}

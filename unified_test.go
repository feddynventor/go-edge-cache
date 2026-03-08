package main

import (
	"fmt"
	"net/http"
	"os"
	"testing"
)

func TestLoadAndDetectMIME(t *testing.T) {
	// Crea file di test
	testFile := "/tmp/test.json"
	testData := []byte(`{"name": "test", "value": 123}`)
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	// Crea cache
	cache, err := NewFileCache(10 * 1024 * 1024)

	// Carica file
	entry, err := cache.Load(testFile)
	if err != nil {
		t.Fatalf("Cache init or File load failed: %v", err)
	}

	// Leggi header (primi 512 byte)
	data := entry.Bytes()
	header := data
	if len(data) > 512 {
		header = data[:512]
	}

	// Detecta MIME
	mimeType := http.DetectContentType(header)

	fmt.Printf("File: %s\n", entry.Path())
	fmt.Printf("Size: %d\n", entry.Size())
	fmt.Printf("MIME: %s\n", mimeType)
	fmt.Printf("Cached: %v\n", cache.Contains(testFile))

	// Verifica
	if mimeType != "text/plain; charset=utf-8" {
		t.Errorf("expected text/plain, got %s", mimeType)
	}
}

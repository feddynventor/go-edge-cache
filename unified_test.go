package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePath(t *testing.T) {
	baseDir := t.TempDir()

	t.Run("valid paths", func(t *testing.T) {
		cases := []struct {
			input    string
			expected string
		}{
			{"/file.html", filepath.Join(baseDir, "file.html")},
			{"/subdir/file.html", filepath.Join(baseDir, "subdir", "file.html")},
			{"/a/b/c/deep.txt", filepath.Join(baseDir, "a", "b", "c", "deep.txt")},
			{"/file.min.js", filepath.Join(baseDir, "file.min.js")},
			{"/sub/./file.txt", filepath.Join(baseDir, "sub", "file.txt")}, // dot normalizzato
		}
		for _, tc := range cases {
			got, err := resolvePath(baseDir, tc.input)
			if err != nil {
				t.Errorf("path valida %q ha restituito errore: %v", tc.input, err)
				continue
			}
			if got != tc.expected {
				t.Errorf("path %q: atteso %q, got %q", tc.input, tc.expected, got)
			}
		}
	})

	t.Run("traversal blocked", func(t *testing.T) {
		cases := []string{
			"/../../../etc/passwd",  // traversal classico multi-livello
			"/../secret",            // uscita diretta da baseDir
			"/subdir/../../outside", // uscita tramite subdir
			"../outside",            // senza leading slash
			"/subdir/../../../etc",  // traversal profondo da subdir
			"/./../../etc/passwd",   // mix di . e ..
			"/subdir/../../",        // trailing slash fuori da baseDir
			"/../",                  // root con trailing slash
		}
		for _, p := range cases {
			_, err := resolvePath(baseDir, p)
			if err == nil {
				t.Errorf("traversal non bloccato per %q", p)
			} else if !errors.Is(err, ErrForbidden) {
				t.Errorf("traversal %q: atteso ErrForbidden, got %v", p, err)
			}
		}
	})

	t.Run("root path blocked", func(t *testing.T) {
		// "/" risolve in baseDir stesso, non in un file dentro baseDir
		_, err := resolvePath(baseDir, "/")
		if err == nil {
			t.Error("path root \"/\" dovrebbe essere bloccata (nessun file specificato)")
		} else if !errors.Is(err, ErrForbidden) {
			t.Errorf("path root: atteso ErrForbidden, got %v", err)
		}
	})

	t.Run("empty path blocked", func(t *testing.T) {
		_, err := resolvePath(baseDir, "")
		if err == nil {
			t.Error("path vuota dovrebbe essere bloccata")
		} else if !errors.Is(err, ErrForbidden) {
			t.Errorf("path vuota: atteso ErrForbidden, got %v", err)
		}
	})

	t.Run("result is always inside baseDir", func(t *testing.T) {
		inputs := []string{
			"/a.txt",
			"/sub/b.txt",
			"/x/y/z/c.txt",
		}
		for _, p := range inputs {
			got, err := resolvePath(baseDir, p)
			if err != nil {
				t.Errorf("path valida %q: errore inatteso: %v", p, err)
				continue
			}
			absBase, _ := filepath.Abs(baseDir)
			if !strings.HasPrefix(got, absBase+string(filepath.Separator)) {
				t.Errorf("risultato %q fuori da baseDir %q", got, absBase)
			}
		}
	})
}

func TestFetchFile(t *testing.T) {
	contentDir := t.TempDir()
	content := []byte("ciao")
	os.WriteFile(filepath.Join(contentDir, "hello.txt"), content, 0644)

	fc, err := NewFileCache(10*1024*1024, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Prima chiamata: cache miss
	result, err := fetchFile(fc, contentDir, "/hello.txt")
	if err != nil {
		t.Fatalf("file esistente: errore inatteso: %v", err)
	}
	if result.Hit {
		t.Error("prima chiamata: atteso Hit=false (cache miss)")
	}
	if result.Entry == nil {
		t.Fatal("prima chiamata: Entry nil")
	}
	if string(result.Entry.Bytes()) != string(content) {
		t.Errorf("contenuto inatteso: got %q, want %q", result.Entry.Bytes(), content)
	}
	if result.Entry.Size() != int64(len(content)) {
		t.Errorf("size inattesa: got %d, want %d", result.Entry.Size(), len(content))
	}
	if result.Entry.Path() == "" {
		t.Error("Entry.Path() vuoto")
	}
	if result.Entry.AddedAt().IsZero() {
		t.Error("Entry.AddedAt() zero")
	}
	if result.Entry.ModifiedAt().IsZero() {
		t.Error("Entry.ModifiedAt() zero")
	}

	// Seconda chiamata: cache hit
	result2, err := fetchFile(fc, contentDir, "/hello.txt")
	if err != nil {
		t.Fatalf("seconda chiamata: errore inatteso: %v", err)
	}
	if !result2.Hit {
		t.Error("seconda chiamata: atteso Hit=true (cache hit)")
	}
	if result2.Entry == nil {
		t.Fatal("seconda chiamata: Entry nil")
	}
	if string(result2.Entry.Bytes()) != string(content) {
		t.Errorf("seconda chiamata: contenuto inatteso: %q", result2.Entry.Bytes())
	}

	// Path traversal → ErrForbidden
	_, err = fetchFile(fc, contentDir, "/../../../etc/passwd")
	if !errors.Is(err, ErrForbidden) {
		t.Errorf("traversal: atteso ErrForbidden, got %v", err)
	}

	// File non esistente → os.ErrNotExist
	_, err = fetchFile(fc, contentDir, "/nonexistent.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file mancante: atteso os.ErrNotExist, got %v", err)
	}
}

func TestLoadAndDetectMIME(t *testing.T) {
	// Crea file di test
	testFile := "/tmp/test.json"
	testData := []byte(`{"name": "test", "value": 123}`)
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(testFile)

	// Crea cache
	cache, err := NewFileCache(10*1024*1024, 0)

	// Carica file
	result, err := cache.Load(testFile)
	if err != nil {
		t.Fatalf("Cache init or File load failed: %v", err)
	}
	entry := result.Entry

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
	fmt.Printf("Hit: %v\n", result.Hit)
	fmt.Printf("Cached: %v\n", cache.Contains(testFile))

	// Verifica
	if mimeType != "text/plain; charset=utf-8" {
		t.Errorf("expected text/plain, got %s", mimeType)
	}
}

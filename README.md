# Edge Cache with GoLang

High-performance HTTP/S server progettato per il serving di contenuti statici con latenza ridotta tramite caching in-memory e logging asincrono.

## Classi
### FileCache

FileCache carica file interi in heap bypassando la page cache del kernel via O_DIRECT. Ogni eviction è controllata esclusivamente dal CDN, eliminando contention con l'algoritmo di paging del kernel.

* O_DIRECT isolation: nessuna competizione tra kernel page cache e (eventuale) algoritmo CDN di eviction
* Block-aligned I/O: rispetta vincoli block size del filesystem

#### Esempio

```
cache := NewFileCache(100 * 1024 * 1024) // 100MB heap
entry, err := cache.Load("/path/to/file")
data := entry.Bytes()
cache.Evict("/path/to/file")
```

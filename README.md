# Edge Cache with GoLang

High-performance HTTP/S server progettato per il serving di contenuti statici con latenza ridotta tramite caching in-memory e logging asincrono.

How to Run:
```
go build
./go-edge-cache <yaml config file>
```

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

### Server
`CacheServer` astrae `http.Handler` ed è l'unico punto di ingresso HTTP; riceve `FileCache` e `RequestQueue` tramite dependency injection

#### Features alto livello
* **Path traversal protection**: `filepath.Join` + `filepath.Abs` garantiscono che il path finale abbia come prefisso `baseDir`; qualsiasi tentativo di uscire dalla directory radice restituisce 403
* **Auto content-type**: sniffa i primi 512 byte via `http.DetectContentType`
* **HTTPS only**: nessun fallback in chiaro

#### Threading interessato
Ogni richiesta viene tracciata in una coda interna senza bloccare il goroutine HTTP. Le responsabilità sono distribuite su tre componenti:

`CacheServer.ServeHTTP` serve la richiesta, determina status e size dai valori di dominio, invia un `RequestEntry` alla coda

`RequestQueue` mantiene il channel bufferizzato e il goroutine consumer dedicato che scrive

`main.go` costruisce le dipendenze nell'ordine corretto e orchestra lo shutdown

#### Flusso
goroutine HTTP (net/http, N paralleli)
```
CacheServer.ServeHTTP()
  fetchFile()          → entry, err
  serveResponse()      → risposta al client
  Enqueue(RequestEntry)
    select {
      case ch <- entry:  ──────────────►  for entry := range ch {
      default: // drop se pieno               fmt.Fprintf(stdout, ...)
    }                                     }
```

#### Shutdown
alla ricezione tramite `signal` di SIGINT o SIGTERM
* srv.Shutdown()       // smette di accettare richieste, attende i goroutine HTTP in volo per implementazione di net/http
* rq.Shutdown()        // chiude il channel, attende che il consumer svuoti la coda
* exit(0)
```

Questa sequenza garantisce che nessun `RequestEntry` venga perso: tutti i goroutine HTTP terminano prima che la coda venga chiusa.

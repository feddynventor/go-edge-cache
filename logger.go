package main

import (
	"fmt"
	"log"
	"os"
	"time"
)

type RequestEntry struct {
	Timestamp  time.Time
	Method     string
	Path       string
	Status     int
	Size       int
	Duration   time.Duration
	RemoteAddr string
}

// buffered channels for req details and done
type RequestQueue struct {
	ch   chan RequestEntry
	done chan struct{}
	file *os.File
}

func NewRequestQueue(queueSize int, logPath string) *RequestQueue {
	if queueSize <= 0 {
		queueSize = 1000
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Fatalf("failed to open log file: %v", err)
	}
	rq := &RequestQueue{
		ch:   make(chan RequestEntry, queueSize),
		done: make(chan struct{}),
		file: f,
	}
	go rq.consume()
	return rq
}

func (rq *RequestQueue) Enqueue(e RequestEntry) {
	select {
	case rq.ch <- e:
	default:
	}
}

// goroutine that consumes with range until channel has data
func (rq *RequestQueue) consume() {
	for entry := range rq.ch {
		fmt.Fprintf(
			rq.file,
			"%s method=%s path=%s status=%d size=%d duration_ms=%.2f remote=%s\n",
			entry.Timestamp.UTC().Format(time.RFC3339),
			entry.Method,
			entry.Path,
			entry.Status,
			entry.Size,
			float64(entry.Duration.Microseconds())/1000.0,
			entry.RemoteAddr,
		)
	}
	rq.file.Close()
	rq.done <- struct{}{}
}

func (rq *RequestQueue) Shutdown() {
	close(rq.ch)
	<-rq.done
}

package main

import (
	"fmt"
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
}

func NewRequestQueue(queueSize int) *RequestQueue {
	if queueSize <= 0 {
		queueSize = 1000
	}
	rq := &RequestQueue{
		ch:   make(chan RequestEntry, queueSize),
		done: make(chan struct{}),
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
			os.Stdout,
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
	rq.done <- struct{}{}
}

func (rq *RequestQueue) Shutdown() {
	close(rq.ch)
	<-rq.done
}

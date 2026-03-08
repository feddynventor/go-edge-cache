package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.yaml.in/yaml/v4"
)

type Config struct {
	Host struct {
		Iface string `yaml:"iface"`
		Port  int    `yaml:"port"`
	} `yaml:"host"`
	Server struct {
		ContentDirectory string `yaml:"content_directory"`
		TLS              struct {
			Cert    string `yaml:"cert"`
			CertKey string `yaml:"cert_key"`
		} `yaml:"tls"`
	} `yaml:"server"`
	Logging struct {
		LogPath      string `yaml:"log_path"`
		QueueMaxSize int    `yaml:"queue_max_size"`
	} `yaml:"logging"`
	Cache struct {
		TTL int `yaml:"ttl"`
	} `yaml:"cache"`
}

func main() {
	filePath := "./config.yaml"
	if len(os.Args) == 2 {
		filePath = os.Args[1]
	}

	raw_config, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Println("Error reading specified config file:", err)
		os.Exit(1)
	}

	var cfg Config
	err = yaml.Unmarshal(raw_config, &cfg)
	if err != nil {
		fmt.Println("Error parsing config YAML:", err)
		os.Exit(1)
	}

	// Dependencies

	fc, err := NewFileCache(256*1024*1024, time.Duration(cfg.Cache.TTL)*time.Second)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	rq := NewRequestQueue(cfg.Logging.QueueMaxSize) // starts logger goroutine

	mux := http.NewServeMux()
	mux.Handle("/", &CacheServer{
		baseDir: cfg.Server.ContentDirectory,
		fc:      fc,
		rq:      rq,
	})

	// Runtime

	addr := fmt.Sprintf("%s:%d", cfg.Host.Iface, cfg.Host.Port)
	srv := &http.Server{Addr: addr, Handler: mux}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			fmt.Println("shutdown error:", err)
		}
	}()

	if err := srv.ListenAndServeTLS(cfg.Server.TLS.Cert, cfg.Server.TLS.CertKey); err != nil && err != http.ErrServerClosed {
		fmt.Println(err)
		os.Exit(1)
	}

	rq.Shutdown()
	fmt.Println("shutdown complete")
}

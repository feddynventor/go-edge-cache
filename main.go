package main

import (
	"fmt"
	"net/http"
	"os"
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

	fc, err := NewFileCache(256*1024*1024, time.Duration(cfg.Cache.TTL)*time.Second)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host.Iface, cfg.Host.Port)
	mux := http.NewServeMux()

	mux.HandleFunc("/", fileHandler(fc, cfg.Server.ContentDirectory))

	if err := http.ListenAndServeTLS(addr, cfg.Server.TLS.Cert, cfg.Server.TLS.CertKey, mux); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

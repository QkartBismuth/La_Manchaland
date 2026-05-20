package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	Controller ControllerConfig `json:"controller"`
}

type ControllerConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	ModelPath    string `json:"model_path"`
	ContextSize  int    `json:"context_size"`
	Threads      int    `json:"threads"`
	RPCWorkers   []string `json:"rpc_workers"`
	AutoDiscover bool   `json:"auto_discover"`
}

type WorkerConfig struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	ControllerIP string `json:"controller_ip"`
	Name         string `json:"name"`
	CudaLayers   int    `json:"cuda_layers"`
}

func LoadController(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultConfig(), nil
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), nil
	}

	return &cfg, nil
}

func LoadWorker(path string) (*WorkerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultWorkerConfig(), nil
	}

	var cfg WorkerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultWorkerConfig(), nil
	}

	return &cfg, nil
}

func DefaultConfig() *Config {
	return &Config{
		Controller: ControllerConfig{
			Host:         "0.0.0.0",
			Port:         8080,
			ContextSize:  4096,
			Threads:      0,
			AutoDiscover: true,
		},
	}
}

func DefaultWorkerConfig() *WorkerConfig {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "worker"
	}

	return &WorkerConfig{
		Host:         "0.0.0.0",
		Port:         50051,
		ControllerIP: "",
		Name:         hostname,
		CudaLayers:   -1,
	}
}

func ConfigDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

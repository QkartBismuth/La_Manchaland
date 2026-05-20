package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lamanchaland/llama-distributed/internal/config"
	"github.com/lamanchaland/llama-distributed/internal/discovery"
	"github.com/lamanchaland/llama-distributed/internal/monitor"
)

func main() {
	cfgPath := flag.String("config", "", "Path to worker config file")
	host := flag.String("host", "", "Host to bind to")
	port := flag.Int("port", 0, "Port for RPC server")
	name := flag.String("name", "", "Worker name")
	cudaLayers := flag.Int("cuda-layers", -1, "Number of layers to offload to CUDA (-1 = all)")
	rpcHost := flag.String("rpc-host", "127.0.0.1", "Host for RPC server")
	rpcPort := flag.Int("rpc-port", 0, "Port for RPC server")
	flag.Parse()

	cfg := config.DefaultWorkerConfig()

	if *cfgPath != "" {
		if c, err := config.LoadWorker(*cfgPath); err == nil {
			cfg = c
		}
	}

	if *host != "" {
		cfg.Host = *host
	}
	if *port != 0 {
		cfg.Port = *port
	}
	if *name != "" {
		cfg.Name = *name
	}
	if *rpcHost != "127.0.0.1" {
		cfg.Host = *rpcHost
	}
	if *rpcPort != 0 {
		cfg.Port = *rpcPort
	}
	if *cudaLayers != -1 {
		cfg.CudaLayers = *cudaLayers
	}

	log.Printf("=== La Manchaland Worker ===")
	log.Printf("Name: %s", cfg.Name)
	log.Printf("Host: %s", cfg.Host)
	log.Printf("Port: %d", cfg.Port)
	log.Printf("CUDA Layers: %d", cfg.CudaLayers)

	mon := monitor.New()
	log.Printf("CUDA detected: %v", mon.HasCUDA())

	disc := discovery.NewDiscovery()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalCh
		log.Println("Shutting down worker...")
		cancel()
	}()

	metadata := map[string]string{
		discovery.TxtKeyName: cfg.Name,
		discovery.TxtKeyPort: fmt.Sprintf("%d", cfg.Port),
	}

	go func() {
		metrics := mon.GetMetrics()
		if len(metrics.GPUs) > 0 {
			metadata[discovery.TxtKeyGPU] = metrics.GPUs[0].Name
			metadata[discovery.TxtKeyVRAM] = fmt.Sprintf("%d", metrics.GPUs[0].MemoryTotal)
		}
		metadata[discovery.TxtKeyRAM] = fmt.Sprintf("%d", metrics.MemoryTotal)
		metadata[discovery.TxtKeyCPU] = fmt.Sprintf("%.1f", metrics.CPULoad)

		if err := disc.StartWorkerBroadcast(ctx, cfg.Name, cfg.Port, metadata); err != nil {
			log.Printf("[mDNS] Broadcast error: %v", err)
		}
	}()

	go mon.StartReporting(ctx, func(m monitor.SystemMetrics) {
		key := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
		gpuName := ""
		vramTotal := uint64(0)
		if len(m.GPUs) > 0 {
			gpuName = m.GPUs[0].Name
			vramTotal = m.GPUs[0].MemoryTotal
		}

		disc.UpdateWorkerMetrics(key, func(w *discovery.WorkerInfo) {
			w.GPUName = gpuName
			w.VRAMTotal = vramTotal
			w.VRAMUsed = 0
			if len(m.GPUs) > 0 {
				w.VRAMUsed = m.GPUs[0].MemoryUsed
			}
			w.RAMTotal = m.MemoryTotal
			w.RAMUsed = m.MemoryUsed
			w.CPULoad = m.CPULoad
			w.Status = "available"
		})
	}, 5*time.Second)

	log.Printf("Worker '%s' is running. Press Ctrl+C to stop.", cfg.Name)

	llamaRPCPath := findLLAMARPCServer()
	if llamaRPCPath != "" {
		log.Printf("Starting llama-rpc-server: %s --host %s --port %d --n-gpu-layers %d",
			llamaRPCPath, cfg.Host, cfg.Port, cfg.CudaLayers)
		go startLLAMARPCServer(ctx, llamaRPCPath, cfg)
	} else {
		log.Println("WARNING: llama-rpc-server not found. Worker will register but cannot serve RPC requests.")
		log.Println("Place llama-rpc-server binary in the same directory as the worker.")
	}

	<-ctx.Done()
	log.Println("Worker stopped.")
}

func findLLAMARPCServer() string {
	paths := []string{
		"llama-rpc-server",
		"llama-rpc-server.exe",
		"./llama-rpc-server",
		"./llama-rpc-server.exe",
		"../bin/llama-rpc-server",
		"../bin/llama-rpc-server.exe",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

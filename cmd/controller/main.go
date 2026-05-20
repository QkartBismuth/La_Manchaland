package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lamanchaland/llama-distributed/internal/api"
	"github.com/lamanchaland/llama-distributed/internal/config"
	"github.com/lamanchaland/llama-distributed/internal/discovery"
	"github.com/lamanchaland/llama-distributed/internal/llmserver"
	"github.com/lamanchaland/llama-distributed/internal/monitor"
	"github.com/lamanchaland/llama-distributed/webfs"
)

func main() {
	cfgPath := flag.String("config", "", "Path to controller config file")
	host := flag.String("host", "0.0.0.0", "Host to bind to")
	port := flag.Int("port", 0, "Port for web UI and API")
	modelPath := flag.String("model", "", "Path to GGUF model file")
	contextSize := flag.Int("ctx-size", 0, "Context size")
	threads := flag.Int("threads", 0, "Number of threads (0 = auto)")
	autoDiscover := flag.Bool("auto-discover", true, "Enable mDNS auto-discovery")
	flag.Parse()

	cfg := config.DefaultConfig()

	if *cfgPath != "" {
		if c, err := config.LoadController(*cfgPath); err == nil {
			cfg = c
		}
	}

	if *host != "0.0.0.0" {
		cfg.Controller.Host = *host
	}
	if *port != 0 {
		cfg.Controller.Port = *port
	}
	if *modelPath != "" {
		cfg.Controller.ModelPath = *modelPath
	}
	if *contextSize != 0 {
		cfg.Controller.ContextSize = *contextSize
	}
	if *threads != 0 {
		cfg.Controller.Threads = *threads
	}
	if !*autoDiscover {
		cfg.Controller.AutoDiscover = false
	}

	log.Printf("=== La Manchaland Controller ===")
	log.Printf("Host: %s", cfg.Controller.Host)
	log.Printf("Port: %d", cfg.Controller.Port)
	log.Printf("Model: %s", cfg.Controller.ModelPath)
	log.Printf("Context Size: %d", cfg.Controller.ContextSize)
	log.Printf("Auto Discover: %v", cfg.Controller.AutoDiscover)

	mon := monitor.New()
	log.Printf("CUDA detected: %v", mon.HasCUDA())

	disc := discovery.NewDiscovery()

	llm := llmserver.New(8081)
	if cfg.Controller.ModelPath != "" {
		llm.SetModelPath(cfg.Controller.ModelPath)
	}
	if cfg.Controller.ContextSize != 0 {
		llm.SetCtxSize(cfg.Controller.ContextSize)
	}
	if cfg.Controller.Threads != 0 {
		llm.SetThreads(cfg.Controller.Threads)
	}

	apiServer := api.NewServer(disc, mon, webfs.Get(), llm)
	apiServer.SetModelPath(cfg.Controller.ModelPath)

	for _, worker := range cfg.Controller.RPCWorkers {
		var wHost string
		var wPort int
		fmt.Sscanf(worker, "%[^:]:%d", &wHost, &wPort)
		if wHost != "" && wPort > 0 {
			apiServer.AddRPCWorker(wHost, wPort)
			disc.AddManualWorker(wHost, wPort, fmt.Sprintf("worker-%s:%d", wHost, wPort))
		}
	}

	if len(cfg.Controller.RPCWorkers) > 0 {
		llm.SetRPCWorkers(cfg.Controller.RPCWorkers)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-signalCh
		log.Println("Shutting down controller...")
		cancel()
	}()

	if cfg.Controller.AutoDiscover {
		go func() {
			if err := disc.StartScanner(ctx); err != nil {
				log.Printf("[mDNS] Scanner error: %v", err)
			}
		}()
	}

	go func() {
		mon.StartReporting(ctx, func(m monitor.SystemMetrics) {}, 10*time.Second)
	}()

	addr := fmt.Sprintf("%s:%d", cfg.Controller.Host, cfg.Controller.Port)
	log.Printf("Starting web UI on http://%s", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: apiServer.Router(),
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Controller stopped.")
}

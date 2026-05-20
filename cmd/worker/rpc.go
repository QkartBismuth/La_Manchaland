package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"github.com/lamanchaland/llama-distributed/internal/config"
)

func startLLAMARPCServer(ctx context.Context, path string, cfg *config.WorkerConfig) {
	args := []string{
		"--host", cfg.Host,
		"--port", fmt.Sprintf("%d", cfg.Port),
		"--n-gpu-layers", fmt.Sprintf("%d", cfg.CudaLayers),
	}

	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: false}
	}

	log.Printf("[rpc-server] Starting: %s %v", path, args)

	if err := cmd.Run(); err != nil {
		if ctx.Err() == nil {
			log.Printf("[rpc-server] Error: %v", err)
		}
	}

	log.Printf("[rpc-server] Stopped")
}

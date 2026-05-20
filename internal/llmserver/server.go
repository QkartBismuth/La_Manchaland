package llmserver

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Server struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	cancel      context.CancelFunc
	backendURL  string
	listenPort  int
	modelPath   string
	mmprojPath  string
	ctxSize     int
	nGPU        int
	rpcWorkers  []string
	nThreads    int
	nPredict    int
	isRunning   bool
}

func New(listenPort int) *Server {
	return &Server{
		backendURL: fmt.Sprintf("http://127.0.0.1:%d", listenPort),
		listenPort: listenPort,
		nGPU:       -1,
		ctxSize:    4096,
		nThreads:   0,
		nPredict:   -1,
	}
}

func (s *Server) SetModelPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelPath = path
}

func (s *Server) SetMMProjPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mmprojPath = path
}

func (s *Server) SetCtxSize(size int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ctxSize = size
}

func (s *Server) SetNGPU(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nGPU = n
}

func (s *Server) SetThreads(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nThreads = n
}

func (s *Server) SetNPredict(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nPredict = n
}

func (s *Server) SetRPCWorkers(workers []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rpcWorkers = workers
}

func (s *Server) BackendURL() string {
	return s.backendURL
}

func (s *Server) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning
}

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.modelPath == "" {
		return fmt.Errorf("no model path set")
	}

	if s.isRunning {
		s.stopLocked()
	}

	llamaPath := findLLamaServer()
	if llamaPath == "" {
		return fmt.Errorf("llama-server binary not found")
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	args := []string{
		"--model", s.modelPath,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(s.listenPort),
		"--ctx-size", strconv.Itoa(s.ctxSize),
		"--log-disable",
		"--no-display-prompt",
	}

	if s.mmprojPath != "" {
		args = append(args, "--mmproj", s.mmprojPath)
	}

	if s.nGPU >= 0 {
		args = append(args, "--n-gpu-layers", strconv.Itoa(s.nGPU))
	}

	if s.nThreads > 0 {
		args = append(args, "--threads", strconv.Itoa(s.nThreads))
	}

	if s.nPredict > 0 {
		args = append(args, "--n-predict", strconv.Itoa(s.nPredict))
	}

	for _, rpc := range s.rpcWorkers {
		args = append(args, "--rpc", rpc)
	}

	log.Printf("[llmserver] Starting: %s %s", llamaPath, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, llamaPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	s.cmd = cmd
	s.isRunning = true

	go func() {
		err := cmd.Run()
		s.mu.Lock()
		s.isRunning = false
		s.mu.Unlock()
		if err != nil && ctx.Err() == nil {
			log.Printf("[llmserver] Server exited with error: %v", err)
		} else {
			log.Printf("[llmserver] Server stopped")
		}
	}()

	for i := 0; i < 30; i++ {
		time.Sleep(500 * time.Millisecond)
		resp, err := http.Get(s.backendURL + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Printf("[llmserver] Server ready on %s", s.backendURL)
				return nil
			}
		}
	}

	return fmt.Errorf("llama-server failed to start within timeout")
}

func (s *Server) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

func (s *Server) stopLocked() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.isRunning = false
	s.cmd = nil
}

func (s *Server) Restart() error {
	s.mu.Lock()
	path := s.modelPath
	s.mu.Unlock()
	if path == "" {
		return fmt.Errorf("no model loaded")
	}
	return s.Start()
}

func (s *Server) HealthCheck() bool {
	s.mu.Lock()
	url := s.backendURL
	s.mu.Unlock()

	resp, err := http.Get(url + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (s *Server) StreamProxy(w http.ResponseWriter, r *http.Request) error {
	s.mu.Lock()
	url := s.backendURL
	s.mu.Unlock()

	reqURL := url + r.URL.Path + "?" + r.URL.RawQuery

	bodyReader := r.Body
	if r.Body != nil {
		defer r.Body.Close()
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, reqURL, bodyReader)
	if err != nil {
		return err
	}

	for k, v := range r.Header {
		proxyReq.Header[k] = v
	}
	proxyReq.Header.Set("Host", "127.0.0.1")

	client := &http.Client{}
	resp, err := client.Do(proxyReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)

	_, err = io.Copy(w, resp.Body)
	return err
}

func (s *Server) ProxyRequest(r *http.Request) (*http.Response, error) {
	s.mu.Lock()
	url := s.backendURL
	s.mu.Unlock()

	reqURL := url + r.URL.Path + "?" + r.URL.RawQuery

	bodyReader := r.Body
	if r.Body != nil {
		defer r.Body.Close()
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}

	for k, v := range r.Header {
		proxyReq.Header[k] = v
	}
	proxyReq.Header.Set("Host", "127.0.0.1")

	client := &http.Client{}
	return client.Do(proxyReq)
}

func findLLamaServer() string {
	paths := []string{
		"llama-server",
		"llama-server.exe",
		"./llama-server",
		"./llama-server.exe",
		"../bin/llama-server",
		"../bin/llama-server.exe",
		"../../bin/llama-server",
		"../../bin/llama-server.exe",
	}

	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			abs, _ := exec.LookPath(p)
			return abs
		}
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}

func GetModels(modelPath string) []map[string]interface{} {
	modelName := "model"
	if modelPath != "" {
		base := modelPath
		if idx := strings.LastIndex(base, "/"); idx >= 0 {
			base = base[idx+1:]
		}
		if idx := strings.LastIndex(base, "\\"); idx >= 0 {
			base = base[idx+1:]
		}
		if idx := strings.LastIndex(base, "."); idx >= 0 {
			base = base[:idx]
		}
		modelName = base
	}

	return []map[string]interface{}{
		{
			"id":       modelName,
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "la-manchaland",
			"meta": map[string]interface{}{
				"path": modelPath,
			},
		},
	}
}

func (s *Server) StartNonBlocking() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.modelPath == "" || s.isRunning {
		return
	}

	llamaPath := findLLamaServer()
	if llamaPath == "" {
		log.Printf("[llmserver] llama-server binary not found")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	args := []string{
		"--model", s.modelPath,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(s.listenPort),
		"--ctx-size", strconv.Itoa(s.ctxSize),
		"--log-disable",
		"--no-display-prompt",
	}

	if s.mmprojPath != "" {
		args = append(args, "--mmproj", s.mmprojPath)
	}

	if s.nGPU >= 0 {
		args = append(args, "--n-gpu-layers", strconv.Itoa(s.nGPU))
	}

	if s.nThreads > 0 {
		args = append(args, "--threads", strconv.Itoa(s.nThreads))
	}

	if s.nPredict > 0 {
		args = append(args, "--n-predict", strconv.Itoa(s.nPredict))
	}

	for _, rpc := range s.rpcWorkers {
		args = append(args, "--rpc", rpc)
	}

	log.Printf("[llmserver] Starting in background: %s", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, llamaPath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	s.cmd = cmd
	s.isRunning = true

	go func() {
		err := cmd.Run()
		s.mu.Lock()
		s.isRunning = false
		s.mu.Unlock()
		if err != nil && ctx.Err() == nil {
			log.Printf("[llmserver] Server exited: %v", err)
		}
	}()

	go func() {
		for i := 0; i < 60; i++ {
			time.Sleep(500 * time.Millisecond)
			resp, err := http.Get(s.backendURL + "/health")
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					log.Printf("[llmserver] Server ready on %s", s.backendURL)
					return
				}
			}
		}
		log.Printf("[llmserver] Server failed to become ready")
	}()
}

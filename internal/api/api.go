package api

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"github.com/lamanchaland/llama-distributed/internal/discovery"
	"github.com/lamanchaland/llama-distributed/internal/monitor"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Server struct {
	discovery     *discovery.Discovery
	monitor       *monitor.Monitor
	staticFS      http.FileSystem
	mu            sync.RWMutex
	clients       map[*websocket.Conn]bool
	modelPath     string
	mmprojPath    string
	modelLoaded   bool
	rpcWorkers    []string
	llamaServer   *exec.Cmd
	llamaServerMu sync.Mutex
	generating    atomic.Bool
}

func NewServer(disc *discovery.Discovery, mon *monitor.Monitor, staticFS http.FileSystem) *Server {
	return &Server{
		discovery:  disc,
		monitor:    mon,
		staticFS:   staticFS,
		clients:    make(map[*websocket.Conn]bool),
		rpcWorkers: make([]string, 0),
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

func (s *Server) SetModelLoaded(loaded bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelLoaded = loaded
}

func (s *Server) AddRPCWorker(host string, port int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rpcWorkers = append(s.rpcWorkers, fmt.Sprintf("%s:%d", host, port))
	s.broadcastUpdate()
}

func (s *Server) RemoveRPCWorker(index int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index >= 0 && index < len(s.rpcWorkers) {
		s.rpcWorkers = append(s.rpcWorkers[:index], s.rpcWorkers[index+1:]...)
		s.broadcastUpdate()
	}
}

func (s *Server) Router() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/workers", s.handleWorkers)
	mux.HandleFunc("/api/workers/manual", s.handleAddWorker)
	mux.HandleFunc("/api/model", s.handleModel)
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/", s.handleStatic)

	return mux
}

func (s *Server) handleWorkers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		workers := s.discovery.GetWorkers()
		respondJSON(w, http.StatusOK, workers)

	case http.MethodDelete:
		s.mu.RLock()
		defer s.mu.RUnlock()
		var req struct {
			Host string `json:"host"`
			Port int    `json:"port"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		key := fmt.Sprintf("%s:%d", req.Host, req.Port)
		s.discovery.RemoveWorker(key)
		s.broadcastUpdate()
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func (s *Server) handleAddWorker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	var req struct {
		Host string `json:"host"`
		Port int    `json:"port"`
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if req.Host == "" || req.Port == 0 {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "host and port required"})
		return
	}

	if req.Name == "" {
		req.Name = fmt.Sprintf("worker-%s:%d", req.Host, req.Port)
	}

	s.discovery.AddManualWorker(req.Host, req.Port, req.Name)
	s.AddRPCWorker(req.Host, req.Port)
	s.broadcastUpdate()

	respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "name": req.Name})
}

func (s *Server) handleModel(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.RLock()
		defer s.mu.RUnlock()
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"path":    s.modelPath,
			"loaded":  s.modelLoaded,
			"workers": s.rpcWorkers,
		})

	case http.MethodPost:
		var req struct {
			Path   string `json:"path"`
			MMProj string `json:"mmproj"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		s.SetModelPath(req.Path)
		if req.MMProj != "" {
			s.SetMMProjPath(req.MMProj)
		}
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "path": req.Path})
	}
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	metrics := s.monitor.GetMetrics()
	respondJSON(w, http.StatusOK, metrics)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[ws] Upgrade error: %v", err)
		return
	}

	s.mu.Lock()
	s.clients[conn] = true
	s.mu.Unlock()

	log.Printf("[ws] Client connected, total: %d", len(s.clients))

	go func() {
		<-r.Context().Done()
		s.mu.Lock()
		delete(s.clients, conn)
		s.mu.Unlock()
		conn.Close()
		log.Printf("[ws] Client disconnected, total: %d", len(s.clients))
	}()
}

func (s *Server) broadcastUpdate() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data := map[string]interface{}{
		"workers": s.discovery.GetWorkers(),
		"model": map[string]interface{}{
			"path":   s.modelPath,
			"loaded": s.modelLoaded,
		},
		"rpc_workers": s.rpcWorkers,
	}

	payload, _ := json.Marshal(data)

	for client := range s.clients {
		err := client.WriteMessage(websocket.TextMessage, payload)
		if err != nil {
			client.Close()
			delete(s.clients, client)
		}
	}
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if s.staticFS == nil {
		http.Error(w, "Static files not available", http.StatusNotFound)
		return
	}

	path := r.URL.Path
	if path == "/" || path == "" {
		path = "index.html"
	} else {
		path = path[1:] // Remove leading slash
	}

	f, err := s.staticFS.Open(path)
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if stat.IsDir() {
		idx, err := s.staticFS.Open("index.html")
		if err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		defer idx.Close()
		idxStat, _ := idx.Stat()
		http.ServeContent(w, r, "index.html", idxStat.ModTime(), idx.(io.ReadSeeker))
		return
	}

	http.ServeContent(w, r, path, stat.ModTime(), f.(io.ReadSeeker))
}

type ChatRequest struct {
	Message string        `json:"message"`
	History []ChatMessage `json:"history"`
	Stream  bool          `json:"stream"`
	Image   string        `json:"image"`
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
}

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	s.mu.RLock()
	modelPath := s.modelPath
	s.mu.RUnlock()

	if modelPath == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "no model loaded"})
		return
	}

	if !s.generating.CompareAndSwap(false, true) {
		respondJSON(w, http.StatusConflict, map[string]string{"error": "already generating"})
		return
	}
	defer s.generating.Store(false)

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.generating.Store(false)
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	llamaPath := s.findLLamaServer()
	if llamaPath == "" {
		s.generating.Store(false)
		respondJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "llama-server not found"})
		return
	}

	s.mu.Lock()
	s.modelLoaded = true
	s.mu.Unlock()

	s.startLLamaServerIfNeeded(llamaPath)

	prompt := s.buildPrompt(req.Message, req.History)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.generating.Store(false)
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	log.Printf("[chat] Processing: %s", prompt[:min(100, len(prompt))])

	args := []string{
		"--model", modelPath,
		"--prompt", prompt,
		"--n-predict", "512",
		"--temp", "0.7",
		"--top-p", "0.9",
		"--top-k", "40",
		"--repeat-penalty", "1.1",
		"--no-display-prompt",
		"--log-disable",
	}

	s.mu.RLock()
	mmproj := s.mmprojPath
	s.mu.RUnlock()

	var imageFile string
	if req.Image != "" && mmproj != "" {
		args = append(args, "--mmproj", mmproj)

		imageFile, _ = s.saveBase64Image(req.Image)
		if imageFile != "" {
			args = append(args, "--image", imageFile)
		}
	}

	cmd := exec.Command(llamaPath, args...)

	cmd.Env = append(cmd.Env, "GGML_CUDA_ENABLE_UNIFIED_MEMORY=1")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.generating.Store(false)
		log.Printf("[chat] Stdout error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if err := cmd.Start(); err != nil {
		s.generating.Store(false)
		log.Printf("[chat] Start error: %v", err)
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start inference"})
		return
	}

	go func() {
		cmd.Wait()
	}()

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "[end of text]") {
			break
		}

		content := s.extractTextFromLine(line)
		if content != "" {
			resp := ChatResponse{Content: content, Done: false}
			data, _ := json.Marshal(resp)
			fmt.Fprintf(w, "data: %s\n", data)
			flusher.Flush()
		}
	}

	done := ChatResponse{Content: "", Done: true}
	data, _ := json.Marshal(done)
	fmt.Fprintf(w, "data: %s\n", data)
	flusher.Flush()
}

func (s *Server) findLLamaServer() string {
	paths := []string{
		"llama-server",
		"llama-server.exe",
		"./llama-server",
		"./llama-server.exe",
		"../bin/llama-server",
		"../bin/llama-server.exe",
		"llama-cli",
		"llama-cli.exe",
	}

	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			return p
		}
		if _, err := exec.Command("cmd", "/c", "where", p).CombinedOutput(); err == nil {
			return p
		}
	}

	return ""
}

func (s *Server) startLLamaServerIfNeeded(llamaPath string) {
	s.llamaServerMu.Lock()
	defer s.llamaServerMu.Unlock()

	if s.llamaServer != nil && s.llamaServer.Process != nil {
		return
	}
}

func (s *Server) buildPrompt(message string, history []ChatMessage) string {
	var sb strings.Builder
	sb.WriteString("<|start_header_id|>system<|end_header_id|>\n\n")
	sb.WriteString("You are a helpful AI assistant. Answer concisely and accurately.<|eot_id|>\n\n")

	for _, msg := range history {
		role := "user"
		if msg.Role == "assistant" {
			role = "assistant"
		}
		sb.WriteString(fmt.Sprintf("<|start_header_id|>%s<|end_header_id|>\n\n", role))
		sb.WriteString(msg.Content + "<|eot_id|>\n\n")
	}

	sb.WriteString("<|start_header_id|>user<|end_header_id|>\n\n")
	sb.WriteString(message + "<|eot_id|>\n\n")
	sb.WriteString("<|start_header_id|>assistant<|end_header_id|>\n\n")

	return sb.String()
}

func (s *Server) extractTextFromLine(line string) string {
	if strings.Contains(line, "inf ") {
		parts := strings.SplitN(line, " inf ", 2)
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	}

	if strings.Contains(line, "[") && strings.Contains(line, "]") {
		return ""
	}

	if line == "" {
		return ""
	}

	return line
}

func (s *Server) saveBase64Image(dataURL string) (string, error) {
	idx := strings.Index(dataURL, ",")
	if idx == -1 {
		return "", fmt.Errorf("invalid data URL")
	}

	b64 := dataURL[idx+1:]
	decoded, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}

	tmpFile, err := os.CreateTemp("", "llama-img-*.png")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(decoded); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

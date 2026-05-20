package api

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/gorilla/websocket"
	"github.com/lamanchaland/llama-distributed/internal/discovery"
	"github.com/lamanchaland/llama-distributed/internal/llmserver"
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
	llmServer     *llmserver.Server
	staticFS      http.FileSystem
	mu            sync.RWMutex
	clients       map[*websocket.Conn]bool
	modelPath     string
	mmprojPath    string
	modelLoaded   bool
	rpcWorkers    []string
	generating    atomic.Bool
}

func NewServer(disc *discovery.Discovery, mon *monitor.Monitor, staticFS http.FileSystem, llm *llmserver.Server) *Server {
	return &Server{
		discovery:  disc,
		monitor:    mon,
		llmServer:  llm,
		staticFS:   staticFS,
		clients:    make(map[*websocket.Conn]bool),
		rpcWorkers: make([]string, 0),
	}
}

func (s *Server) SetModelPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelPath = path
	if s.llmServer != nil {
		s.llmServer.SetModelPath(path)
	}
}

func (s *Server) SetMMProjPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mmprojPath = path
	if s.llmServer != nil {
		s.llmServer.SetMMProjPath(path)
	}
}

func (s *Server) SetCtxSize(size int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.llmServer != nil {
		s.llmServer.SetCtxSize(size)
	}
}

func (s *Server) SetNGPU(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.llmServer != nil {
		s.llmServer.SetNGPU(n)
	}
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
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/", s.handleStatic)

	setupV1Routes(mux, s.llmServer, &s.modelPath)

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
		running := s.llmServer != nil && s.llmServer.IsRunning()
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"path":    s.modelPath,
			"mmproj":  s.mmprojPath,
			"loaded":  s.modelLoaded,
			"running": running,
			"workers": s.rpcWorkers,
		})

	case http.MethodPost:
		var req struct {
			Path      string `json:"path"`
			MMProj    string `json:"mmproj"`
			Action    string `json:"action"`
			CtxSize   int    `json:"ctx_size"`
			NGPULayers int   `json:"n_gpu_layers"`
			Threads   int    `json:"threads"`
			NPredict  int    `json:"n_predict"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}

		s.mu.Lock()
		s.modelPath = req.Path
		if req.MMProj != "" {
			s.mmprojPath = req.MMProj
		}
		s.mu.Unlock()

		if s.llmServer != nil {
			s.llmServer.SetModelPath(req.Path)
			s.llmServer.SetMMProjPath(req.MMProj)
			if req.CtxSize > 0 {
				s.llmServer.SetCtxSize(req.CtxSize)
			}
			if req.NGPULayers != 0 {
				s.llmServer.SetNGPU(req.NGPULayers)
			}
			if req.Threads > 0 {
				s.llmServer.SetThreads(req.Threads)
			}
			if req.NPredict > 0 {
				s.llmServer.SetNPredict(req.NPredict)
			}
		}

		if req.Action == "start" || req.Action == "load" {
			if s.llmServer != nil {
				go func() {
					s.mu.Lock()
					s.llmServer.SetRPCWorkers(s.rpcWorkers)
					s.mu.Unlock()
					if err := s.llmServer.Start(); err != nil {
						log.Printf("[model] Failed to start LLM server: %v", err)
					} else {
						s.mu.Lock()
						s.modelLoaded = true
						s.mu.Unlock()
						s.broadcastUpdate()
					}
				}()
			}
		}

		if req.Action == "stop" || req.Action == "unload" {
			if s.llmServer != nil {
				s.llmServer.Stop()
				s.mu.Lock()
				s.modelLoaded = false
				s.mu.Unlock()
				s.broadcastUpdate()
			}
		}

		respondJSON(w, http.StatusOK, map[string]string{"status": "ok", "path": req.Path})

	case http.MethodDelete:
		if s.llmServer != nil {
			s.llmServer.Stop()
		}
		s.mu.Lock()
		s.modelPath = ""
		s.mmprojPath = ""
		s.modelLoaded = false
		s.mu.Unlock()
		s.broadcastUpdate()
		respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

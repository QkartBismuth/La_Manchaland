package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"

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
	discovery  *discovery.Discovery
	monitor    *monitor.Monitor
	mu         sync.RWMutex
	clients    map[*websocket.Conn]bool
	modelPath  string
	modelLoaded bool
	rpcWorkers []string
}

func NewServer(disc *discovery.Discovery, mon *monitor.Monitor) *Server {
	return &Server{
		discovery: disc,
		monitor:   mon,
		clients:   make(map[*websocket.Conn]bool),
		rpcWorkers: make([]string, 0),
	}
}

func (s *Server) SetModelPath(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modelPath = path
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
			Path string `json:"path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}
		s.SetModelPath(req.Path)
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
	if r.URL.Path == "/" || r.URL.Path == "" {
		http.ServeFile(w, r, "web/ui/index.html")
		return
	}

	http.ServeFile(w, r, "web/ui"+r.URL.Path)
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

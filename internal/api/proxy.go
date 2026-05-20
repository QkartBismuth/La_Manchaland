package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/lamanchaland/llama-distributed/internal/llmserver"
)

func setupV1Routes(mux *http.ServeMux, server *llmserver.Server, modelPath *string) {
	mux.HandleFunc("/v1/models", handleModels(server, modelPath))
	mux.HandleFunc("/v1/chat/completions", handleChatCompletions(server))
	mux.HandleFunc("/v1/completions", handleCompletions(server))
	mux.HandleFunc("/v1/embeddings", handleEmbeddings(server))
	mux.HandleFunc("/v1/", handleV1Fallback(server))
}

func handleModels(server *llmserver.Server, modelPath *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			respondError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		models := llmserver.GetModels(*modelPath)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data":   models,
		})
	}
}

func handleChatCompletions(server *llmserver.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !server.IsRunning() {
			respondError(w, http.StatusServiceUnavailable, "model not loaded")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		defer r.Body.Close()

		var reqBody map[string]interface{}
		if err := json.Unmarshal(body, &reqBody); err != nil {
			respondError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		reqBody["stream"] = reqBody["stream"] != nil && reqBody["stream"] == true

		bodyBytes, _ := json.Marshal(reqBody)
		proxyReq, err := http.NewRequestWithContext(r.Context(), "POST", server.BackendURL()+r.URL.Path, strings.NewReader(string(bodyBytes)))
		if err != nil {
			respondError(w, http.StatusInternalServerError, "proxy error")
			return
		}

		proxyReq.Header.Set("Content-Type", "application/json")
		proxyReq.Header.Set("Accept", "application/json")

		client := &http.Client{}
		resp, err := client.Do(proxyReq)
		if err != nil {
			respondError(w, http.StatusBadGateway, "failed to connect to inference engine")
			return
		}
		defer resp.Body.Close()

		if reqBody["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(resp.StatusCode)
			io.Copy(w, resp.Body)
			return
		}

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

func handleCompletions(server *llmserver.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !server.IsRunning() {
			respondError(w, http.StatusServiceUnavailable, "model not loaded")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		defer r.Body.Close()

		proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, server.BackendURL()+r.URL.Path, strings.NewReader(string(body)))
		if err != nil {
			respondError(w, http.StatusInternalServerError, "proxy error")
			return
		}

		proxyReq.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(proxyReq)
		if err != nil {
			respondError(w, http.StatusBadGateway, "failed to connect to inference engine")
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

func handleEmbeddings(server *llmserver.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !server.IsRunning() {
			respondError(w, http.StatusServiceUnavailable, "model not loaded")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		defer r.Body.Close()

		proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, server.BackendURL()+r.URL.Path, strings.NewReader(string(body)))
		if err != nil {
			respondError(w, http.StatusInternalServerError, "proxy error")
			return
		}

		proxyReq.Header.Set("Content-Type", "application/json")

		client := &http.Client{}
		resp, err := client.Do(proxyReq)
		if err != nil {
			respondError(w, http.StatusBadGateway, "failed to connect to inference engine")
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}
}

func handleV1Fallback(server *llmserver.Server) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !server.IsRunning() {
			respondError(w, http.StatusServiceUnavailable, "model not loaded")
			return
		}

		director := func(req *http.Request) {
			req.URL.Scheme = "http"
			req.URL.Host = "127.0.0.1"
			req.URL.Path = strings.TrimPrefix(req.URL.Path, "/v1")
			req.Host = "127.0.0.1"
		}

		proxy := &httputil.ReverseProxy{
			Director: director,
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				log.Printf("[v1-proxy] Error: %v", err)
				respondError(w, http.StatusBadGateway, "proxy error")
			},
		}

		proxy.ServeHTTP(w, r)
	}
}

func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	type ErrorDetail struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
	}

	type ErrorResponse struct {
		Error ErrorDetail `json:"error"`
	}

	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Message: message,
			Type:    "invalid_request_error",
			Code:    status,
		},
	})
}

func GetOpenAICompatibleInfo() map[string]interface{} {
	return map[string]interface{}{
		"object": "list",
		"data": []map[string]interface{}{
			{
				"id":       "la-manchaland",
				"object":   "model",
				"created":  time.Now().Unix(),
				"owned_by": "la-manchaland",
			},
		},
	}
}

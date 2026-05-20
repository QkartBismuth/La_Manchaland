.PHONY: build build-controller build-worker clean run-controller run-worker help

BINARY_CONTROLLER=bin/la-manchaland-controller
BINARY_WORKER=bin/la-manchaland-worker
GO=go
GOFLAGS=-ldflags "-s -w"

help:
	@echo "=== La Manchaland - Distributed LLM Inference ==="
	@echo ""
	@echo "Targets:"
	@echo "  build          - Build controller and worker binaries"
	@echo "  build-controller - Build controller binary only"
	@echo "  build-worker   - Build worker binary only"
	@echo "  run-controller - Run controller with default settings"
	@echo "  run-worker     - Run worker with default settings"
	@echo "  clean          - Remove built binaries"
	@echo ""
	@echo "Variables:"
	@echo "  MODEL=path/to/model.gguf - Path to GGUF model file"
	@echo "  PORT=8080                - Controller port"
	@echo "  WORKER_PORT=50051        - Worker RPC port"
	@echo "  WORKER_NAME=my-worker    - Worker name"

build: build-controller build-worker
	@echo "Build complete."

build-controller:
	@mkdir -p bin
	$(GO) build $(GOFLAGS) -o $(BINARY_CONTROLLER) ./cmd/controller

build-worker:
	@mkdir -p bin
	$(GO) build $(GOFLAGS) -o $(BINARY_WORKER) ./cmd/worker

run-controller: build-controller
	@if [ -z "$(MODEL)" ]; then \
		echo "WARNING: No model specified. Use MODEL=path/to/model.gguf"; \
	fi
	./$(BINARY_CONTROLLER) \
		$(if $(MODEL),-model $(MODEL)) \
		$(if $(PORT),-port $(PORT))

run-worker: build-worker
	./$(BINARY_WORKER) \
		$(if $(WORKER_PORT),-port $(WORKER_PORT)) \
		$(if $(WORKER_NAME),-name $(WORKER_NAME)) \
		$(if $(CUDA_LAYERS),-cuda-layers $(CUDA_LAYERS))

clean:
	rm -rf bin

install-deps:
	$(GO) mod download
	$(GO) mod tidy

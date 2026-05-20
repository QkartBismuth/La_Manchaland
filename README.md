# La Manchaland - Distributed LLM Inference

A distributed system for running large language models across multiple devices in a local network. Combine GPU/CPU resources from multiple Windows/Linux machines to run models that exceed the capacity of a single device.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                     CONTROLLER                           │
│  ┌─────────────────┐  ┌───────────────────────────────┐  │
│  │   Web UI (8080) │  │  llama.cpp (model loading)    │  │
│  │   - Dashboard   │  │  - Layer offloading via RPC   │  │
│  │   - Graph view  │  │  - CUDA/CPU support           │  │
│  │   - Metrics     │  └───────────────┬───────────────┘  │
│  │   - Chat        │                  │                  │
│  │   - Settings    │                  │                  │
│  └────────┬────────┘                  │                  │
│           │ OpenAI-compatible API     │                  │
│  ┌────────┴───────────────────────────┴───────────────┐  │
│  │              mDNS Discovery + HTTP API + /v1/*       │  │
│  └────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘
            │ llama.cpp RPC (TCP)
     ┌──────┴──────┐         ┌──────────┐
     ▼             ▼         ▼          ▼
┌─────────┐  ┌─────────┐ ┌─────────┐ ┌─────────┐
│ Worker 1│  │ Worker 2│ │ Worker 3│ │ Worker N│
│  CUDA   │  │  CUDA   │ │   CPU   │ │  CUDA   │
│ Win/Linux│ │ Win/Linux│ │ Linux   │ │  Win    │
└─────────┘  └─────────┘ └─────────┘ └─────────┘
```

## Features

- **Distributed Inference** - Run models larger than a single device's VRAM by offloading layers across multiple GPUs
- **OpenAI-Compatible API** - Drop-in replacement for OpenAI API (`/v1/chat/completions`, `/v1/completions`, `/v1/embeddings`, `/v1/models`)
- **Auto Discovery** - Workers automatically find each other via mDNS (Zeroconf) on the local network
- **Manual Connection** - Add workers manually by specifying host and port
- **Real-time Monitoring** - Dashboard with live metrics: VRAM, RAM, GPU utilization, temperature
- **Network Topology Graph** - Visual representation of connected workers and their status
- **Cross-platform** - Supports Windows and Linux workers
- **CUDA Support** - Full NVIDIA GPU acceleration on all devices
- **CPU Fallback** - Works on CPU-only machines as well
- **Single Binary** - No runtime dependencies, everything compiled into one executable
- **Multimodal** - Vision model support via mmproj files and image attachments
- **Model Settings** - Configurable context size, GPU layers, temperature, top_p, top_k, repeat penalty
- **Reasoning Mode** - Toggle reasoning/chain-of-thought for complex queries

## Quick Start

### Prerequisites

- Go 1.23 or higher
- llama.cpp binaries (`llama-server`, `llama-rpc-server`)
- NVIDIA GPU with CUDA (optional, but recommended for workers)

### Building

```bash
# Clone the repository
git clone <repository-url>
cd La_Manchaland

# Download dependencies
go mod download

# Build both binaries
go build -ldflags "-s -w" -o bin/la-manchaland-controller.exe ./cmd/controller
go build -ldflags "-s -w" -o bin/la-manchaland-worker.exe ./cmd/worker
```

### Running the Controller

```bash
# With a model
./bin/la-manchaland-controller -model /path/to/model.gguf -port 8080

# With config file
./bin/la-manchaland-controller -config controller.json
```

Open your browser to `http://localhost:8080` to access the Web UI.

### Running a Worker

```bash
# Default settings
./bin/la-manchaland-worker

# With custom settings
./bin/la-manchaland-worker -port 50051 -name my-gpu-worker -cuda-layers -1

# With config file
./bin/la-manchaland-worker -config worker.json
```

### Configuration Files

**controller.json**
```json
{
  "controller": {
    "host": "0.0.0.0",
    "port": 8080,
    "model_path": "/path/to/model.gguf",
    "context_size": 4096,
    "threads": 0,
    "rpc_workers": ["192.168.1.100:50051"],
    "auto_discover": true
  }
}
```

**worker.json**
```json
{
  "host": "0.0.0.0",
  "port": 50051,
  "controller_ip": "",
  "name": "my-worker",
  "cuda_layers": -1
}
```

## How It Works

### 1. Worker Discovery

Workers broadcast their presence on the local network using mDNS (Zeroconf). The controller automatically discovers and displays them in the UI. Workers can also be added manually by specifying their IP and port.

### 2. Model Loading

The model file (GGUF format) is loaded on the controller using `llama-server`. The controller then distributes model layers across connected workers using llama.cpp's built-in RPC protocol.

### 3. Layer Offloading

When you run inference, the controller sends computation requests to workers. Each worker processes its assigned layers using its local GPU (CUDA) or CPU, then returns results back to the controller.

### 4. OpenAI-Compatible API

The controller exposes a fully OpenAI-compatible API on the same port (`/v1/*`). External applications (Open WebUI, Continue.dev, Cursor, LM Studio, etc.) can connect directly as if it were an OpenAI endpoint.

### 5. Real-time Monitoring

All connected workers report their metrics (VRAM, RAM, GPU utilization, temperature) to the controller. These are displayed in real-time on the dashboard via WebSocket connection.

## API Endpoints

### Management API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/workers` | List all discovered workers |
| POST | `/api/workers/manual` | Add a worker manually |
| DELETE | `/api/workers` | Remove a worker |
| GET | `/api/model` | Get model status |
| POST | `/api/model` | Set model path, load/unload |
| GET | `/api/metrics` | Get local system metrics |
| WS | `/ws` | WebSocket for real-time updates |

### OpenAI-Compatible API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/models` | List available models |
| POST | `/v1/chat/completions` | Chat completions (streaming + non-streaming) |
| POST | `/v1/completions` | Text completions |
| POST | `/v1/embeddings` | Embeddings |
| ANY | `/v1/*` | Reverse proxy to llama-server |

#### Example Usage

```bash
# List models
curl http://localhost:8080/v1/models

# Chat completion (streaming)
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "la-manchaland",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'

# Chat completion (non-streaming)
curl http://localhost:8080/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "la-manchaland",
    "messages": [{"role": "user", "content": "Hello!"}],
    "temperature": 0.7,
    "max_tokens": 512
  }'
```

## Web UI

The controller includes a built-in web interface with three tabs:

### Dashboard
- **Model Configuration** - Set model path, mmproj file, load/unload models
- **Model Settings** - Configure context size, GPU layers, threads, temperature, top_p, top_k, repeat penalty
- **Worker Cards** - Display each worker's status, VRAM, RAM, GPU info
- **System Metrics** - Real-time CPU, RAM, VRAM, GPU utilization bars

### Topology
- **Network Graph** - Visual graph showing controller and connected workers
- **Node Inspection** - Click nodes to view detailed device metrics

### Chat
- **OpenAI Chat** - Send messages via `/v1/chat/completions` endpoint
- **API Link** - Copy API URL for use with external applications
- **Reasoning Toggle** - Enable/disable chain-of-thought reasoning mode
- **Image Attachments** - Support for multimodal vision models
- **Streaming Responses** - Real-time token-by-token output

## llama.cpp Integration

This project uses llama.cpp for both distributed inference and local model serving.

### Required Binaries

| Binary | Used By | Purpose |
|--------|---------|---------|
| `llama-server` | Controller | Model serving + OpenAI API |
| `llama-rpc-server` | Workers | Distributed layer computation |

### Building llama.cpp

```bash
git clone https://github.com/ggml-org/llama.cpp
cd llama.cpp

# Build with CUDA support
cmake -B build -DGGML_CUDA=ON
cmake --build build --config Release

# Copy binaries
cp build/bin/llama-server /path/to/La_Manchaland/bin/
cp build/bin/llama-rpc-server /path/to/La_Manchaland/bin/
```

### Worker RPC Command

The worker binary will automatically start `llama-rpc-server` if found in the same directory:

```bash
./llama-rpc-server --host 0.0.0.0 --port 50051 --n-gpu-layers -1
```

## Project Structure

```
La_Manchaland/
├── cmd/
│   ├── controller/        # Controller binary entry point
│   └── worker/            # Worker binary entry point
├── internal/
│   ├── api/               # HTTP API, WebSocket, OpenAI proxy
│   │   ├── api.go         # Core API handlers
│   │   └── proxy.go       # OpenAI-compatible API proxy
│   ├── config/            # Configuration management
│   ├── discovery/         # mDNS discovery (Zeroconf)
│   ├── llmserver/         # llama-server process manager
│   └── monitor/           # System metrics (GPU/CPU/RAM)
├── static/                # Embedded static assets (go:embed)
├── webfs/
│   └── web/
│       └── ui/            # Web interface (HTML/CSS/JS)
│           ├── index.html
│           ├── css/style.css
│           └── js/
│               ├── app.js
│               └── topology.js
├── bin/                   # Built binaries + llama.cpp
├── go.mod
└── README.md
```

## Troubleshooting

### Workers not discovered

- Ensure controller and workers are on the same local network
- Check firewall settings - mDNS uses UDP port 5353
- Try manual worker addition via UI or config

### CUDA not detected

- Ensure NVIDIA drivers are installed and up to date
- Verify CUDA toolkit is installed
- Run `nvidia-smi` to confirm GPU is detected
- Build llama.cpp with `-DGGML_CUDA=ON`

### Model loading fails

- Verify the model file exists and is a valid GGUF file
- Check file permissions
- Ensure sufficient RAM/VRAM available
- Confirm `llama-server` is accessible (PATH or same directory)

### RPC connection fails

- Ensure `llama-rpc-server` is in the worker's PATH or same directory
- Check that the worker port is not blocked by firewall
- Verify worker is running and listening on the correct port

### External apps can't connect to API

- Verify controller is running and accessible on port 8080
- Ensure model is loaded (check `/api/model` or UI status)
- Test with: `curl http://localhost:8080/v1/models`

## License

MIT License - See LICENSE file for details.

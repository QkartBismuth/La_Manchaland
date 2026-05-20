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
│  │   - Config      │                  │                  │
│  └─────────────────┘                  │                  │
│  ┌────────────────────────────────────┴───────────────┐  │
│  │              mDNS Discovery + HTTP API              │  │
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
- **Auto Discovery** - Workers automatically find each other via mDNS (Zeroconf) on the local network
- **Manual Connection** - Add workers manually by specifying host and port
- **Real-time Monitoring** - Dashboard with live metrics: VRAM, RAM, GPU utilization, temperature
- **Network Topology Graph** - Visual representation of connected workers and their status
- **Cross-platform** - Supports Windows and Linux workers
- **CUDA Support** - Full NVIDIA GPU acceleration on all devices
- **CPU Fallback** - Works on CPU-only machines as well
- **Single Binary** - No runtime dependencies, everything compiled into one executable

## Quick Start

### Prerequisites

- Go 1.23 or higher
- llama.cpp binaries (for RPC server functionality)
- NVIDIA GPU with CUDA (optional, but recommended for workers)

### Building

```bash
# Clone the repository
git clone <repository-url>
cd La_Manchaland

# Download dependencies
make install-deps

# Build both binaries
make build
```

### Running the Controller

```bash
# With a model
make run-controller MODEL=/path/to/model.gguf PORT=8080

# Or directly
./bin/la-manchaland-controller -model /path/to/model.gguf -port 8080
```

Open your browser to `http://localhost:8080` to access the Web UI.

### Running a Worker

```bash
# Default settings
make run-worker

# With custom settings
make run-worker WORKER_PORT=50051 WORKER_NAME=my-gpu-worker CUDA_LAYERS=-1

# Or directly
./bin/la-manchaland-worker -port 50051 -name my-gpu-worker -cuda-layers -1
```

### Configuration Files

You can use JSON configuration files instead of command-line arguments:

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

Run with config:
```bash
./bin/la-manchaland-controller -config controller.json
./bin/la-manchaland-worker -config worker.json
```

## How It Works

### 1. Worker Discovery

Workers broadcast their presence on the local network using mDNS (Zeroconf). The controller automatically discovers and displays them in the UI. Workers can also be added manually by specifying their IP and port.

### 2. Model Loading

The model file (GGUF format) is loaded on the controller. The controller then distributes model layers across connected workers using llama.cpp's built-in RPC protocol.

### 3. Layer Offloading

When you run inference, the controller sends computation requests to workers. Each worker processes its assigned layers using its local GPU (CUDA) or CPU, then returns results back to the controller.

### 4. Real-time Monitoring

All connected workers report their metrics (VRAM, RAM, GPU utilization, temperature) to the controller. These are displayed in real-time on the dashboard via WebSocket connection.

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/workers` | List all discovered workers |
| POST | `/api/workers/manual` | Add a worker manually |
| DELETE | `/api/workers` | Remove a worker |
| GET | `/api/model` | Get model status |
| POST | `/api/model` | Set model path |
| GET | `/api/metrics` | Get local system metrics |
| WS | `/ws` | WebSocket for real-time updates |

## Web UI

The controller includes a built-in web interface:

- **Model Configuration** - Set model path and load/unload models
- **Worker Cards** - Display each worker's status, VRAM, RAM, GPU info
- **Network Topology** - Visual graph showing controller and connected workers
- **System Metrics** - Real-time CPU, RAM, VRAM, GPU utilization bars
- **Add Worker Modal** - Manually add workers by IP and port

## llama.cpp RPC Integration

This project uses llama.cpp's built-in RPC server for distributed inference. Workers need access to `llama-rpc-server` binary from llama.cpp.

### Building llama.cpp

```bash
git clone https://github.com/ggml-org/llama.cpp
cd llama.cpp

# Build with CUDA support
cmake -B build -DGGML_CUDA=ON
cmake --build build --config Release

# Copy the RPC server to your worker directory
cp build/bin/llama-rpc-server /path/to/worker/directory/
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
│   ├── api/               # HTTP API and WebSocket server
│   ├── config/            # Configuration management
│   ├── discovery/         # mDNS discovery (Zeroconf)
│   └── monitor/           # System metrics (GPU/CPU/RAM)
├── web/
│   └── ui/                # Web interface (HTML/CSS/JS)
│       ├── index.html
│       ├── css/style.css
│       └── js/
│           ├── app.js
│           └── topology.js
├── Makefile
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

### RPC connection fails

- Ensure `llama-rpc-server` is in the worker's PATH or same directory
- Check that the worker port is not blocked by firewall
- Verify worker is running and listening on the correct port

## License

MIT License - See LICENSE file for details.

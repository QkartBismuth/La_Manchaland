const App = {
    ws: null,
    topology: null,
    workers: [],
    reconnectAttempts: 0,
    maxReconnectAttempts: 10,

    init() {
        this.topology = new TopologyGraph(document.getElementById('topology-graph'));
        this.topology.resize();

        this.bindEvents();
        this.connectWebSocket();
        this.fetchInitialData();

        window.addEventListener('resize', () => this.topology.resize());
    },

    bindEvents() {
        document.getElementById('add-worker-btn').addEventListener('click', () => {
            document.getElementById('add-worker-modal').classList.add('active');
        });

        document.getElementById('cancel-add').addEventListener('click', () => {
            document.getElementById('add-worker-modal').classList.remove('active');
        });

        document.getElementById('add-worker-form').addEventListener('submit', (e) => {
            e.preventDefault();
            this.addWorker();
        });

        document.getElementById('load-model-btn').addEventListener('click', () => {
            this.loadModel();
        });

        document.getElementById('add-worker-modal').addEventListener('click', (e) => {
            if (e.target === document.getElementById('add-worker-modal')) {
                document.getElementById('add-worker-modal').classList.remove('active');
            }
        });
    },

    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        try {
            this.ws = new WebSocket(wsUrl);

            this.ws.onopen = () => {
                this.reconnectAttempts = 0;
                this.updateConnectionStatus(true);
                console.log('[ws] Connected');
            };

            this.ws.onmessage = (event) => {
                const data = JSON.parse(event.data);
                this.updateWorkers(data.workers || []);
                this.updateModelStatus(data.model || {});
            };

            this.ws.onclose = () => {
                this.updateConnectionStatus(false);
                this.scheduleReconnect();
            };

            this.ws.onerror = (err) => {
                console.error('[ws] Error:', err);
            };
        } catch (e) {
            this.scheduleReconnect();
        }
    },

    scheduleReconnect() {
        if (this.reconnectAttempts < this.maxReconnectAttempts) {
            this.reconnectAttempts++;
            setTimeout(() => this.connectWebSocket(), Math.min(1000 * Math.pow(2, this.reconnectAttempts), 10000));
        }
    },

    updateConnectionStatus(connected) {
        const status = document.getElementById('connection-status');
        if (connected) {
            status.textContent = 'Connected';
            status.className = 'status-indicator connected';
        } else {
            status.textContent = 'Disconnected';
            status.className = 'status-indicator disconnected';
        }
    },

    async fetchInitialData() {
        try {
            const [workersRes, modelRes, metricsRes] = await Promise.all([
                fetch('/api/workers'),
                fetch('/api/model'),
                fetch('/api/metrics'),
            ]);

            if (workersRes.ok) {
                const workers = await workersRes.json();
                this.updateWorkers(workers);
            }

            if (modelRes.ok) {
                const model = await modelRes.json();
                this.updateModelStatus(model);
            }

            if (metricsRes.ok) {
                const metrics = await metricsRes.json();
                this.updateMetrics(metrics);
            }
        } catch (e) {
            console.error('Failed to fetch initial data:', e);
        }
    },

    updateWorkers(workers) {
        this.workers = workers;
        this.renderWorkers();
        this.topology.update({ workers });
        this.topology.startAnimation();

        document.getElementById('worker-count').textContent = `${workers.length} worker${workers.length !== 1 ? 's' : ''}`;
    },

    renderWorkers() {
        const grid = document.getElementById('workers-grid');

        if (this.workers.length === 0) {
            grid.innerHTML = `
                <div class="empty-state">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
                        <rect x="2" y="3" width="20" height="14" rx="2"/>
                        <path d="M8 21h8M12 17v4"/>
                    </svg>
                    <p>No workers connected</p>
                    <span>Start a worker instance or add one manually</span>
                </div>
            `;
            return;
        }

        grid.innerHTML = this.workers.map((w, i) => `
            <div class="worker-card" data-index="${i}">
                <div class="worker-card-header">
                    <div>
                        <div class="worker-name">${this.escapeHtml(w.name || 'worker')}</div>
                        <div class="worker-address">${w.host}:${w.port}</div>
                    </div>
                    <span class="worker-status ${w.status || 'available'}">${w.status || 'available'}</span>
                </div>
                <div class="worker-stats">
                    <div class="stat-item">
                        <div class="stat-label">RAM</div>
                        <div class="stat-value">${w.ram_total ? this.formatBytes(w.ram_used || 0) + ' / ' + this.formatBytes(w.ram_total) : 'N/A'}</div>
                    </div>
                    <div class="stat-item">
                        <div class="stat-label">VRAM</div>
                        <div class="stat-value">${w.vram_total ? this.formatBytes(w.vram_used || 0) + ' / ' + this.formatBytes(w.vram_total) : 'N/A'}</div>
                    </div>
                    <div class="stat-item">
                        <div class="stat-label">GPU</div>
                        <div class="stat-value">${w.gpu_name || 'N/A'}</div>
                    </div>
                    <div class="stat-item">
                        <div class="stat-label">CPU</div>
                        <div class="stat-value">${w.cpu_load ? w.cpu_load.toFixed(1) + '%' : 'N/A'}</div>
                    </div>
                </div>
            </div>
        `).join('');
    },

    updateModelStatus(model) {
        const state = document.getElementById('model-state');
        const pathInput = document.getElementById('model-path');

        if (model.path) {
            pathInput.value = model.path;
        }

        if (model.loaded) {
            state.textContent = 'Loaded';
            state.className = 'loaded';
        } else {
            state.textContent = 'Not loaded';
            state.className = 'not-loaded';
        }
    },

    updateMetrics(metrics) {
        document.getElementById('cpu-load').textContent = metrics.cpu_load ? metrics.cpu_load.toFixed(1) + '%' : '0%';
        document.getElementById('cpu-bar').style.width = (metrics.cpu_load || 0) + '%';

        const ramUsed = metrics.memory_used || 0;
        const ramTotal = metrics.memory_total || 0;
        document.getElementById('ram-usage').textContent = `${this.formatBytes(ramUsed)} / ${this.formatBytes(ramTotal)}`;
        document.getElementById('ram-bar').style.width = ramTotal > 0 ? ((ramUsed / ramTotal) * 100) + '%' : '0%';

        if (metrics.gpus && metrics.gpus.length > 0) {
            const gpu = metrics.gpus[0];
            document.getElementById('gpu-metric-card').style.display = 'block';
            document.getElementById('gpu-util-card').style.display = 'block';

            document.getElementById('vram-usage').textContent = `${this.formatBytes(gpu.memory_used || 0)} / ${this.formatBytes(gpu.memory_total || 0)}`;
            document.getElementById('vram-bar').style.width = gpu.memory_total > 0 ? ((gpu.memory_used / gpu.memory_total) * 100) + '%' : '0%';

            document.getElementById('gpu-util').textContent = (gpu.utilization || 0) + '%';
            document.getElementById('gpu-bar').style.width = (gpu.utilization || 0) + '%';
        }
    },

    async addWorker() {
        const host = document.getElementById('worker-host').value.trim();
        const port = parseInt(document.getElementById('worker-port').value, 10);
        const name = document.getElementById('worker-name').value.trim();

        try {
            const res = await fetch('/api/workers/manual', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ host, port, name }),
            });

            if (res.ok) {
                document.getElementById('add-worker-modal').classList.remove('active');
                document.getElementById('add-worker-form').reset();
                this.fetchInitialData();
            } else {
                alert('Failed to add worker');
            }
        } catch (e) {
            alert('Error adding worker: ' + e.message);
        }
    },

    async loadModel() {
        const path = document.getElementById('model-path').value.trim();
        if (!path) {
            alert('Please enter a model path');
            return;
        }

        try {
            const res = await fetch('/api/model', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ path }),
            });

            if (res.ok) {
                this.fetchInitialData();
            } else {
                alert('Failed to load model');
            }
        } catch (e) {
            alert('Error loading model: ' + e.message);
        }
    },

    formatBytes(bytes) {
        if (!bytes || bytes === 0) return '0 B';
        const units = ['B', 'KB', 'MB', 'GB', 'TB'];
        const i = Math.floor(Math.log(bytes) / Math.log(1024));
        return (bytes / Math.pow(1024, i)).toFixed(1) + ' ' + units[i];
    },

    escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    },
};

document.addEventListener('DOMContentLoaded', () => App.init());

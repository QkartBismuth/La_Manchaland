const App = {
    ws: null,
    topology: null,
    workers: [],
    localMetrics: null,
    reconnectAttempts: 0,
    maxReconnectAttempts: 10,
    chatHistory: [],
    isGenerating: false,
    conversationId: '',
    mmprojPath: '',
    attachedImage: null,
    attachedImageBase64: '',

    init() {
        this.topology = new TopologyGraph(document.getElementById('topology-graph'), (node) => this.showDeviceDetail(node));
        this.topology.resize();

        this.bindEvents();
        this.connectWebSocket();
        this.fetchInitialData();

        window.addEventListener('resize', () => this.topology.resize());
    },

    bindEvents() {
        document.querySelectorAll('.tab-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                const tab = btn.dataset.tab;
                this.switchTab(tab);
            });
        });

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

        document.getElementById('browse-model-btn').addEventListener('click', () => {
            document.getElementById('model-file-input').click();
        });

        document.getElementById('model-file-input').addEventListener('change', (e) => {
            if (e.target.files.length > 0) {
                const file = e.target.files[0];
                document.getElementById('model-path').value = file.path || file.name;
            }
        });

        document.getElementById('browse-mmproj-btn').addEventListener('click', () => {
            document.getElementById('mmproj-file-input').click();
        });

        document.getElementById('mmproj-file-input').addEventListener('change', (e) => {
            if (e.target.files.length > 0) {
                const file = e.target.files[0];
                this.mmprojPath = file.path || file.name;
                document.getElementById('mmproj-path').value = this.mmprojPath;
            }
        });

        document.getElementById('attach-image-btn').addEventListener('click', () => {
            document.getElementById('chat-image-input').click();
        });

        document.getElementById('chat-image-input').addEventListener('change', (e) => {
            if (e.target.files.length > 0) {
                this.loadImagePreview(e.target.files[0]);
            }
        });

        document.getElementById('remove-image-btn').addEventListener('click', () => {
            this.attachedImage = null;
            this.attachedImageBase64 = '';
            document.getElementById('image-preview-container').style.display = 'none';
            document.getElementById('chat-image-input').value = '';
        });

        document.getElementById('load-model-btn').addEventListener('click', () => {
            this.loadModel();
        });

        document.getElementById('add-worker-modal').addEventListener('click', (e) => {
            if (e.target === document.getElementById('add-worker-modal')) {
                document.getElementById('add-worker-modal').classList.remove('active');
            }
        });

        document.getElementById('close-device-detail').addEventListener('click', () => {
            document.getElementById('device-detail-modal').classList.remove('active');
        });

        document.getElementById('device-detail-modal').addEventListener('click', (e) => {
            if (e.target === document.getElementById('device-detail-modal')) {
                document.getElementById('device-detail-modal').classList.remove('active');
            }
        });

        document.getElementById('chat-send-btn').addEventListener('click', () => this.sendChatMessage());

        document.getElementById('chat-input').addEventListener('keydown', (e) => {
            if (e.key === 'Enter' && !e.shiftKey) {
                e.preventDefault();
                this.sendChatMessage();
            }
        });

        document.getElementById('chat-input').addEventListener('input', function() {
            this.style.height = 'auto';
            this.style.height = Math.min(this.scrollHeight, 150) + 'px';
        });
    },

    switchTab(tab) {
        document.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
        document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

        document.querySelector(`.tab-btn[data-tab="${tab}"]`).classList.add('active');
        document.getElementById(`tab-${tab}`).classList.add('active');

        if (tab === 'topology') {
            setTimeout(() => {
                this.topology.resize();
                this.topology.startAnimation();
            }, 50);
        }
    },

    connectWebSocket() {
        const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = `${protocol}//${window.location.host}/ws`;

        try {
            this.ws = new WebSocket(wsUrl);

            this.ws.onopen = () => {
                this.reconnectAttempts = 0;
                this.updateConnectionStatus(true);
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

            this.ws.onerror = () => {
                this.scheduleReconnect();
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
                this.localMetrics = metrics;
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
            <div class="worker-card">
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
            alert('Please select a model file');
            return;
        }

        const btn = document.getElementById('load-model-btn');
        btn.disabled = true;
        btn.textContent = 'Loading...';

        try {
            const res = await fetch('/api/model', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    path: path,
                    mmproj: this.mmprojPath,
                }),
            });

            if (res.ok) {
                this.fetchInitialData();
            } else {
                alert('Failed to load model');
            }
        } catch (e) {
            alert('Error loading model: ' + e.message);
        } finally {
            btn.disabled = false;
            btn.textContent = 'Load Model';
        }
    },

    showDeviceDetail(node) {
        const modal = document.getElementById('device-detail-modal');
        const icon = document.getElementById('device-icon');
        const name = document.getElementById('device-name');
        const address = document.getElementById('device-address');
        const status = document.getElementById('device-status-badge');

        name.textContent = node.label;

        if (node.type === 'controller') {
            icon.textContent = 'C';
            icon.className = 'device-icon';
            address.textContent = 'localhost';
            status.textContent = 'active';
            status.className = 'device-status-badge';

            if (this.localMetrics) {
                document.getElementById('dm-cpu').textContent = this.localMetrics.cpu_load ? this.localMetrics.cpu_load.toFixed(1) + '%' : '-';
                document.getElementById('dm-ram').textContent = this.localMetrics.memory_total ? this.formatBytes(this.localMetrics.memory_total) : '-';
                document.getElementById('dm-ram-used').textContent = this.localMetrics.memory_used ? this.formatBytes(this.localMetrics.memory_used) : '-';
                document.getElementById('dm-gpu').textContent = (this.localMetrics.gpus && this.localMetrics.gpus[0]) ? this.localMetrics.gpus[0].name : '-';
                document.getElementById('dm-vram').textContent = (this.localMetrics.gpus && this.localMetrics.gpus[0]) ? this.formatBytes(this.localMetrics.gpus[0].memory_total) : '-';
                document.getElementById('dm-vram-used').textContent = (this.localMetrics.gpus && this.localMetrics.gpus[0]) ? this.formatBytes(this.localMetrics.gpus[0].memory_used) : '-';
                document.getElementById('dm-gpu-temp').textContent = (this.localMetrics.gpus && this.localMetrics.gpus[0]) ? this.localMetrics.gpus[0].temperature + '°C' : '-';
                document.getElementById('dm-gpu-util').textContent = (this.localMetrics.gpus && this.localMetrics.gpus[0]) ? this.localMetrics.gpus[0].utilization + '%' : '-';
            }
        } else {
            icon.textContent = 'W';
            icon.className = 'device-icon worker';
            address.textContent = node.data ? `${node.data.host}:${node.data.port}` : '-';
            status.textContent = node.status || 'available';
            status.className = `device-status-badge ${node.status || 'available'}`;

            const d = node.data || {};
            document.getElementById('dm-cpu').textContent = d.cpu_load ? d.cpu_load.toFixed(1) + '%' : '-';
            document.getElementById('dm-ram').textContent = d.ram_total ? this.formatBytes(d.ram_total) : '-';
            document.getElementById('dm-ram-used').textContent = d.ram_used ? this.formatBytes(d.ram_used) : '-';
            document.getElementById('dm-gpu').textContent = d.gpu_name || '-';
            document.getElementById('dm-vram').textContent = d.vram_total ? this.formatBytes(d.vram_total) : '-';
            document.getElementById('dm-vram-used').textContent = d.vram_used ? this.formatBytes(d.vram_used) : '-';
            document.getElementById('dm-gpu-temp').textContent = '-';
            document.getElementById('dm-gpu-util').textContent = '-';
        }

        document.getElementById('dm-last-seen').textContent = node.data && node.data.last_seen
            ? new Date(node.data.last_seen).toLocaleString()
            : 'Now';

        modal.classList.add('active');
    },

    loadImagePreview(file) {
        this.attachedImage = file;
        const reader = new FileReader();
        reader.onload = (e) => {
            this.attachedImageBase64 = e.target.result;
            document.getElementById('image-preview').src = this.attachedImageBase64;
            document.getElementById('image-preview-container').style.display = 'flex';
        };
        reader.readAsDataURL(file);
    },

    async sendChatMessage() {
        const input = document.getElementById('chat-input');
        const message = input.value.trim();
        if (!message && !this.attachedImageBase64) return;
        if (this.isGenerating) return;

        const imageBase64 = this.attachedImageBase64;
        this.attachedImage = null;
        this.attachedImageBase64 = '';
        document.getElementById('image-preview-container').style.display = 'none';
        document.getElementById('chat-image-input').value = '';

        input.value = '';
        input.style.height = 'auto';
        this.isGenerating = true;

        this.appendMessage('user', message, imageBase64);

        const assistantMsg = this.appendMessage('assistant', '');
        const contentEl = assistantMsg.querySelector('.message-content p');
        const cursorEl = document.createElement('span');
        cursorEl.className = 'cursor';
        contentEl.appendChild(cursorEl);

        try {
            const response = await fetch('/api/chat', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    message: message,
                    history: this.chatHistory,
                    stream: true,
                    image: imageBase64 || null,
                }),
            });

            if (!response.ok) {
                cursorEl.remove();
                contentEl.textContent = 'Error: Failed to get response. Is the model loaded?';
                this.isGenerating = false;
                return;
            }

            const reader = response.body.getReader();
            const decoder = new TextDecoder();
            let fullText = '';

            while (true) {
                const { done, value } = await reader.read();
                if (done) break;

                const chunk = decoder.decode(value, { stream: true });
                const lines = chunk.split('\n').filter(l => l.startsWith('data: '));

                for (const line of lines) {
                    const data = JSON.parse(line.substring(6));
                    if (data.content) {
                        fullText += data.content;
                        contentEl.textContent = fullText;
                        contentEl.appendChild(cursorEl);

                        const messagesEl = document.getElementById('chat-messages');
                        messagesEl.scrollTop = messagesEl.scrollHeight;
                    }
                    if (data.done) {
                        break;
                    }
                }
            }

            cursorEl.remove();
            this.chatHistory.push({ role: 'user', content: message });
            this.chatHistory.push({ role: 'assistant', content: fullText });

        } catch (e) {
            if (cursorEl.parentNode) cursorEl.remove();
            contentEl.textContent = 'Error: ' + e.message;
        }

        this.isGenerating = false;
    },

    appendMessage(role, content, imageBase64) {
        const container = document.getElementById('chat-messages');

        const emptyState = container.querySelector('.chat-empty-state');
        if (emptyState) emptyState.remove();

        const msg = document.createElement('div');
        msg.className = `message ${role}`;

        const avatar = document.createElement('div');
        avatar.className = 'message-avatar';

        if (role === 'user') {
            avatar.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>';
        } else if (role === 'assistant') {
            avatar.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83"/></svg>';
        } else {
            avatar.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"/><path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83"/></svg>';
        }

        const contentEl = document.createElement('div');
        contentEl.className = 'message-content';

        if (imageBase64) {
            const img = document.createElement('img');
            img.src = imageBase64;
            img.className = 'message-image';
            contentEl.appendChild(img);
        }

        const p = document.createElement('p');
        p.textContent = content;
        contentEl.appendChild(p);

        msg.appendChild(avatar);
        msg.appendChild(contentEl);
        container.appendChild(msg);

        container.scrollTop = container.scrollHeight;
        return msg;
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

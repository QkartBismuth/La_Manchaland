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
    reasoningEnabled: false,
    modelSettings: {
        ctxSize: 4096,
        nGpuLayers: -1,
        threads: 0,
        nPredict: 512,
        temperature: 0.7,
        topP: 0.9,
        topK: 40,
        repeatPenalty: 1.1,
    },

    init() {
        this.topology = new TopologyGraph(document.getElementById('topology-graph'), (node) => this.showDeviceDetail(node));
        this.topology.resize();

        this.bindEvents();
        this.connectWebSocket();
        this.fetchInitialData();

        window.addEventListener('resize', () => this.topology.resize());
    },

    bindEvents() {
        document.querySelectorAll('.m3-tab').forEach(btn => {
            btn.addEventListener('click', () => {
                this.switchTab(btn.dataset.tab);
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
                document.getElementById('mmproj-section').style.display = 'flex';
            }
        });

        document.getElementById('load-model-btn').addEventListener('click', () => {
            this.loadModel();
        });

        document.getElementById('unload-model-btn').addEventListener('click', () => {
            this.unloadModel();
        });

        document.getElementById('model-settings-btn').addEventListener('click', () => {
            this.openModelSettings();
        });

        document.getElementById('cancel-settings').addEventListener('click', () => {
            document.getElementById('model-settings-modal').classList.remove('active');
        });

        document.getElementById('save-settings').addEventListener('click', () => {
            this.saveModelSettings();
        });

        document.getElementById('reasoning-toggle').addEventListener('click', () => {
            this.toggleReasoning();
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

        document.getElementById('close-device-detail').addEventListener('click', () => {
            document.getElementById('device-detail-modal').classList.remove('active');
        });

        document.querySelectorAll('.m3-dialog-backdrop').forEach(backdrop => {
            backdrop.addEventListener('click', (e) => {
                if (e.target === backdrop) {
                    backdrop.classList.remove('active');
                }
            });
        });
    },

    switchTab(tab) {
        document.querySelectorAll('.m3-tab').forEach(b => b.classList.remove('active'));
        document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));

        document.querySelector(`.m3-tab[data-tab="${tab}"]`).classList.add('active');
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
        const el = document.getElementById('connection-status');
        if (connected) {
            el.className = 'status-dot connected';
            el.title = 'Connected';
        } else {
            el.className = 'status-dot disconnected';
            el.title = 'Disconnected';
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
                this.updateWorkers(await workersRes.json());
            }

            if (modelRes.ok) {
                this.updateModelStatus(await modelRes.json());
            }

            if (metricsRes.ok) {
                this.localMetrics = await metricsRes.json();
                this.updateMetrics(this.localMetrics);
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
        document.getElementById('worker-count-text').textContent = workers.length;
    },

    renderWorkers() {
        const grid = document.getElementById('workers-grid');

        if (this.workers.length === 0) {
            grid.innerHTML = `
                <div class="empty-state">
                    <span class="material-symbols-rounded empty-icon">devices_off</span>
                    <p class="empty-title">No workers connected</p>
                    <span class="empty-desc">Start a worker instance or add one manually</span>
                </div>
            `;
            return;
        }

        grid.innerHTML = this.workers.map(w => `
            <div class="worker-card">
                <div class="worker-card-header">
                    <div class="worker-card-title">
                        <div class="worker-icon-sm">W</div>
                        <div>
                            <div class="worker-name">${this.escapeHtml(w.name || 'worker')}</div>
                            <div class="worker-address">${w.host}:${w.port}</div>
                        </div>
                    </div>
                    <span class="m3-chip ${w.status || 'available'}">${w.status || 'available'}</span>
                </div>
                <div class="worker-stats">
                    <div class="worker-stat">
                        <span class="worker-stat-label">RAM</span>
                        <span class="worker-stat-value">${w.ram_total ? this.formatBytes(w.ram_used || 0) + ' / ' + this.formatBytes(w.ram_total) : 'N/A'}</span>
                    </div>
                    <div class="worker-stat">
                        <span class="worker-stat-label">VRAM</span>
                        <span class="worker-stat-value">${w.vram_total ? this.formatBytes(w.vram_used || 0) + ' / ' + this.formatBytes(w.vram_total) : 'N/A'}</span>
                    </div>
                    <div class="worker-stat">
                        <span class="worker-stat-label">GPU</span>
                        <span class="worker-stat-value">${w.gpu_name || 'N/A'}</span>
                    </div>
                    <div class="worker-stat">
                        <span class="worker-stat-label">CPU</span>
                        <span class="worker-stat-value">${w.cpu_load ? w.cpu_load.toFixed(1) + '%' : 'N/A'}</span>
                    </div>
                </div>
            </div>
        `).join('');
    },

    updateModelStatus(model) {
        const state = document.getElementById('model-state');
        const pathInput = document.getElementById('model-path');
        const loadBtn = document.getElementById('load-model-btn');
        const unloadBtn = document.getElementById('unload-model-btn');

        if (model.path) {
            pathInput.value = model.path;
        }

        if (model.running || model.loaded) {
            state.textContent = 'Running';
            state.className = 'model-state-text loaded';
            loadBtn.style.display = 'none';
            unloadBtn.style.display = 'inline-flex';
        } else if (model.path) {
            state.textContent = 'Loaded';
            state.className = 'model-state-text';
            loadBtn.style.display = 'inline-flex';
            unloadBtn.style.display = 'none';
        } else {
            state.textContent = 'Not loaded';
            state.className = 'model-state-text';
            loadBtn.style.display = 'inline-flex';
            unloadBtn.style.display = 'none';
        }

        if (model.mmproj) {
            this.mmprojPath = model.mmproj;
            const mmprojInput = document.getElementById('mmproj-path');
            if (mmprojInput) mmprojInput.value = model.mmproj;
            document.getElementById('mmproj-section').style.display = 'flex';
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
            document.getElementById('gpu-metric-card').style.display = 'flex';
            document.getElementById('gpu-util-card').style.display = 'flex';

            document.getElementById('vram-usage').textContent = `${this.formatBytes(gpu.memory_used || 0)} / ${this.formatBytes(gpu.memory_total || 0)}`;
            document.getElementById('vram-bar').style.width = gpu.memory_total > 0 ? ((gpu.memory_used / gpu.memory_total) * 100) + '%' : '0%';

            document.getElementById('gpu-util').textContent = (gpu.utilization || 0) + '%';
            document.getElementById('gpu-bar').style.width = (gpu.utilization || 0) + '%';
        }
    },

    copyApiUrl() {
        const baseUrl = `${window.location.protocol}//${window.location.host}/v1/chat/completions`;
        navigator.clipboard.writeText(baseUrl).then(() => {
            const icon = document.getElementById('chat-api-copy-icon');
            icon.textContent = 'check';
            setTimeout(() => { icon.textContent = 'content_copy'; }, 1500);
        });
    },

    openModelSettings() {
        document.getElementById('setting-ctx-size').value = this.modelSettings.ctxSize;
        document.getElementById('setting-n-gpu-layers').value = this.modelSettings.nGpuLayers;
        document.getElementById('setting-threads').value = this.modelSettings.threads;
        document.getElementById('setting-n-predict').value = this.modelSettings.nPredict;
        document.getElementById('setting-temp').value = this.modelSettings.temperature;
        document.getElementById('setting-top-p').value = this.modelSettings.topP;
        document.getElementById('setting-top-k').value = this.modelSettings.topK;
        document.getElementById('setting-repeat-penalty').value = this.modelSettings.repeatPenalty;
        document.getElementById('model-settings-modal').classList.add('active');
    },

    saveModelSettings() {
        this.modelSettings.ctxSize = parseInt(document.getElementById('setting-ctx-size').value) || 4096;
        this.modelSettings.nGpuLayers = parseInt(document.getElementById('setting-n-gpu-layers').value) ?? -1;
        this.modelSettings.threads = parseInt(document.getElementById('setting-threads').value) || 0;
        this.modelSettings.nPredict = parseInt(document.getElementById('setting-n-predict').value) || 512;
        this.modelSettings.temperature = parseFloat(document.getElementById('setting-temp').value) ?? 0.7;
        this.modelSettings.topP = parseFloat(document.getElementById('setting-top-p').value) ?? 0.9;
        this.modelSettings.topK = parseInt(document.getElementById('setting-top-k').value) || 40;
        this.modelSettings.repeatPenalty = parseFloat(document.getElementById('setting-repeat-penalty').value) ?? 1.1;
        document.getElementById('model-settings-modal').classList.remove('active');
    },

    toggleReasoning() {
        this.reasoningEnabled = !this.reasoningEnabled;
        const icon = document.getElementById('reasoning-icon');
        const status = document.getElementById('reasoning-status');
        const btn = document.getElementById('reasoning-toggle');
        if (this.reasoningEnabled) {
            icon.textContent = 'lightbulb';
            status.textContent = 'ON';
            btn.classList.add('active');
        } else {
            icon.textContent = 'lightbulb_outline';
            status.textContent = 'OFF';
            btn.classList.remove('active');
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
        btn.innerHTML = '<span class="material-symbols-rounded">hourglass_top</span> Loading...';

        try {
            const s = this.modelSettings;
            const res = await fetch('/api/model', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    path,
                    mmproj: this.mmprojPath,
                    action: 'load',
                    ctx_size: s.ctxSize,
                    n_gpu_layers: s.nGpuLayers,
                    threads: s.threads,
                    n_predict: s.nPredict,
                }),
            });

            if (res.ok) {
                this.fetchInitialData();
            } else {
                const err = await res.json().catch(() => null);
                alert('Failed to load model: ' + (err?.error?.message || 'unknown error'));
            }
        } catch (e) {
            alert('Error loading model: ' + e.message);
        } finally {
            btn.disabled = false;
            btn.innerHTML = '<span class="material-symbols-rounded">play_arrow</span> Load';
        }
    },

    async unloadModel() {
        try {
            const res = await fetch('/api/model', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ action: 'unload' }),
            });

            if (res.ok) {
                this.fetchInitialData();
            } else {
                alert('Failed to unload model');
            }
        } catch (e) {
            alert('Error unloading model: ' + e.message);
        }
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

        const messages = [];
        for (const msg of this.chatHistory) {
            if (msg.role === 'user' && msg.image_url) {
                messages.push({
                    role: 'user',
                    content: [
                        { type: 'image_url', image_url: { url: msg.image_url } },
                        { type: 'text', text: msg.content || '' }
                    ]
                });
            } else {
                messages.push({ role: msg.role, content: msg.content || '' });
            }
        }

        const userMsgContent = [];
        if (imageBase64) {
            userMsgContent.push({ type: 'image_url', image_url: { url: imageBase64 } });
        }
        if (message) {
            userMsgContent.push({ type: 'text', text: message });
        }

        messages.push({
            role: 'user',
            content: userMsgContent.length === 1 && userMsgContent[0].type === 'text'
                ? message
                : userMsgContent
        });

        this.appendMessage('user', message, imageBase64);

        const assistantMsg = this.appendMessage('assistant', '');
        const contentEl = assistantMsg.querySelector('.message-content p');
        const cursorEl = document.createElement('span');
        cursorEl.className = 'cursor-blink';
        contentEl.appendChild(cursorEl);

        try {
            const body = {
                model: 'la-manchaland',
                messages: messages,
                stream: true,
            };

            const s = this.modelSettings;
            if (s.temperature !== 0.7) body.temperature = s.temperature;
            if (s.topP !== 0.9) body.top_p = s.topP;
            if (s.topK !== 40) body.top_k = s.topK;
            if (s.repeatPenalty !== 1.1) body.repeat_penalty = s.repeatPenalty;
            if (s.nPredict !== 512) body.max_tokens = s.nPredict;

            if (this.reasoningEnabled) {
                const reasoningSystem = {
                    role: 'system',
                    content: 'Think step by step. Show your reasoning process before giving the final answer. Use <think> tags to structure your thinking.'
                };
                body.messages.unshift(reasoningSystem);
            }

            const response = await fetch('/v1/chat/completions', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(body),
            });

            if (!response.ok) {
                const errData = await response.json().catch(() => null);
                cursorEl.remove();
                contentEl.textContent = 'Error: ' + (errData?.error?.message || 'Failed to get response');
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
                const lines = chunk.split('\n');

                for (const line of lines) {
                    const trimmed = line.trim();
                    if (!trimmed.startsWith('data:')) continue;
                    const dataStr = trimmed.substring(5).trim();
                    if (dataStr === '[DONE]') break;
                    if (!dataStr) continue;

                    try {
                        const data = JSON.parse(dataStr);
                        const delta = data.choices?.[0]?.delta;
                        if (delta?.reasoning_content) {
                            if (!contentEl.querySelector('.reasoning-block')) {
                                const reasonBlock = document.createElement('div');
                                reasonBlock.className = 'reasoning-block';
                                reasonBlock.innerHTML = '<div class="reasoning-header"><span class="material-symbols-rounded" style="font-size:14px">psychology</span> Reasoning</div><div class="reasoning-text"></div>';
                                contentEl.insertBefore(reasonBlock, contentEl.querySelector('p'));
                            }
                            const reasonText = contentEl.querySelector('.reasoning-text');
                            if (reasonText) reasonText.textContent += delta.reasoning_content;
                        }
                        if (delta?.content) {
                            fullText += delta.content;
                            contentEl.querySelector('p').textContent = fullText;
                            contentEl.appendChild(cursorEl);

                            const messagesEl = document.getElementById('chat-messages');
                            messagesEl.scrollTop = messagesEl.scrollHeight;
                        }
                    } catch (e) {
                        // skip parse errors
                    }
                }
            }

            cursorEl.remove();
            this.chatHistory.push({ role: 'user', content: message, image_url: imageBase64 || null });
            this.chatHistory.push({ role: 'assistant', content: fullText });

        } catch (e) {
            if (cursorEl.parentNode) cursorEl.remove();
            contentEl.textContent = 'Error: ' + e.message;
        }

        this.isGenerating = false;
    },

    appendMessage(role, content, imageBase64) {
        const container = document.getElementById('chat-messages');
        const emptyState = container.querySelector('.message.system');
        if (emptyState && role !== 'system') emptyState.remove();

        const msg = document.createElement('div');
        msg.className = `message ${role}`;

        const contentEl = document.createElement('div');
        contentEl.className = 'message-content';

        if (imageBase64) {
            const img = document.createElement('img');
            img.src = imageBase64;
            img.className = 'message-image';
            contentEl.appendChild(img);
        }

        if (this.reasoningEnabled && role === 'assistant') {
            contentEl.innerHTML = '<div class="reasoning-block"><div class="reasoning-header"><span class="material-symbols-rounded" style="font-size:14px">psychology</span> Reasoning</div><div class="reasoning-text">Thinking...</div></div>';
        }

        const p = document.createElement('p');
        p.textContent = content;
        contentEl.appendChild(p);

        msg.appendChild(contentEl);
        container.appendChild(msg);
        container.scrollTop = container.scrollHeight;
        return msg;
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
            icon.className = 'device-icon-large';
            address.textContent = 'localhost';
            status.textContent = 'active';
            status.className = 'device-status-badge m3-chip available';

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
            icon.className = 'device-icon-large worker-icon';
            address.textContent = node.data ? `${node.data.host}:${node.data.port}` : '-';
            status.textContent = node.status || 'available';
            status.className = `device-status-badge m3-chip ${node.status || 'available'}`;

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

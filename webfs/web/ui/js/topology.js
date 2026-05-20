class TopologyGraph {
    constructor(canvas, onClick) {
        this.canvas = canvas;
        this.ctx = canvas.getContext('2d');
        this.nodes = [];
        this.edges = [];
        this.animationId = null;
        this.centerX = 0;
        this.centerY = 0;
        this.onClick = onClick;
        this.hoveredNode = null;
        this.nodePositions = {};

        canvas.addEventListener('mousemove', (e) => this.onMouseMove(e));
        canvas.addEventListener('mouseleave', () => {
            this.hoveredNode = null;
        });
        canvas.addEventListener('click', (e) => this.onClickHandler(e));
    }

    resize() {
        const rect = this.canvas.parentElement.getBoundingClientRect();
        this.canvas.width = rect.width - 48;
        this.canvas.height = 500;
        this.centerX = this.canvas.width / 2;
        this.centerY = this.canvas.height / 2;
    }

    update(data) {
        this.nodes = [];
        this.edges = [];

        this.nodes.push({
            id: 'controller',
            label: 'Controller',
            type: 'controller',
            status: 'active',
            radius: 40,
            data: null,
        });

        const workers = data.workers || [];
        const angleStep = (2 * Math.PI) / Math.max(workers.length, 1);
        const radius = Math.min(this.canvas.width, this.canvas.height) * 0.35;

        workers.forEach((worker, i) => {
            const angle = i * angleStep - Math.PI / 2;
            const id = worker.name || worker.host || `worker-${i}`;
            this.nodes.push({
                id: id,
                label: worker.name || worker.host || `Worker ${i + 1}`,
                type: 'worker',
                status: worker.status || 'available',
                radius: 30,
                data: worker,
            });

            this.edges.push({
                from: 'controller',
                to: id,
                status: worker.status === 'busy' ? 'busy' : 'active',
            });
        });

        this.layoutNodes();
    }

    layoutNodes() {
        this.nodePositions = {};

        const controllerNode = this.nodes.find(n => n.id === 'controller');
        if (controllerNode) {
            controllerNode.x = this.centerX;
            controllerNode.y = this.centerY;
            this.nodePositions['controller'] = controllerNode;
        }

        const workerNodes = this.nodes.filter(n => n.id !== 'controller');
        const angleStep = (2 * Math.PI) / Math.max(workerNodes.length, 1);
        const radius = Math.min(this.canvas.width, this.canvas.height) * 0.35;

        workerNodes.forEach((node, i) => {
            const angle = i * angleStep - Math.PI / 2;
            node.x = this.centerX + Math.cos(angle) * radius;
            node.y = this.centerY + Math.sin(angle) * radius;
            this.nodePositions[node.id] = node;
        });
    }

    getNodeAtPosition(mx, my) {
        for (const node of this.nodes) {
            const dx = mx - node.x;
            const dy = my - node.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist <= node.radius + 10) {
                return node;
            }
        }
        return null;
    }

    onMouseMove(e) {
        const rect = this.canvas.getBoundingClientRect();
        const mx = e.clientX - rect.left;
        const my = e.clientY - rect.top;

        const node = this.getNodeAtPosition(mx, my);
        this.hoveredNode = node;
        this.canvas.style.cursor = node ? 'pointer' : 'default';
    }

    onClickHandler(e) {
        const rect = this.canvas.getBoundingClientRect();
        const mx = e.clientX - rect.left;
        const my = e.clientY - rect.top;

        const node = this.getNodeAtPosition(mx, my);
        if (node && this.onClick) {
            this.onClick(node);
        }
    }

    startAnimation() {
        if (this.animationId) return;
        this.animate();
    }

    stopAnimation() {
        if (this.animationId) {
            cancelAnimationFrame(this.animationId);
            this.animationId = null;
        }
    }

    animate() {
        this.draw();
        this.animationId = requestAnimationFrame(() => this.animate());
    }

    draw() {
        const { ctx, canvas } = this;
        ctx.clearRect(0, 0, canvas.width, canvas.height);

        this.edges.forEach(edge => this.drawEdge(edge));
        this.nodes.forEach(node => this.drawNode(node));
    }

    drawEdge(edge) {
        const fromNode = this.nodePositions[edge.from];
        const toNode = this.nodePositions[edge.to];
        if (!fromNode || !toNode) return;

        const { ctx } = this;
        ctx.beginPath();
        ctx.moveTo(fromNode.x, fromNode.y);
        ctx.lineTo(toNode.x, toNode.y);

        if (edge.status === 'busy') {
            ctx.strokeStyle = '#f59e0b';
            ctx.setLineDash([8, 4]);
        } else {
            ctx.strokeStyle = 'rgba(59, 130, 246, 0.4)';
            ctx.setLineDash([]);
        }

        ctx.lineWidth = 2;
        ctx.stroke();
        ctx.setLineDash([]);
    }

    drawNode(node) {
        const { ctx } = this;
        const isHovered = this.hoveredNode && this.hoveredNode.id === node.id;
        const currentRadius = isHovered ? node.radius + 5 : node.radius;

        const gradient = ctx.createRadialGradient(
            node.x - currentRadius / 3,
            node.y - currentRadius / 3,
            0,
            node.x,
            node.y,
            currentRadius
        );

        if (node.type === 'controller') {
            gradient.addColorStop(0, '#60a5fa');
            gradient.addColorStop(1, '#3b82f6');
        } else if (node.status === 'available') {
            gradient.addColorStop(0, '#34d399');
            gradient.addColorStop(1, '#10b981');
        } else if (node.status === 'busy') {
            gradient.addColorStop(0, '#fbbf24');
            gradient.addColorStop(1, '#f59e0b');
        } else {
            gradient.addColorStop(0, '#f87171');
            gradient.addColorStop(1, '#ef4444');
        }

        ctx.beginPath();
        ctx.arc(node.x, node.y, currentRadius, 0, Math.PI * 2);
        ctx.fillStyle = gradient;
        ctx.fill();

        ctx.beginPath();
        ctx.arc(node.x, node.y, currentRadius + 6, 0, Math.PI * 2);
        const glowColor = isHovered ? 'rgba(59, 130, 246, 0.5)' : 'rgba(59, 130, 246, 0.15)';
        ctx.strokeStyle = glowColor;
        ctx.lineWidth = isHovered ? 3 : 2;
        ctx.stroke();

        ctx.fillStyle = '#fff';
        ctx.font = node.type === 'controller' ? 'bold 16px Inter, sans-serif' : '13px Inter, sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(node.type === 'controller' ? 'C' : 'W', node.x, node.y);

        ctx.fillStyle = '#e5e7eb';
        ctx.font = '12px Inter, sans-serif';
        ctx.textBaseline = 'top';
        ctx.fillText(node.label, node.x, node.y + currentRadius + 12);
    }
}

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
        canvas.addEventListener('mouseleave', () => { this.hoveredNode = null; });
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
                id,
                label: worker.name || worker.host || `Worker ${i + 1}`,
                type: 'worker',
                status: worker.status || 'available',
                radius: 30,
                data: worker,
            });
            this.edges.push({ from: 'controller', to: id, status: worker.status === 'busy' ? 'busy' : 'active' });
        });

        this.layoutNodes();
    }

    layoutNodes() {
        this.nodePositions = {};
        const controller = this.nodes.find(n => n.id === 'controller');
        if (controller) {
            controller.x = this.centerX;
            controller.y = this.centerY;
            this.nodePositions['controller'] = controller;
        }

        const workers = this.nodes.filter(n => n.id !== 'controller');
        const angleStep = (2 * Math.PI) / Math.max(workers.length, 1);
        const radius = Math.min(this.canvas.width, this.canvas.height) * 0.35;

        workers.forEach((node, i) => {
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
            if (Math.sqrt(dx * dx + dy * dy) <= node.radius + 10) return node;
        }
        return null;
    }

    onMouseMove(e) {
        const rect = this.canvas.getBoundingClientRect();
        const node = this.getNodeAtPosition(e.clientX - rect.left, e.clientY - rect.top);
        this.hoveredNode = node;
        this.canvas.style.cursor = node ? 'pointer' : 'default';
    }

    onClickHandler(e) {
        const rect = this.canvas.getBoundingClientRect();
        const node = this.getNodeAtPosition(e.clientX - rect.left, e.clientY - rect.top);
        if (node && this.onClick) this.onClick(node);
    }

    startAnimation() {
        if (this.animationId) return;
        this.animate();
    }

    stopAnimation() {
        if (this.animationId) { cancelAnimationFrame(this.animationId); this.animationId = null; }
    }

    animate() {
        this.draw();
        this.animationId = requestAnimationFrame(() => this.animate());
    }

    draw() {
        const { ctx, canvas } = this;
        ctx.clearRect(0, 0, canvas.width, canvas.height);
        this.edges.forEach(e => this.drawEdge(e));
        this.nodes.forEach(n => this.drawNode(n));
    }

    drawEdge(edge) {
        const from = this.nodePositions[edge.from];
        const to = this.nodePositions[edge.to];
        if (!from || !to) return;

        const { ctx } = this;
        ctx.beginPath();
        ctx.moveTo(from.x, from.y);
        ctx.lineTo(to.x, to.y);
        ctx.strokeStyle = edge.status === 'busy' ? '#f59e0b' : 'rgba(208,188,255,.25)';
        ctx.lineWidth = 2;
        ctx.setLineDash(edge.status === 'busy' ? [8, 4] : []);
        ctx.stroke();
        ctx.setLineDash([]);
    }

    drawNode(node) {
        const { ctx } = this;
        const isHovered = this.hoveredNode && this.hoveredNode.id === node.id;
        const r = isHovered ? node.radius + 6 : node.radius;

        const gradient = ctx.createRadialGradient(node.x - r / 3, node.y - r / 3, 0, node.x, node.y, r);

        if (node.type === 'controller') {
            gradient.addColorStop(0, '#d0bcff');
            gradient.addColorStop(1, '#6750a4');
        } else if (node.status === 'available') {
            gradient.addColorStop(0, '#b8f397');
            gradient.addColorStop(1, '#386a20');
        } else if (node.status === 'busy') {
            gradient.addColorStop(0, '#fcd87b');
            gradient.addColorStop(1, '#d4a017');
        } else {
            gradient.addColorStop(0, '#f2b8b5');
            gradient.addColorStop(1, '#b3261e');
        }

        ctx.beginPath();
        ctx.arc(node.x, node.y, r, 0, Math.PI * 2);
        ctx.fillStyle = gradient;
        ctx.fill();

        if (isHovered) {
            ctx.beginPath();
            ctx.arc(node.x, node.y, r + 8, 0, Math.PI * 2);
            ctx.strokeStyle = 'rgba(208,188,255,.35)';
            ctx.lineWidth = 3;
            ctx.stroke();
        }

        ctx.fillStyle = '#fff';
        ctx.font = node.type === 'controller' ? '700 18px Google Sans, Roboto, sans-serif' : '600 14px Roboto, sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(node.type === 'controller' ? 'C' : 'W', node.x, node.y);

        ctx.fillStyle = '#e6e1e5';
        ctx.font = '500 12px Roboto, sans-serif';
        ctx.textBaseline = 'top';
        ctx.fillText(node.label, node.x, node.y + r + 12);
    }
}

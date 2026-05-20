class TopologyGraph {
    constructor(canvas) {
        this.canvas = canvas;
        this.ctx = canvas.getContext('2d');
        this.nodes = [];
        this.edges = [];
        this.animationId = null;
        this.centerX = 0;
        this.centerY = 0;
    }

    resize() {
        const rect = this.canvas.parentElement.getBoundingClientRect();
        this.canvas.width = rect.width - 48;
        this.canvas.height = 400;
        this.centerX = this.canvas.width / 2;
        this.centerY = this.canvas.height / 2;
    }

    update(data) {
        this.nodes = [];
        this.edges = [];

        this.nodes.push({
            id: 'controller',
            label: 'Controller',
            x: this.centerX,
            y: this.centerY,
            type: 'controller',
            status: 'active',
            radius: 35,
        });

        const workers = data.workers || [];
        const angleStep = (2 * Math.PI) / Math.max(workers.length, 1);
        const radius = Math.min(this.canvas.width, this.canvas.height) * 0.35;

        workers.forEach((worker, i) => {
            const angle = i * angleStep - Math.PI / 2;
            this.nodes.push({
                id: worker.name || worker.host,
                label: worker.name || worker.host,
                x: this.centerX + Math.cos(angle) * radius,
                y: this.centerY + Math.sin(angle) * radius,
                type: 'worker',
                status: worker.status || 'available',
                radius: 25,
                data: worker,
            });

            this.edges.push({
                from: 'controller',
                to: worker.name || worker.host,
                status: worker.status === 'busy' ? 'busy' : 'active',
            });
        });
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
        const fromNode = this.nodes.find(n => n.id === edge.from);
        const toNode = this.nodes.find(n => n.id === edge.to);
        if (!fromNode || !toNode) return;

        const { ctx } = this;
        ctx.beginPath();
        ctx.moveTo(fromNode.x, fromNode.y);
        ctx.lineTo(toNode.x, toNode.y);

        if (edge.status === 'busy') {
            ctx.strokeStyle = '#f59e0b';
            ctx.setLineDash([8, 4]);
        } else {
            ctx.strokeStyle = '#3b82f6';
            ctx.setLineDash([]);
        }

        ctx.lineWidth = 2;
        ctx.stroke();
        ctx.setLineDash([]);

        const midX = (fromNode.x + toNode.x) / 2;
        const midY = (fromNode.y + toNode.y) / 2;
        const angle = Math.atan2(toNode.y - fromNode.y, toNode.x - fromNode.x);

        const arrowSize = 8;
        ctx.save();
        ctx.translate(midX, midY);
        ctx.rotate(angle);
        ctx.beginPath();
        ctx.moveTo(arrowSize, 0);
        ctx.lineTo(-arrowSize, -arrowSize / 2);
        ctx.lineTo(-arrowSize, arrowSize / 2);
        ctx.closePath();
        ctx.fillStyle = edge.status === 'busy' ? '#f59e0b' : '#3b82f6';
        ctx.fill();
        ctx.restore();
    }

    drawNode(node) {
        const { ctx } = this;

        const gradient = ctx.createRadialGradient(
            node.x - node.radius / 3,
            node.y - node.radius / 3,
            0,
            node.x,
            node.y,
            node.radius
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
        ctx.arc(node.x, node.y, node.radius, 0, Math.PI * 2);
        ctx.fillStyle = gradient;
        ctx.fill();

        ctx.beginPath();
        ctx.arc(node.x, node.y, node.radius + 4, 0, Math.PI * 2);
        ctx.strokeStyle = node.type === 'controller' ? '#60a5fa' : '#10b981';
        ctx.lineWidth = 2;
        ctx.globalAlpha = 0.3;
        ctx.stroke();
        ctx.globalAlpha = 1;

        ctx.fillStyle = '#fff';
        ctx.font = node.type === 'controller' ? 'bold 14px Inter, sans-serif' : '12px Inter, sans-serif';
        ctx.textAlign = 'center';
        ctx.textBaseline = 'middle';
        ctx.fillText(node.type === 'controller' ? 'C' : 'W', node.x, node.y);

        ctx.fillStyle = '#e5e7eb';
        ctx.font = '11px Inter, sans-serif';
        ctx.textBaseline = 'top';
        ctx.fillText(node.label, node.x, node.y + node.radius + 10);
    }
}

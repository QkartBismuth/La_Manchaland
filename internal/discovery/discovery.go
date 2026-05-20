package discovery

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
)

const (
	ServiceType = "_llama-distributed"
	Domain      = "local"
	TxtKeyHost  = "host"
	TxtKeyPort  = "port"
	TxtKeyName  = "name"
	TxtKeyVRAM  = "vram"
	TxtKeyRAM   = "ram"
	TxtKeyGPU   = "gpu"
	TxtKeyCPU   = "cpu"
)

type WorkerInfo struct {
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	Port      int       `json:"port"`
	IP        string    `json:"ip"`
	VRAMTotal uint64    `json:"vram_total"`
	VRAMUsed  uint64    `json:"vram_used"`
	RAMTotal  uint64    `json:"ram_total"`
	RAMUsed   uint64    `json:"ram_used"`
	GPUName   string    `json:"gpu_name"`
	CPULoad   float64   `json:"cpu_load"`
	Status    string    `json:"status"`
	LastSeen  time.Time `json:"last_seen"`
}

type Discovery struct {
	server  *zeroconf.Server
	resolver *zeroconf.Resolver
	mu      sync.RWMutex
	workers map[string]*WorkerInfo
	ch      chan *WorkerInfo
}

func NewDiscovery() *Discovery {
	return &Discovery{
		workers: make(map[string]*WorkerInfo),
		ch:      make(chan *WorkerInfo, 100),
	}
}

func (d *Discovery) StartWorkerBroadcast(ctx context.Context, name string, port int, metadata map[string]string) error {
	host, _ := getLocalIP()

	var entries []string
	for k, v := range metadata {
		entries = append(entries, fmt.Sprintf("%s=%s", k, v))
	}

	server, err := zeroconf.Register(
		name,
		ServiceType,
		Domain,
		port,
		entries,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to register mDNS service: %w", err)
	}

	d.server = server
	log.Printf("[mDNS] Worker '%s' registered on %s:%d", name, host, port)

	<-ctx.Done()
	d.server.Shutdown()
	return nil
}

func (d *Discovery) StartScanner(ctx context.Context) error {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return fmt.Errorf("failed to create resolver: %w", err)
	}

	d.resolver = resolver

	entries := make(chan *zeroconf.ServiceEntry)

	go func() {
		for entry := range entries {
			info := parseEntry(entry)
			d.mu.Lock()
			d.workers[info.Host+":"+fmt.Sprint(info.Port)] = info
			d.mu.Unlock()
			select {
			case d.ch <- info:
			default:
			}
			log.Printf("[mDNS] Discovered worker: %s at %s:%d", info.Name, entry.AddrIPv4[0], info.Port)
		}
	}()

	err = resolver.Browse(ctx, ServiceType, Domain, entries)
	if err != nil {
		return fmt.Errorf("failed to browse services: %w", err)
	}

	log.Printf("[mDNS] Scanner started, listening for workers...")
	<-ctx.Done()
	return nil
}

func (d *Discovery) GetWorkers() []*WorkerInfo {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*WorkerInfo, 0, len(d.workers))
	for _, w := range d.workers {
		result = append(result, w)
	}
	return result
}

func (d *Discovery) AddManualWorker(host string, port int, name string) {
	key := fmt.Sprintf("%s:%d", host, port)

	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()
	if existing, ok := d.workers[key]; ok {
		existing.LastSeen = now
		existing.Status = "connected"
		return
	}

	d.workers[key] = &WorkerInfo{
		Name:     name,
		Host:     host,
		Port:     port,
		IP:       host,
		Status:   "connected",
		LastSeen: now,
	}
	log.Printf("[manual] Added worker: %s at %s:%d", name, host, port)
}

func (d *Discovery) UpdateWorkerMetrics(key string, metrics func(*WorkerInfo)) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if w, ok := d.workers[key]; ok {
		metrics(w)
		w.LastSeen = time.Now()
	}
}

func (d *Discovery) RemoveWorker(key string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.workers, key)
}

func (d *Discovery) WorkerChannel() <-chan *WorkerInfo {
	return d.ch
}

func parseEntry(entry *zeroconf.ServiceEntry) *WorkerInfo {
	info := &WorkerInfo{
		Name:   entry.Instance,
		Status: "available",
	}

	if len(entry.AddrIPv4) > 0 {
		info.IP = entry.AddrIPv4[0].String()
		info.Host = info.IP
	}

	info.Port = entry.Port
	info.LastSeen = time.Now()

	for _, txt := range entry.Text {
		_ = txt // Parse key=value pairs
	}

	return info
}

func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String(), nil
			}
		}
	}

	return "127.0.0.1", nil
}

package monitor

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
)

type GPUMetrics struct {
	Name       string `json:"name"`
	MemoryTotal uint64 `json:"memory_total"`
	MemoryUsed  uint64 `json:"memory_used"`
	MemoryFree  uint64 `json:"memory_free"`
	Utilization int    `json:"utilization"`
	Temperature int    `json:"temperature"`
}

type SystemMetrics struct {
	CPULoad    float64      `json:"cpu_load"`
	MemoryTotal uint64     `json:"memory_total"`
	MemoryUsed  uint64     `json:"memory_used"`
	MemoryFree  uint64     `json:"memory_free"`
	GPUs        []GPUMetrics `json:"gpus"`
}

type Monitor struct {
	mu        sync.RWMutex
	lastStats SystemMetrics
	hasCUDA   bool
	gpuCount  int
}

func New() *Monitor {
	m := &Monitor{}
	m.hasCUDA = detectCUDA()
	return m
}

func (m *Monitor) HasCUDA() bool {
	return m.hasCUDA
}

func (m *Monitor) GetMetrics() SystemMetrics {
	metrics := SystemMetrics{}

	v, err := mem.VirtualMemory()
	if err == nil {
		metrics.MemoryTotal = v.Total
		metrics.MemoryUsed = v.Used
		metrics.MemoryFree = v.Available
	}

	cpuPct, err := cpu.Percent(0, false)
	if err == nil && len(cpuPct) > 0 {
		metrics.CPULoad = cpuPct[0]
	}

	if m.hasCUDA {
		metrics.GPUs = m.getGPUMetrics()
	}

	m.mu.Lock()
	m.lastStats = metrics
	m.mu.Unlock()

	return metrics
}

func (m *Monitor) StartReporting(ctx context.Context, callback func(SystemMetrics), interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics := m.GetMetrics()
			callback(metrics)
		}
	}
}

func detectCUDA() bool {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("nvidia-smi", "--query-gpu=count", "--format=csv,noheader")
		output, err := cmd.Output()
		if err == nil {
			count, _ := strconv.Atoi(strings.TrimSpace(string(output)))
			return count > 0
		}
	} else {
		cmd := exec.Command("nvidia-smi", "-L")
		output, err := cmd.Output()
		if err == nil && len(output) > 0 {
			return true
		}
	}
	return false
}

func (m *Monitor) getGPUMetrics() []GPUMetrics {
	if runtime.GOOS == "windows" {
		return m.getWindowsGPUMetrics()
	}
	return m.getLinuxGPUMetrics()
}

func (m *Monitor) getWindowsGPUMetrics() []GPUMetrics {
	var gpus []GPUMetrics

	cmd := exec.Command("nvidia-smi",
		"--query-gpu=name,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu",
		"--format=csv,noheader,nounits")

	output, err := cmd.Output()
	if err != nil {
		return gpus
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Split(line, ", ")
		if len(fields) < 6 {
			continue
		}

		memTotal, _ := strconv.ParseUint(strings.TrimSpace(fields[1]), 10, 64)
		memUsed, _ := strconv.ParseUint(strings.TrimSpace(fields[2]), 10, 64)
		memFree, _ := strconv.ParseUint(strings.TrimSpace(fields[3]), 10, 64)
		util, _ := strconv.Atoi(strings.TrimSpace(fields[4]))
		temp, _ := strconv.Atoi(strings.TrimSpace(fields[5]))

		gpus = append(gpus, GPUMetrics{
			Name:        strings.TrimSpace(fields[0]),
			MemoryTotal: memTotal * 1024 * 1024,
			MemoryUsed:  memUsed * 1024 * 1024,
			MemoryFree:  memFree * 1024 * 1024,
			Utilization: util,
			Temperature: temp,
		})
	}

	return gpus
}

func (m *Monitor) getLinuxGPUMetrics() []GPUMetrics {
	var gpus []GPUMetrics

	cmd := exec.Command("nvidia-smi",
		"--query-gpu=name,memory.total,memory.used,memory.free,utilization.gpu,temperature.gpu",
		"--format=csv,noheader,nounits")

	output, err := cmd.Output()
	if err != nil {
		return gpus
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Split(line, ", ")
		if len(fields) < 6 {
			continue
		}

		memTotal, _ := strconv.ParseUint(strings.TrimSpace(fields[1]), 10, 64)
		memUsed, _ := strconv.ParseUint(strings.TrimSpace(fields[2]), 10, 64)
		memFree, _ := strconv.ParseUint(strings.TrimSpace(fields[3]), 10, 64)
		util, _ := strconv.Atoi(strings.TrimSpace(fields[4]))
		temp, _ := strconv.Atoi(strings.TrimSpace(fields[5]))

		gpus = append(gpus, GPUMetrics{
			Name:        strings.TrimSpace(fields[0]),
			MemoryTotal: memTotal * 1024 * 1024,
			MemoryUsed:  memUsed * 1024 * 1024,
			MemoryFree:  memFree * 1024 * 1024,
			Utilization: util,
			Temperature: temp,
		})
	}

	return gpus
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

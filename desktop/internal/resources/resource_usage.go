package resources

import (
	"sync"
	"time"
)

// ResourceUsage reports the native application process, excluding WebView helpers.
// A nil CPUPercent means the first sample is still establishing a baseline.
type ResourceUsage struct {
	CPUPercent  *float64 `json:"cpu_percent"`
	MemoryBytes uint64   `json:"memory_bytes"`
}

type processSample struct {
	at          time.Time
	cpuTime     time.Duration
	memoryBytes uint64
}

type ResourceSampler struct {
	mu       sync.Mutex
	previous processSample
}

func (s *ResourceSampler) measure(current processSample) ResourceUsage {
	usage := ResourceUsage{MemoryBytes: current.memoryBytes}
	elapsed := current.at.Sub(s.previous.at)
	cpu := current.cpuTime - s.previous.cpuTime
	if !s.previous.at.IsZero() && elapsed > 0 && cpu >= 0 {
		percent := float64(cpu) / float64(elapsed) * 100
		usage.CPUPercent = &percent
	}
	s.previous = current
	return usage
}

// GetResourceUsage samples cumulative CPU time and current resident memory.
// CPU follows Activity Monitor's convention: one fully occupied core is 100%.
func (s *ResourceSampler) Sample() (ResourceUsage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sample, err := readProcessSample()
	if err != nil {
		s.previous = processSample{}
		return ResourceUsage{}, err
	}
	return s.measure(sample), nil
}

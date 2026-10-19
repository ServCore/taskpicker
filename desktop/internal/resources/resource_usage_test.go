package resources

import (
	"testing"
	"time"
)

func TestResourceSampler(t *testing.T) {
	var sampler ResourceSampler
	start := time.Now()
	first := sampler.measure(processSample{at: start, cpuTime: time.Second, memoryBytes: 123})
	if first.CPUPercent != nil || first.MemoryBytes != 123 {
		t.Fatalf("first sample should establish CPU baseline: %+v", first)
	}
	// Two busy cores over two seconds should report 200%, not be capped at 100%.
	second := sampler.measure(processSample{at: start.Add(2 * time.Second), cpuTime: 5 * time.Second, memoryBytes: 456})
	if second.CPUPercent == nil || *second.CPUPercent != 200 || second.MemoryBytes != 456 {
		t.Fatalf("unexpected interval usage: %+v", second)
	}
	idle := sampler.measure(processSample{at: start.Add(4 * time.Second), cpuTime: 5 * time.Second})
	if idle.CPUPercent == nil || *idle.CPUPercent != 0 {
		t.Fatalf("idle interval should report zero: %+v", idle)
	}
	reset := sampler.measure(processSample{at: start.Add(6 * time.Second), cpuTime: time.Second})
	if reset.CPUPercent != nil {
		t.Fatal("counter reset must not produce negative CPU usage")
	}
}

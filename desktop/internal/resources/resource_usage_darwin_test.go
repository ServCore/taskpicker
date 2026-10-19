//go:build darwin && cgo

package resources

import (
	"testing"
)

func TestNativeResourceUsage(t *testing.T) {
	sampler := &ResourceSampler{}
	first, err := sampler.Sample()
	if err != nil {
		t.Fatal(err)
	}
	if first.MemoryBytes == 0 || first.CPUPercent != nil {
		t.Fatalf("expected resident memory and CPU baseline: %+v", first)
	}
	second, err := sampler.Sample()
	if err != nil || second.MemoryBytes == 0 || second.CPUPercent == nil || *second.CPUPercent < 0 {
		t.Fatalf("expected a valid second sample: %+v, %v", second, err)
	}
}

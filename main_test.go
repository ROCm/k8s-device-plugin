package main

import (
	"testing"
)

func TestCountGPUDev(t *testing.T) {
	count := countGPUDev("testdata/topology-parsing")

	expCount := 2
	if count != expCount {
		t.Errorf("Count was incorrect, got: %d, want: %d.", count, expCount)
	}
}

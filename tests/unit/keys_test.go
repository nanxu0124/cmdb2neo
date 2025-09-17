package unit

import (
	"testing"

	"cmdb2neo/internal/domain"
)

func TestLabelPattern(t *testing.T) {
	pattern := domain.LabelPattern([]string{"Compute", "VirtualMachine"})
	if pattern != ":Compute:VirtualMachine" {
		t.Fatalf("unexpected pattern %s", pattern)
	}
}

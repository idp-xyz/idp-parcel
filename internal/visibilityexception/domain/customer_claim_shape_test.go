package domain

import "testing"

// Covers: AT-PS-124——索赔口的形状版本是 VCL-1（与 PS 四口的名不重，带 -1 后缀）。
func TestCustomerClaimShapeVersionIsNamed(t *testing.T) {
	if CustomerClaimShapeVersion != "VCL-1" {
		t.Fatalf("CustomerClaimShapeVersion = %q, want VCL-1", CustomerClaimShapeVersion)
	}
}

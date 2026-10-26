package domain

import "testing"

// Covers: AT-PS-124——索赔口的形状版本是 VCL-1（与 PS 四口的名不重，带 -1 后缀）。
func TestCustomerClaimShapeVersionIsNamed(t *testing.T) {
	if CustomerClaimShapeVersion != "VCL-1" {
		t.Fatalf("CustomerClaimShapeVersion = %q, want VCL-1", CustomerClaimShapeVersion)
	}
}

// Covers: AT-VE-208——VCL-1 的载荷字段是封闭全集：只有接入请求标识与六个业务引用，租户、账户、操作者与提交时间都不在其中
// （前三者取自信封，提交时间取到达信封）。
func TestCustomerClaimShapeFieldsAreClosedAndCarryNoIdentity(t *testing.T) {
	want := []string{"accessRequestId", "batch", "item", "applicant", "contract", "target", "kind"}
	if len(CustomerClaimShapeFields) != len(want) {
		t.Fatalf("fields = %v, want %v", CustomerClaimShapeFields, want)
	}
	for index, name := range want {
		if CustomerClaimShapeFields[index] != name {
			t.Fatalf("field %d = %q, want %q", index, CustomerClaimShapeFields[index], name)
		}
	}
}

package domain

import "testing"

// Covers: 全集、valid、String 三者同一来源——全集里的每一格都有名、都 valid；不在全集里的取值没有名、也不 valid。String 仍是
// switch，这一条钉住它与全集不会各说各的。
func TestDisputeResolutionKindsValidAndStringAgree(t *testing.T) {
	inList := map[DisputeResolutionKind]bool{}
	for _, kind := range DisputeResolutionKinds() {
		inList[kind] = true
	}
	for value := 0; value < 256; value++ {
		kind := DisputeResolutionKind(value)
		if inList[kind] != kind.valid() || inList[kind] != (kind.String() != "") {
			t.Fatalf("取值 %d：在全集 %v、valid %v、名 %q", value, inList[kind], kind.valid(), kind.String())
		}
	}
}

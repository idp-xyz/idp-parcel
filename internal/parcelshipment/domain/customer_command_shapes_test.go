package domain

import "testing"

// Covers: AT-PS-124——首方客户命令四口各有自己的形状版本名，名互不相同、也不同于提交口的 PSC-1，带版本号后缀（ADR-0139 决定八）。
func TestCustomerCommandShapeVersionsAreDistinct(t *testing.T) {
	seen := map[string]bool{payloadCanonicalizationVersion: true}
	for _, version := range []string{
		CustomerWithdrawalShapeVersion, CustomerParcelCancellationShapeVersion,
		CustomerSourceDataAmendmentShapeVersion, CustomerSupplementShapeVersion,
	} {
		if len(version) < 3 || version[len(version)-2:] != "-1" {
			t.Fatalf("形状版本 %q 不带 -1 后缀", version)
		}
		if seen[version] {
			t.Fatalf("形状版本 %q 与别的口重名", version)
		}
		seen[version] = true
	}
}

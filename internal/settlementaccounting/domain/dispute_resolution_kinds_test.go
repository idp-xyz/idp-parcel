package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: DisputeResolutionKinds 交出的全集与四走向同一份，终局恰是接受、部分接受、拒绝——读写两面按 Final 从全集里挑去库里筛，
// 漏一格或多一格都会让待对账的张数与「还能不能再处理」悄悄走样。
func TestDisputeResolutionKindsListsEveryKindOnceAndOnlyPendingReviewIsNotFinal(t *testing.T) {
	seen := map[string]bool{}
	for _, kind := range domain.DisputeResolutionKinds() {
		if kind.String() == "" || seen[kind.String()] {
			t.Fatalf("全集里有无名或重复的一格：%v", kind)
		}
		seen[kind.String()] = true
		if kind.Final() == (kind == domain.DisputePendingReview) {
			t.Fatalf("%s 的终局判读不对", kind)
		}
	}
	for value := 0; value < 256; value++ {
		if name := domain.DisputeResolutionKind(value).String(); name != "" && !seen[name] {
			t.Fatalf("有一格不在全集里：%s", name)
		}
	}
}

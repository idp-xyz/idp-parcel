package identity_test

import (
	"bytes"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/identity"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// Covers: ADR-0212 决定二「标识由 NO 签发」——签出的预报标识互不相同、过得了领域构造、带预报前缀；熵源读不满时如实报错，不交回一个
// 可预测的值。
func TestEachIssuedDeliveryPreAdviceIDIsNewAndCarriesItsPrefix(t *testing.T) {
	factory, err := identity.NewDeliveryPreAdviceIDs()
	if err != nil {
		t.Fatal(err)
	}
	issued := make(map[string]bool, 256)
	for range 256 {
		id, err := factory.NextDeliveryPreAdviceID(t.Context())
		if err != nil {
			t.Fatalf("签发：%v", err)
		}
		if !strings.HasPrefix(id.String(), "PREADV-") || issued[id.String()] {
			t.Fatalf("标识 %q 没带前缀或重号", id.String())
		}
		issued[id.String()] = true
	}

	starved, err := identity.NewDeliveryPreAdviceIDs(platformidentity.WithEntropy(bytes.NewReader([]byte{1, 2, 3})))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := starved.NextDeliveryPreAdviceID(t.Context()); err == nil {
		t.Fatal("熵源只给了三个字节，签发却成功了")
	}
}

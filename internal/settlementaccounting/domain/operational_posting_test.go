package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func TestPostedAndUnsettledSubtractsWriteoffFromBothSides(t *testing.T) {
	posted, unsettled, err := domain.PostedAndUnsettled(10_000, 4_000, 7_000)
	if err != nil {
		t.Fatalf("求余额：%v", err)
	}
	if posted != 6_000 || unsettled != 3_000 {
		t.Fatalf("入账 = %d，未结 = %d，want 6000 / 3000", posted, unsettled)
	}
}

func TestPostedAndUnsettledKeepsANegativePostedNet(t *testing.T) {
	posted, unsettled, err := domain.PostedAndUnsettled(-500, 0, 0)
	if err != nil {
		t.Fatalf("退回大于收款：%v", err)
	}
	if posted != -500 || unsettled != 0 {
		t.Fatalf("入账 = %d，未结 = %d，want -500 / 0", posted, unsettled)
	}
}

func TestPostedAndUnsettledRefusesANegativeUnsettled(t *testing.T) {
	posted, unsettled, err := domain.PostedAndUnsettled(100, 50, 0)
	if !errors.Is(err, domain.ErrNegativeConfirmedUnsettled) {
		t.Fatalf("err = %v，want 已确认未结为负", err)
	}
	if posted != 0 || unsettled != 0 {
		t.Fatalf("失败时仍交出数：入账 = %d，未结 = %d", posted, unsettled)
	}
}

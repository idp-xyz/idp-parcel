package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func TestOperatingBasisAdoptionNamesAtLeastOneBasis(t *testing.T) {
	if _, err := domain.NewOperatingBasisAdoption(false, false); !errors.Is(err, domain.ErrOperatingBasisAdoptionEmpty) {
		t.Fatalf("两个都是假应拒，err=%v", err)
	}
	estimated, err := domain.NewOperatingBasisAdoption(true, false)
	if err != nil || !estimated.Estimated() || estimated.Confirmed() || !estimated.Adopts(domain.EstimatedBasis) || estimated.Adopts(domain.ConfirmedBasis) {
		t.Fatalf("只采用预估：%+v err=%v", estimated, err)
	}
	both, err := domain.NewOperatingBasisAdoption(true, true)
	if err != nil || !both.Adopts(domain.EstimatedBasis) || !both.Adopts(domain.ConfirmedBasis) || both.Adopts(domain.SettledBasis) {
		t.Fatalf("两者都采用仍不放行已结算：%+v err=%v", both, err)
	}
	if !estimated.Equal(estimated) || estimated.Equal(both) {
		t.Fatal("同内容应相等，异内容不应相等")
	}
}

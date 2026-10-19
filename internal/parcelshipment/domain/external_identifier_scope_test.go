package domain_test

import (
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// Covers: PS CONTEXT「范围相容」——范围只对适用范围是客户账户编号空间的关联起作用：客户包裹件号在客户账户内算数，渠道包裹件号在渠道
// 服务方内算数（ADR-0212 决定四「渠道包裹件号不需要范围」）。读口按这份分类比，分类只在领域一处。
func TestOnlyCustomerParcelNumbersCountInACustomerAccountNumberSpace(t *testing.T) {
	if !domain.CustomerParcelNumber.CountsInCustomerAccountSpace() {
		t.Fatal("客户包裹件号没算在客户账户编号空间里")
	}
	if domain.ChannelParcelNumber.CountsInCustomerAccountSpace() || domain.ExternalIdentifierKindInvalid.CountsInCustomerAccountSpace() {
		t.Fatal("渠道包裹件号或坏值被算进客户账户编号空间")
	}
	if got := domain.CustomerAccountSpaceKinds(); !slices.Equal(got, []domain.ExternalIdentifierKind{domain.CustomerParcelNumber}) {
		t.Fatalf("客户账户编号空间的类型 = %v, want 只有客户包裹件号", got)
	}
}

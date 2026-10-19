package domain

import "fmt"

// SellBillingMomentAdoption 是一个结算账户采用的包裹计费时点（ADR-0190 决定三，租户取值）：键是（租户、结算账户），
// 值是（计费时点、计价范围）。不设修订，照 ADR-0158 与 ADR-0179 决定五的不可变元组：同一户再登同一份是重放，任一格
// 不同是冲突、不覆盖。一户只采用一种时点（决定二）由键形本身守住。租户在册的键上，不在这个值里。
type SellBillingMomentAdoption struct {
	account SettlementAccountID
	moment  ParcelBillingMoment
	scope   EstimatePricingScope
}

// NewSellBillingMomentAdoption 拒空白与词表外的时点——没写就是没写，不默认成任何一种。
func NewSellBillingMomentAdoption(
	account SettlementAccountID,
	moment ParcelBillingMoment,
	scope EstimatePricingScope,
) (SellBillingMomentAdoption, error) {
	if !account.valid() || !scope.valid() {
		return SellBillingMomentAdoption{}, fmt.Errorf("%w: sell billing moment adoption", ErrBlankValue)
	}
	if !moment.valid() {
		return SellBillingMomentAdoption{}, fmt.Errorf("%w: parcel billing moment %d", ErrBlankValue, uint8(moment))
	}
	return SellBillingMomentAdoption{account: account, moment: moment, scope: scope}, nil
}

func (adoption SellBillingMomentAdoption) Account() SettlementAccountID { return adoption.account }
func (adoption SellBillingMomentAdoption) Moment() ParcelBillingMoment  { return adoption.moment }
func (adoption SellBillingMomentAdoption) PricingScope() EstimatePricingScope {
	return adoption.scope
}

// SameRegistration 比的是登记内容，不含落库时刻。
func (adoption SellBillingMomentAdoption) SameRegistration(other SellBillingMomentAdoption) bool {
	return adoption.account.String() == other.account.String() &&
		adoption.moment == other.moment &&
		adoption.scope.String() == other.scope.String()
}

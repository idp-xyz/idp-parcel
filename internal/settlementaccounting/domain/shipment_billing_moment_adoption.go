package domain

import "fmt"

// ShipmentBillingMomentAdoption 是一个结算账户采用的委托级计费时点。键是（租户、结算账户），值是时点，计价范围可缺。
// 没有这一行就不触发。不设默认采用，也不把包裹计费时点那两种词写进这一本。
type ShipmentBillingMomentAdoption struct {
	account SettlementAccountID
	moment  ShipmentBillingMoment
	scope   EstimatePricingScope
}

// NewShipmentBillingMomentAdoption 拒空白与词表外的时点。没写就是没写。
func NewShipmentBillingMomentAdoption(
	account SettlementAccountID,
	moment ShipmentBillingMoment,
	scope EstimatePricingScope,
) (ShipmentBillingMomentAdoption, error) {
	if !account.valid() {
		return ShipmentBillingMomentAdoption{}, fmt.Errorf("%w: shipment billing moment adoption", ErrBlankValue)
	}
	if !moment.valid() {
		return ShipmentBillingMomentAdoption{}, fmt.Errorf("%w: shipment billing moment %d", ErrBlankValue, uint8(moment))
	}
	return ShipmentBillingMomentAdoption{account: account, moment: moment, scope: scope}, nil
}

func (adoption ShipmentBillingMomentAdoption) Account() SettlementAccountID { return adoption.account }
func (adoption ShipmentBillingMomentAdoption) Moment() ShipmentBillingMoment {
	return adoption.moment
}
func (adoption ShipmentBillingMomentAdoption) PricingScope() EstimatePricingScope {
	return adoption.scope
}

// SameRegistration 比的是登记内容，不含落库时刻。
func (adoption ShipmentBillingMomentAdoption) SameRegistration(other ShipmentBillingMomentAdoption) bool {
	return adoption.account.String() == other.account.String() &&
		adoption.moment == other.moment &&
		adoption.scope.String() == other.scope.String()
}

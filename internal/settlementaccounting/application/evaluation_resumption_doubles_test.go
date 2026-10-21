package application_test

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// stallRegisterDouble 是两本发起停办册的内存替身：同一自然键再记一次就换上这一次，与真册同一口径；err 让写口答不出。
type stallRegisterDouble struct {
	sell map[string]domain.SellEvaluationInitiationStall
	buy  map[string]domain.BuyEvaluationInitiationStall
	err  error
}

func newStallRegister() *stallRegisterDouble {
	return &stallRegisterDouble{
		sell: map[string]domain.SellEvaluationInitiationStall{},
		buy:  map[string]domain.BuyEvaluationInitiationStall{},
	}
}

func sellStallKey(tenant domain.TenantID, key domain.SellEvaluationRequestNaturalKey) string {
	return tenant.String() + "/" + key.Parcel.String() + "/" + key.Moment.String()
}

func buyStallKey(tenant domain.TenantID, occurrence domain.ChargeOccurrenceID, version domain.OccurrenceVersion) string {
	return tenant.String() + "/" + occurrence.String() + "@" + version.String()
}

func (double *stallRegisterDouble) RecordSellInitiationStall(
	_ context.Context, tenant domain.TenantID, stall domain.SellEvaluationInitiationStall,
) error {
	if double.err != nil {
		return double.err
	}
	double.sell[sellStallKey(tenant, stall.NaturalKey())] = stall
	return nil
}

func (double *stallRegisterDouble) FindSellInitiationStall(
	_ context.Context, tenant domain.TenantID, key domain.SellEvaluationRequestNaturalKey,
) (domain.SellEvaluationInitiationStall, bool, error) {
	stall, found := double.sell[sellStallKey(tenant, key)]
	return stall, found, nil
}

func (double *stallRegisterDouble) RecordBuyInitiationStall(
	_ context.Context, tenant domain.TenantID, stall domain.BuyEvaluationInitiationStall,
) error {
	if double.err != nil {
		return double.err
	}
	double.buy[buyStallKey(tenant, stall.Occurrence().ID(), stall.Occurrence().Version())] = stall
	return nil
}

func (double *stallRegisterDouble) FindBuyInitiationStall(
	_ context.Context, tenant domain.TenantID, occurrence domain.ChargeOccurrenceID, version domain.OccurrenceVersion,
) (domain.BuyEvaluationInitiationStall, bool, error) {
	stall, found := double.buy[buyStallKey(tenant, occurrence, version)]
	return stall, found, nil
}

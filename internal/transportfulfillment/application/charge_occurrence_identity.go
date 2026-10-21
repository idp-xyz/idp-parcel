package application

import (
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 发生项的身份与「是否已被另一项计过」那一问的派生，只在这一处（ADR-0207 决定一、四，ADR-0209 决定四）。登记用例形成发生项、补形成
// 记未形成之前问「盖住」（ADR-0231 决定六）都从这里取：两边各派生一份，登记用例的派生日后一改，补形成就会问一个对不上的身份，把
// 已形成的那一件记成未形成。

// actualFulfillmentScope 交回实际履约发生项在一段上的事实依据与主要业务范围：都取那一段。
func actualFulfillmentScope(segment domain.FulfillmentSegmentReference) (domain.OccurrenceBasisReference, domain.OccurrenceScopeReference, error) {
	basis, err := domain.ActualFulfillmentBasis(segment)
	if err != nil {
		return domain.OccurrenceBasisReference{}, domain.OccurrenceScopeReference{}, err
	}
	scope, err := domain.SegmentOccurrenceScope(segment)
	if err != nil {
		return domain.OccurrenceBasisReference{}, domain.OccurrenceScopeReference{}, err
	}
	return basis, scope, nil
}

// occurrenceIdentity 由（租户、原因、事实依据、成员集合）派生发生项身份，交回它首版的键与问「盖住」的那一问：同一（租户、原因）、
// 同一范围上另一项仍有效的发生项是否已计过这件事。本次按不按范围计要等协议的数量形，由调用方在问之前填 WholeScope。
func occurrenceIdentity(
	tenant domain.TenantID,
	reason domain.ChargeOccurrenceReason,
	basis domain.OccurrenceBasisReference,
	scope domain.OccurrenceScopeReference,
	members []domain.CarriedObjectReference,
) (ports.ChargeOccurrenceKey, ports.ChargeOccurrenceCoverageQuery, error) {
	reference, err := domain.DeriveChargeOccurrenceReference(tenant, reason, basis, members)
	if err != nil {
		return ports.ChargeOccurrenceKey{}, ports.ChargeOccurrenceCoverageQuery{}, err
	}
	key := ports.ChargeOccurrenceKey{TenantID: tenant, Occurrence: reference, Validity: domain.FirstOccurrenceValidity()}
	coverage := ports.ChargeOccurrenceCoverageQuery{
		TenantID:   tenant,
		Reason:     reason,
		Scope:      scope,
		FactBasis:  basis,
		Members:    members,
		Except:     reference,
		ScopeUnits: domain.ScopeCountingUnits(),
	}
	return key, coverage, nil
}

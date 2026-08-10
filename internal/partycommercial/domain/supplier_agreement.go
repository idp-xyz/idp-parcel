package domain

import (
	"errors"
	"time"
)

var ErrInvalidSupplierAgreement = errors.New("party commercial: invalid supplier agreement")

// SupplierAgreement is the content of one supplier agreement version: the
// reusable procurement conditions, the purchase-direction pricing plan it binds
// and the settlement responsibility scope.
//
// It holds no fulfilment or payable fact. transport-fulfillment snapshots the
// agreement and conditions a particular transport order adopted; settlement-
// accounting forms expected supplier cost, bill matching and approved payables.
// Keeping those out is what stops one commercial version from appearing to own
// what actually happened under it.
type SupplierAgreement struct {
	version       CommercialVersion
	supplier      PartyID
	legalEntity   LegalEntityReference
	scope         CommercialScopeReference
	purchasePlan  PricingPlanReference
	effective     EffectiveInterval
	terminatedAt  time.Time
	terminationOn RelationshipBasisReference
}

func NewSupplierAgreement(
	version CommercialVersion,
	supplier PartyID,
	legalEntity LegalEntityReference,
	scope CommercialScopeReference,
	purchasePlan PricingPlanReference,
	effective EffectiveInterval,
) (SupplierAgreement, error) {
	if version.kind != SupplierAgreementObject ||
		version.status != CommercialVersionEffective ||
		!supplier.valid() || !legalEntity.valid() || !scope.valid() ||
		!purchasePlan.valid() || !effective.valid() {
		return SupplierAgreement{}, ErrInvalidSupplierAgreement
	}
	return SupplierAgreement{
		version:      version,
		supplier:     supplier,
		legalEntity:  legalEntity,
		scope:        scope,
		purchasePlan: purchasePlan,
		effective:    effective,
	}, nil
}

func (agreement SupplierAgreement) Version() CommercialVersion {
	return agreement.version
}

func (agreement SupplierAgreement) Supplier() PartyID {
	return agreement.supplier
}

func (agreement SupplierAgreement) Scope() CommercialScopeReference {
	return agreement.scope
}

// PurchasePricingPlan is the buy-direction plan this agreement binds. Purchase,
// sale and inter-entity pricing rules are expressed separately, so this plan is
// never an executable price toward a customer.
func (agreement SupplierAgreement) PurchasePricingPlan() PricingPlanReference {
	return agreement.purchasePlan
}

// Direction is fixed at BUY. A supplier agreement arranges procurement; letting
// it carry any other direction would make a cost look like a sellable price.
func (agreement SupplierAgreement) Direction() PriceDirection {
	return BuyDirection
}

// Terminate stops the agreement supporting new procurement decisions from that
// moment. It rewrites nothing: transport orders, fulfilment facts, supplier bill
// claims and approved payables formed under it keep citing what was true then.
func (agreement SupplierAgreement) Terminate(
	basis RelationshipBasisReference,
	at time.Time,
) (SupplierAgreement, error) {
	if !agreement.terminatedAt.IsZero() || !basis.valid() || at.IsZero() {
		return SupplierAgreement{}, ErrInvalidSupplierAgreement
	}
	agreement.terminatedAt = at.UTC()
	agreement.terminationOn = basis
	return agreement, nil
}

func (agreement SupplierAgreement) TerminatedAt() (time.Time, bool) {
	if agreement.terminatedAt.IsZero() {
		return time.Time{}, false
	}
	return agreement.terminatedAt, true
}

// SupportsProcurementAt answers whether a new procurement decision may rest on
// this agreement. Approval and effectiveness are both required, and termination
// closes it from its own moment onward.
func (agreement SupplierAgreement) SupportsProcurementAt(at time.Time) bool {
	if agreement.version.status != CommercialVersionEffective {
		return false
	}
	if !agreement.terminatedAt.IsZero() && !at.Before(agreement.terminatedAt) {
		return false
	}
	return agreement.effective.Contains(at)
}

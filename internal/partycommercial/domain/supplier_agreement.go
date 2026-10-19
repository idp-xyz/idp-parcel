package domain

import (
	"errors"
	"time"
)

var ErrInvalidSupplierAgreement = errors.New("party commercial: invalid supplier agreement")

// SupplierAgreement 是一个供应商商业协议版本的正文：可复用的采购条件、它绑定的
// 采购方向定价方案，以及结算责任范围。
//
// 它不保存任何履约事实或应付事实。transport-fulfillment 保存一次运输委托实际采用
// 的协议与履约条件快照，settlement-accounting 形成供应商预期成本、账单匹配和审核
// 应付。把这些挡在外面，才不会让一个商业版本看起来拥有在它之下实际发生的事。
type SupplierAgreement struct {
	version       CommercialVersion
	supplier      PartyID
	legalEntity   LegalEntityReference
	scope         CommercialScopeReference
	purchasePlan  PricingPlanReference
	effective     EffectiveInterval
	terminatedAt  time.Time
	terminationOn RelationshipBasisReference
	// chargeConditions 是可缺的收费发生条件一节（ADR-0209）。零值即没声明：运输履约按这一版登记发生项时答`未配置`。
	chargeConditions ChargeOccurrenceConditions
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

// WithChargeConditions 交回带收费发生条件的同一版正文。条件只能随正文一次发布固定：发布后再加就是改一份已固定的
// 正文，要另发新版本（CONTEXT「采购服务、价格、结算或责任条件变化时形成新版本」），所以已带条件的正文再加即拒。
func (agreement SupplierAgreement) WithChargeConditions(conditions ChargeOccurrenceConditions) (SupplierAgreement, error) {
	if !conditions.valid() || agreement.chargeConditions.valid() || !agreement.supplier.valid() {
		return SupplierAgreement{}, ErrInvalidSupplierAgreement
	}
	agreement.chargeConditions = conditions
	return agreement, nil
}

// ChargeConditions 交回这一版声明的收费发生条件；整节没声明答 false。
func (agreement SupplierAgreement) ChargeConditions() (ChargeOccurrenceConditions, bool) {
	if !agreement.chargeConditions.valid() {
		return ChargeOccurrenceConditions{}, false
	}
	return agreement.chargeConditions, true
}

func (agreement SupplierAgreement) Version() CommercialVersion {
	return agreement.version
}

func (agreement SupplierAgreement) Supplier() PartyID {
	return agreement.supplier
}

func (agreement SupplierAgreement) LegalEntity() LegalEntityReference {
	return agreement.legalEntity
}

func (agreement SupplierAgreement) Scope() CommercialScopeReference {
	return agreement.scope
}

func (agreement SupplierAgreement) Effective() EffectiveInterval {
	return agreement.effective
}

// PurchasePricingPlan 是本协议绑定的采购方向定价方案。采购、销售和法人间结算价格
// 规则分别表达，所以这个方案永远不是面向客户的可执行价格。
func (agreement SupplierAgreement) PurchasePricingPlan() PricingPlanReference {
	return agreement.purchasePlan
}

// Direction 固定为 BUY。供应商商业协议约定的是采购；让它带上别的价格方向，
// 等于把一笔成本装扮成可售价格。
func (agreement SupplierAgreement) Direction() PriceDirection {
	return BuyDirection
}

// Terminate 自该时点起停止本协议支持新的采购决定。它不改写任何既有事实：在它之下
// 已经形成的运输委托、履约事实、供应商账单主张和审核应付，继续引用当时有效的依据。
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

// SupportsProcurementAt 回答一个新的采购决定能否依据本协议形成。批准生效与落在
// 有效区间内两者都必需；终止自其自身时点起关闭后续使用。
func (agreement SupplierAgreement) SupportsProcurementAt(at time.Time) bool {
	if agreement.version.status != CommercialVersionEffective {
		return false
	}
	if !agreement.terminatedAt.IsZero() && !at.Before(agreement.terminatedAt) {
		return false
	}
	return agreement.effective.Contains(at)
}

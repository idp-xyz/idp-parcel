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
	// selectionCap 是壳上最新一条区间更正的区间（NarrowedBy），capped 标出它在场；retired 标出壳已同起点退役（EndedBy）。
	// 三者都是点读时从版本册折进来的，不是发布时的正文。
	selectionCap EffectiveInterval
	capped       bool
	retired      bool
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

// NarrowedBy 交回被版本壳上的区间更正收窄的同一版正文（ADR-0233 决定五）：此后按版本点读的核对也照更正后的区间判，
// 与闭包选候选同一口径（ADR-0038 决定二）。正文自带的区间不改写，更正与它取交，判法同 bodyExcludedAt。
func (agreement SupplierAgreement) NarrowedBy(correction ValidityCorrection) (SupplierAgreement, error) {
	if !sameVersionKey(agreement.version, correction.tenant, correction.kind, correction.objectID, correction.version) ||
		!correction.corrected.valid() {
		return SupplierAgreement{}, ErrInvalidSupplierAgreement
	}
	agreement.selectionCap = correction.corrected
	agreement.capped = true
	return agreement, nil
}

// EndedBy 交回壳已同起点退役（ADR-0233 决定三、五）的同一版正文：它不再支持任何新的采购决定，终止时刻读作退役时刻。
// 退役只管退役之后作出的决定，而问到这里的决定都在其后，所以与业务时点无关地一律不支持——旧版的整段时间都由同起点
// 的后继接着。
func (agreement SupplierAgreement) EndedBy(retirement VersionRetirement) (SupplierAgreement, error) {
	if !sameVersionKey(agreement.version, retirement.tenant, retirement.kind, retirement.objectID, retirement.version) ||
		retirement.at.IsZero() {
		return SupplierAgreement{}, ErrInvalidSupplierAgreement
	}
	agreement.retired = true
	if agreement.terminatedAt.IsZero() || retirement.at.Before(agreement.terminatedAt) {
		agreement.terminatedAt = retirement.at
	}
	return agreement, nil
}

func sameVersionKey(version CommercialVersion, tenant TenantID, kind CommercialObjectKind, objectID CommercialObjectID, label CommercialVersionLabel) bool {
	return version.tenant == tenant && version.kind == kind && version.objectID == objectID && version.version == label
}

// SupportsProcurementAt 回答一个新的采购决定能否依据本协议形成。批准生效与落在
// 有效区间内两者都必需；终止自其自身时点起关闭后续使用；壳上的区间更正收窄区间，同起点退役关闭全部新的使用。
func (agreement SupplierAgreement) SupportsProcurementAt(at time.Time) bool {
	if agreement.version.status != CommercialVersionEffective || agreement.retired {
		return false
	}
	if !agreement.terminatedAt.IsZero() && !at.Before(agreement.terminatedAt) {
		return false
	}
	if agreement.capped && !agreement.selectionCap.Contains(at) {
		return false
	}
	return agreement.effective.Contains(at)
}

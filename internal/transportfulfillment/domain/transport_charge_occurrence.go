package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidChargeOccurrence = errors.New("transport fulfillment: invalid transport charge occurrence")
	// ErrNotAFailedAttempt：失败尝试费的事实依据只能是失败的对象结果——把揽收到手的
	// 对象报成失败尝试费，是在给不存在的失败开成本来源。
	ErrNotAFailedAttempt = errors.New("transport fulfillment: the referenced attempt result did not fail")
)

// ChargeOccurrenceReference 是发生项的稳定身份。settlement-accounting 以它回指本体，
// 供应商预期成本、账单匹配都锚在它上。
type ChargeOccurrenceReference struct{ requiredValue }

func NewChargeOccurrenceReference(value string) (ChargeOccurrenceReference, error) {
	required, err := newRequiredValue("charge occurrence reference", value)
	return ChargeOccurrenceReference{required}, err
}

// JourneyReference 指名发生项所属的旅程。原旅程与替代/改送/退运旅程各自关联自己的
// 发生项——新旅程不得覆盖、搬移或自动净额抵销原旅程发生项（CONTEXT 成本来源节）。
type JourneyReference struct{ requiredValue }

func NewJourneyReference(value string) (JourneyReference, error) {
	required, err := newRequiredValue("journey reference", value)
	return JourneyReference{required}, err
}

// ProcurementLegalEntityReference 指名承担本次采购责任的运营法人。
type ProcurementLegalEntityReference struct{ requiredValue }

func NewProcurementLegalEntityReference(value string) (ProcurementLegalEntityReference, error) {
	required, err := newRequiredValue("procurement legal entity reference", value)
	return ProcurementLegalEntityReference{required}, err
}

// ServiceProviderReference 指名服务提供方（伙伴或自营法人）。
type ServiceProviderReference struct{ requiredValue }

func NewServiceProviderReference(value string) (ServiceProviderReference, error) {
	required, err := newRequiredValue("service provider reference", value)
	return ServiceProviderReference{required}, err
}

// AgreementSnapshotReference 指名实际采用的供应商协议与履约条件快照。协议版本生命周期
// 属 party-commercial，这里只存快照引用。
type AgreementSnapshotReference struct{ requiredValue }

func NewAgreementSnapshotReference(value string) (AgreementSnapshotReference, error) {
	required, err := newRequiredValue("agreement snapshot reference", value)
	return AgreementSnapshotReference{required}, err
}

// OccurrenceScopeReference 指名发生项的唯一主要业务范围。
type OccurrenceScopeReference struct{ requiredValue }

func NewOccurrenceScopeReference(value string) (OccurrenceScopeReference, error) {
	required, err := newRequiredValue("occurrence scope reference", value)
	return OccurrenceScopeReference{required}, err
}

// OccurrenceBasisReference 指名发生项引用的事实依据（订舱对象、取消结果、失败的尝试
// 结果、履约段/参与关系），以及有效性修订所依据的更正来源。
type OccurrenceBasisReference struct{ requiredValue }

func NewOccurrenceBasisReference(value string) (OccurrenceBasisReference, error) {
	required, err := newRequiredValue("occurrence basis reference", value)
	return OccurrenceBasisReference{required}, err
}

// QuantityUnitReference 指名数量的单位。数量与单位成对固定，不能事后按报表分组换算。
type QuantityUnitReference struct{ requiredValue }

func NewQuantityUnitReference(value string) (QuantityUnitReference, error) {
	required, err := newRequiredValue("quantity unit reference", value)
	return QuantityUnitReference{required}, err
}

// OccurrenceValidityVersion 是发生项的有效性版本：更正换版本，不删原项。
type OccurrenceValidityVersion struct{ requiredValue }

func NewOccurrenceValidityVersion(value string) (OccurrenceValidityVersion, error) {
	required, err := newRequiredValue("occurrence validity version", value)
	return OccurrenceValidityVersion{required}, err
}

// ChargeOccurrenceReason 是发生原因的封闭四值（CONTEXT：「订舱费、尚未执行服务的取消费
// ……失败揽收/派送尝试费和其他适用运输服务分别形成发生项」）。各自引不同的事实依据：
// 订舱引订舱对象、取消引取消结果、失败尝试引失败的对象结果、实际履约引履约段或参与关系。
type ChargeOccurrenceReason uint8

const (
	ChargeOccurrenceReasonInvalid ChargeOccurrenceReason = iota
	BookingOccurrence
	CancellationOccurrence
	FailedAttemptOccurrence
	ActualFulfillmentOccurrence
)

func (reason ChargeOccurrenceReason) valid() bool {
	return reason >= BookingOccurrence && reason <= ActualFulfillmentOccurrence
}

func (reason ChargeOccurrenceReason) String() string {
	switch reason {
	case BookingOccurrence:
		return "BOOKING"
	case CancellationOccurrence:
		return "CANCELLATION"
	case FailedAttemptOccurrence:
		return "FAILED_ATTEMPT"
	case ActualFulfillmentOccurrence:
		return "ACTUAL_FULFILLMENT"
	default:
		return ""
	}
}

// OccurrenceRevisionKind 是有效性修订的封闭走向：失效（发生范围被证明无效）或替代
// （由新的判断替换）。范围更正需要自己的形状，另票落地——这里刻意没有第三格。
type OccurrenceRevisionKind uint8

const (
	OccurrenceRevisionKindInvalid OccurrenceRevisionKind = iota
	OccurrenceInvalidated
	OccurrenceSuperseded
)

func (kind OccurrenceRevisionKind) valid() bool {
	return kind == OccurrenceInvalidated || kind == OccurrenceSuperseded
}

func (kind OccurrenceRevisionKind) String() string {
	switch kind {
	case OccurrenceInvalidated:
		return "INVALIDATED"
	case OccurrenceSuperseded:
		return "SUPERSEDED"
	default:
		return ""
	}
}

// TransportChargeOccurrenceSpec 是形成一个发生项所需的全部输入。
type TransportChargeOccurrenceSpec struct {
	TenantID    TenantID
	Occurrence  ChargeOccurrenceReference
	Journey     JourneyReference
	LegalEntity ProcurementLegalEntityReference
	Provider    ServiceProviderReference
	Agreement   AgreementSnapshotReference
	Reason      ChargeOccurrenceReason
	FactBasis   OccurrenceBasisReference
	Scope       OccurrenceScopeReference
	Members     []CarriedObjectReference
	Quantity    int64
	Unit        QuantityUnitReference
	OccurredAt  time.Time
	Validity    OccurrenceValidityVersion
}

// TransportChargeOccurrence 是运输准备或实际履约中已经发生、可能依据供应商协议形成
// 外部运输成本的业务事实范围（CONTEXT「运输收费发生项」）。
//
// 它不是价格、供应商预期成本、账单主张、审核应付或真实付款——类型上没有任何金额或
// 币种字段可以承载那些结论；`parcel-pricing` 形成 BUY 纯评价，`settlement-accounting`
// 再判断费用金额责任。
type TransportChargeOccurrence struct {
	tenantID      TenantID
	occurrence    ChargeOccurrenceReference
	journey       JourneyReference
	legalEntity   ProcurementLegalEntityReference
	provider      ServiceProviderReference
	agreement     AgreementSnapshotReference
	reason        ChargeOccurrenceReason
	factBasis     OccurrenceBasisReference
	scope         OccurrenceScopeReference
	members       []CarriedObjectReference
	quantity      int64
	unit          QuantityUnitReference
	occurredAt    time.Time
	validity      OccurrenceValidityVersion
	corrects      OccurrenceValidityVersion
	revisionKind  OccurrenceRevisionKind
	revisionBasis OccurrenceBasisReference
	revisedAt     time.Time
}

// FormTransportChargeOccurrence 固定发生项的全部依据。不能根据当前伙伴、当前线路或
// 报表分组临时补齐（CONTEXT）——缺件在构造期被拒，不留待事后填充。
func FormTransportChargeOccurrence(spec TransportChargeOccurrenceSpec) (TransportChargeOccurrence, error) {
	if !spec.TenantID.valid() ||
		!spec.Occurrence.valid() ||
		!spec.Journey.valid() ||
		!spec.LegalEntity.valid() ||
		!spec.Provider.valid() ||
		!spec.Agreement.valid() ||
		!spec.Reason.valid() ||
		!spec.FactBasis.valid() ||
		!spec.Scope.valid() ||
		len(spec.Members) == 0 ||
		spec.Quantity <= 0 ||
		!spec.Unit.valid() ||
		spec.OccurredAt.IsZero() ||
		!spec.Validity.valid() {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	seen := make(map[CarriedObjectReference]struct{}, len(spec.Members))
	for _, member := range spec.Members {
		if !member.valid() {
			return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
		}
		if _, exists := seen[member]; exists {
			return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
		}
		seen[member] = struct{}{}
	}
	return TransportChargeOccurrence{
		tenantID:    spec.TenantID,
		occurrence:  spec.Occurrence,
		journey:     spec.Journey,
		legalEntity: spec.LegalEntity,
		provider:    spec.Provider,
		agreement:   spec.Agreement,
		reason:      spec.Reason,
		factBasis:   spec.FactBasis,
		scope:       spec.Scope,
		members:     append([]CarriedObjectReference(nil), spec.Members...),
		quantity:    spec.Quantity,
		unit:        spec.Unit,
		occurredAt:  spec.OccurredAt.UTC(),
		validity:    spec.Validity,
	}, nil
}

// ChargeOccurrenceForFailedAttempt 从失败的对象结果形成失败尝试费发生项：事实依据与
// 业务时间取自结果本身，揽收到手的结果被拒（AT-TF-094：失败尝试形成发生项，第二次成功
// 不覆盖第一次）。
func ChargeOccurrenceForFailedAttempt(
	spec TransportChargeOccurrenceSpec,
	result AttemptObjectResult,
) (TransportChargeOccurrence, error) {
	if !result.Outcome().Failed() {
		return TransportChargeOccurrence{}, ErrNotAFailedAttempt
	}
	basis, err := NewOccurrenceBasisReference(
		"ATTEMPT-RESULT/" + result.Attempt().String() + "/" + result.Object().String())
	if err != nil {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	spec.Reason = FailedAttemptOccurrence
	spec.FactBasis = basis
	spec.OccurredAt = result.OccurredAt()
	return FormTransportChargeOccurrence(spec)
}

func (occurrence TransportChargeOccurrence) TenantID() TenantID {
	return occurrence.tenantID
}

func (occurrence TransportChargeOccurrence) Occurrence() ChargeOccurrenceReference {
	return occurrence.occurrence
}

func (occurrence TransportChargeOccurrence) Journey() JourneyReference {
	return occurrence.journey
}

func (occurrence TransportChargeOccurrence) LegalEntity() ProcurementLegalEntityReference {
	return occurrence.legalEntity
}

func (occurrence TransportChargeOccurrence) Provider() ServiceProviderReference {
	return occurrence.provider
}

func (occurrence TransportChargeOccurrence) Agreement() AgreementSnapshotReference {
	return occurrence.agreement
}

func (occurrence TransportChargeOccurrence) Reason() ChargeOccurrenceReason {
	return occurrence.reason
}

func (occurrence TransportChargeOccurrence) FactBasis() OccurrenceBasisReference {
	return occurrence.factBasis
}

func (occurrence TransportChargeOccurrence) Scope() OccurrenceScopeReference {
	return occurrence.scope
}

func (occurrence TransportChargeOccurrence) Members() []CarriedObjectReference {
	return append([]CarriedObjectReference(nil), occurrence.members...)
}

func (occurrence TransportChargeOccurrence) Quantity() (int64, QuantityUnitReference) {
	return occurrence.quantity, occurrence.unit
}

// OccurredAt 是发生的业务时间——结算与审计锚在它上，消息与处理时间不能替代。
func (occurrence TransportChargeOccurrence) OccurredAt() time.Time {
	return occurrence.occurredAt
}

func (occurrence TransportChargeOccurrence) Validity() OccurrenceValidityVersion {
	return occurrence.validity
}

// Corrects 交回本版本修订的前一有效性版本（若本版本由修订产生）。
func (occurrence TransportChargeOccurrence) Corrects() (OccurrenceValidityVersion, bool) {
	if !occurrence.corrects.valid() {
		return OccurrenceValidityVersion{}, false
	}
	return occurrence.corrects, true
}

// Revision 报告修订三件（走向、依据、时刻），只在由修订产生的版本上给出。
func (occurrence TransportChargeOccurrence) Revision() (OccurrenceRevisionKind, OccurrenceBasisReference, time.Time, bool) {
	if occurrence.revisedAt.IsZero() {
		return OccurrenceRevisionKindInvalid, OccurrenceBasisReference{}, time.Time{}, false
	}
	return occurrence.revisionKind, occurrence.revisionBasis, occurrence.revisedAt, true
}

// ReviseValidity 依据更正来源形成新的有效性版本：保留原发生项（值语义，接收者不动），
// 新版本回指前身（CONTEXT「来源更正……保留原发生项并形成失效、替代或范围更正关系；
// 结算依据新的有效性追加调整，不删除原成本」）。沿用原版本号就是覆盖，构造期拒绝；
// 旅程、范围与成员原样保留——修订换的是有效性，不是把发生项搬去另一个旅程。
func (occurrence TransportChargeOccurrence) ReviseValidity(
	kind OccurrenceRevisionKind,
	version OccurrenceValidityVersion,
	basis OccurrenceBasisReference,
	revisedAt time.Time,
) (TransportChargeOccurrence, error) {
	if !occurrence.validity.valid() {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	if !kind.valid() || !version.valid() || !basis.valid() || revisedAt.IsZero() {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	if version == occurrence.validity {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	if revisedAt.Before(occurrence.occurredAt) {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	revised := occurrence
	revised.members = append([]CarriedObjectReference(nil), occurrence.members...)
	revised.corrects = occurrence.validity
	revised.validity = version
	revised.revisionKind = kind
	revised.revisionBasis = basis
	revised.revisedAt = revisedAt.UTC()
	return revised, nil
}

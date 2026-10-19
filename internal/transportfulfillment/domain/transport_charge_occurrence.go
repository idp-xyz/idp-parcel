package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"
)

var (
	ErrInvalidChargeOccurrence = errors.New("transport fulfillment: invalid transport charge occurrence")
	// ErrNotAFailedAttempt：失败尝试费的事实依据只能是失败的对象结果——把揽收到手的
	// 对象报成失败尝试费，是在给不存在的失败开成本来源。
	ErrNotAFailedAttempt = errors.New("transport fulfillment: the referenced attempt result did not fail")
	// ErrNotAParticipant：实际履约发生项的成员必须在段里有一条有效参与。不在段里、或段里那条参与已被更正撤回
	// （失效版本），都没有可计费的履约范围——替它开发生项就是给没发生的运输开成本来源。
	ErrNotAParticipant = errors.New("transport fulfillment: the object has no valid participation in the segment")
	// ErrParticipationNotEnded：参与还在场，履约范围没有终点。实际履约发生项锚在参与结束那一刻（下一次交接、有效
	// 交付或明确终止）；在场时登记，数量与时点都还没定下来。
	ErrParticipationNotEnded = errors.New("transport fulfillment: the participation has not ended")
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

// ParseChargeOccurrenceReason 按字面认原因，大小写不折叠：登记册、信封消费方与结算触发册存的都是这四个字面量。
func ParseChargeOccurrenceReason(value string) (ChargeOccurrenceReason, error) {
	for reason := BookingOccurrence; reason <= ActualFulfillmentOccurrence; reason++ {
		if reason.String() == value {
			return reason, nil
		}
	}
	return ChargeOccurrenceReasonInvalid, ErrInvalidChargeOccurrence
}

// OccurrenceConditionBasis 是「凭什么算满足了采用协议的发生条件」的封闭集合（CONTEXT「只有满足当前采用协议所需
// 事实条件的明确范围，才能形成运输收费发生项」；ADR-0207 决定三）。
//
// 首版只有登记者断言一格：协议正文还没有声明发生条件的文法，条件由人判断并留痕。协议能声明之后多出的那一格
// 归 ADR-0209，到时按声明判的发生项与按断言形成的发生项在册上仍分得开——审计要问的正是「这一项是谁判的」。
type OccurrenceConditionBasis uint8

const (
	OccurrenceConditionBasisInvalid OccurrenceConditionBasis = iota
	ConditionAssertedByRegistrant
)

func (basis OccurrenceConditionBasis) String() string {
	switch basis {
	case ConditionAssertedByRegistrant:
		return "REGISTRANT_ASSERTED"
	default:
		return ""
	}
}

// OccurrenceCondition 是一个发生项满足发生条件的依据。断言是登记者的原话（引哪一版协议的哪一条、凭什么事实），
// 不是布尔：只记「断言过」等于让日后复核的人无从知道当时断言的是什么。
type OccurrenceCondition struct {
	basis     OccurrenceConditionBasis
	assertion requiredValue
}

// NewAssertedOccurrenceCondition 立一份登记者断言。断言缺席即拒：没有人为条件作答，就没有发生项可形成。
func NewAssertedOccurrenceCondition(assertion string) (OccurrenceCondition, error) {
	statement, err := newRequiredValue("occurrence condition assertion", assertion)
	if err != nil {
		return OccurrenceCondition{}, err
	}
	return OccurrenceCondition{basis: ConditionAssertedByRegistrant, assertion: statement}, nil
}

func (condition OccurrenceCondition) Basis() OccurrenceConditionBasis {
	return condition.basis
}

// Assertion 交回登记者断言的原话；只在断言那一格上给出。
func (condition OccurrenceCondition) Assertion() (string, bool) {
	if condition.basis != ConditionAssertedByRegistrant {
		return "", false
	}
	return condition.assertion.String(), true
}

func (condition OccurrenceCondition) valid() bool {
	return condition.basis == ConditionAssertedByRegistrant && condition.assertion.valid()
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
	Condition   OccurrenceCondition
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
	condition     OccurrenceCondition
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
		AtRegistryPrecision(spec.OccurredAt).IsZero() ||
		!spec.Validity.valid() ||
		!spec.Condition.valid() {
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
		condition:   spec.Condition,
	}, nil
}

// ChargeOccurrenceForFailedAttempt 从失败的揽收对象结果形成失败尝试费发生项：事实依据、主要业务范围与业务时间
// 取自结果本身，揽收到手的结果被拒（AT-TF-094：失败尝试形成发生项，第二次成功不覆盖第一次）。范围取那一次尝试：
// 一次到场是一个可计费的范围，同一次到场里其他对象的成败各成其项。
func ChargeOccurrenceForFailedAttempt(
	spec TransportChargeOccurrenceSpec,
	result AttemptObjectResult,
) (TransportChargeOccurrence, error) {
	basis, err := FailedPickupAttemptBasis(result)
	if err != nil {
		return TransportChargeOccurrence{}, err
	}
	scope, err := NewOccurrenceScopeReference(result.Attempt().String())
	if err != nil {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	spec.Reason = FailedAttemptOccurrence
	spec.FactBasis = basis
	spec.Scope = scope
	spec.OccurredAt = result.OccurredAt()
	return FormTransportChargeOccurrence(spec)
}

// FailedPickupAttemptBasis 是失败揽收结果作事实依据时的引用。与派送那一侧分写前缀：两侧的尝试身份各由自己的
// 登记册签发，同一个串在两侧可以各指一次到场，共用前缀会让两项落到同一个发生项身份上。
func FailedPickupAttemptBasis(result AttemptObjectResult) (OccurrenceBasisReference, error) {
	if !result.Outcome().Failed() {
		return OccurrenceBasisReference{}, ErrNotAFailedAttempt
	}
	basis, err := NewOccurrenceBasisReference(
		"ATTEMPT-RESULT/" + result.Attempt().String() + "/" + result.Object().String())
	if err != nil {
		return OccurrenceBasisReference{}, ErrInvalidChargeOccurrence
	}
	return basis, nil
}

// FailedDeliveryAttemptBasis 是失败派送结果（无人签收、地址错误、拒收）作事实依据时的引用（AT-TF-095）。
func FailedDeliveryAttemptBasis(result DeliveryAttemptResult) (OccurrenceBasisReference, error) {
	if !result.Outcome().Failed() {
		return OccurrenceBasisReference{}, ErrNotAFailedAttempt
	}
	basis, err := NewOccurrenceBasisReference(
		"DELIVERY-ATTEMPT-RESULT/" + result.Attempt().String() + "/" + result.Object().String())
	if err != nil {
		return OccurrenceBasisReference{}, ErrInvalidChargeOccurrence
	}
	return basis, nil
}

// ChargeOccurrenceForDeliveryAttempt 从失败的派送对象结果形成失败尝试费发生项，形同揽收那一侧：依据、范围与时点
// 取自结果，妥投被拒。妥投的成本来源若有，是实际履约那一项，不是一次「失败」。
func ChargeOccurrenceForDeliveryAttempt(
	spec TransportChargeOccurrenceSpec,
	result DeliveryAttemptResult,
) (TransportChargeOccurrence, error) {
	basis, err := FailedDeliveryAttemptBasis(result)
	if err != nil {
		return TransportChargeOccurrence{}, err
	}
	scope, err := NewOccurrenceScopeReference(result.Attempt().String())
	if err != nil {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	spec.Reason = FailedAttemptOccurrence
	spec.FactBasis = basis
	spec.Scope = scope
	spec.OccurredAt = result.OccurredAt()
	return FormTransportChargeOccurrence(spec)
}

// ActualFulfillmentBasis 是实际履约发生项的事实依据：那一个实际履约段。成员各自的参与与终点在段里，成员清单与业务
// 时间另由发生项固定；依据只指段，同一段上不同成员范围的两项因此由成员区分，不由依据区分。
func ActualFulfillmentBasis(segment FulfillmentSegmentReference) (OccurrenceBasisReference, error) {
	if !segment.valid() {
		return OccurrenceBasisReference{}, ErrInvalidChargeOccurrence
	}
	return NewOccurrenceBasisReference("FULFILLMENT-SEGMENT/" + segment.String())
}

// ChargeOccurrenceForActualFulfillment 从段内已结束的参与形成实际履约发生项（ADR-0207 决定一）。成员取规格里的
// 对象，每一个都必须在段里有一条有效参与、且那条参与已经结束；业务时间取各成员终点中最晚的一刻——那是这个成员
// 范围的运输全部完成的时刻，早于它登出的时点会让还在途的成员被算成已履约。范围取段本身。
//
// 判的是参与的当前版（替代链的链尾）：被更正替代的旧版不再表达控制，凭它计费等于按已被推翻的事实开成本来源。
func ChargeOccurrenceForActualFulfillment(
	spec TransportChargeOccurrenceSpec,
	segment ActualFulfillmentSegment,
) (TransportChargeOccurrence, error) {
	if spec.TenantID != segment.TenantID() {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	latest, err := ActualFulfillmentEndedAt(segment, spec.Members)
	if err != nil {
		return TransportChargeOccurrence{}, err
	}
	basis, err := ActualFulfillmentBasis(segment.Segment())
	if err != nil {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	scope, err := NewOccurrenceScopeReference(segment.Segment().String())
	if err != nil {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	spec.Reason = ActualFulfillmentOccurrence
	spec.FactBasis = basis
	spec.Scope = scope
	spec.OccurredAt = latest
	return FormTransportChargeOccurrence(spec)
}

// ActualFulfillmentEndedAt 判成员在段里的参与是否都已结束，交回最晚的终点。登记方要在形成之前拿到这个时点去核
// 协议（协议在业务发生那一刻是否支持采购），与形成时取的是同一个判据，所以由领域一处给出、不让编排另算一遍。
func ActualFulfillmentEndedAt(segment ActualFulfillmentSegment, members []CarriedObjectReference) (time.Time, error) {
	if !segment.Established() || len(members) == 0 {
		return time.Time{}, ErrInvalidChargeOccurrence
	}
	var latest time.Time
	for _, member := range members {
		participation, present := segment.ParticipationFor(member)
		if !present || participation.Voided() {
			return time.Time{}, ErrNotAParticipant
		}
		_, _, endedAt, ended := participation.End()
		if !ended {
			return time.Time{}, ErrParticipationNotEnded
		}
		if endedAt.After(latest) {
			latest = endedAt
		}
	}
	return latest, nil
}

// chargeOccurrenceReferencePrefix 是登记派生的发生项引用的类别段。
const chargeOccurrenceReferencePrefix = "TCO"

// firstOccurrenceValidity 是登记形成的首个有效性版本。更正另起版本、回指它（ReviseValidity），不在首登处换号。
const firstOccurrenceValidity = "V1"

// DeriveChargeOccurrenceReference 由（租户、原因、事实依据、成员范围）确定性派生发生项引用（ADR-0207 决定四）。
//
// 派生而不签发：同一件事实被重登时要落回同一个身份，重放才答得出「已存在」，异内容才答得出「冲突」；签发的号每次
// 都新，重登就成了第二项、成本被算两遍。成员按字面排序后进指纹——成员是集合，换个顺序报来不是另一项。各维以 NUL
// 分隔，「a/b」+「c」与「a」+「b/c」不会算成同一个身份。取指纹的前十六字节：引用会进结算请求与信封，长度不随
// 任何一维多长而涨。
func DeriveChargeOccurrenceReference(
	tenant TenantID,
	reason ChargeOccurrenceReason,
	basis OccurrenceBasisReference,
	members []CarriedObjectReference,
) (ChargeOccurrenceReference, error) {
	if !tenant.valid() || !reason.valid() || !basis.valid() || len(members) == 0 {
		return ChargeOccurrenceReference{}, ErrInvalidChargeOccurrence
	}
	dimensions := make([]string, 0, len(members))
	for _, member := range members {
		if !member.valid() {
			return ChargeOccurrenceReference{}, ErrInvalidChargeOccurrence
		}
		dimensions = append(dimensions, member.String())
	}
	slices.Sort(dimensions)
	dimensions = append([]string{tenant.String(), reason.String(), basis.String()}, dimensions...)
	digest := sha256.Sum256([]byte(strings.Join(dimensions, "\x00")))
	return NewChargeOccurrenceReference(chargeOccurrenceReferencePrefix + "-" + hex.EncodeToString(digest[:16]))
}

// FirstOccurrenceValidity 交回登记形成的首个有效性版本。
func FirstOccurrenceValidity() OccurrenceValidityVersion {
	return OccurrenceValidityVersion{requiredValue{value: firstOccurrenceValidity}}
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

// Condition 交回这一项满足发生条件的依据。
func (occurrence TransportChargeOccurrence) Condition() OccurrenceCondition {
	return occurrence.condition
}

// SameContentAs 判两份发生项是不是同一件事：身份同、内容逐件同。登记时同身份异内容答冲突、同内容答已存在
// （ADR-0207 决定四）。成员按集合比，时刻按登记册精度比——读回的那一份经过库面，顺序与精度都可能与刚形成的不同，
// 那不是另一件事。
func (occurrence TransportChargeOccurrence) SameContentAs(other TransportChargeOccurrence) bool {
	if occurrence.tenantID != other.tenantID ||
		occurrence.occurrence != other.occurrence ||
		occurrence.journey != other.journey ||
		occurrence.legalEntity != other.legalEntity ||
		occurrence.provider != other.provider ||
		occurrence.agreement != other.agreement ||
		occurrence.reason != other.reason ||
		occurrence.factBasis != other.factBasis ||
		occurrence.scope != other.scope ||
		occurrence.quantity != other.quantity ||
		occurrence.unit != other.unit ||
		!sameOnRegistry(occurrence.occurredAt, other.occurredAt) ||
		occurrence.validity != other.validity ||
		occurrence.condition != other.condition ||
		occurrence.corrects != other.corrects ||
		occurrence.revisionKind != other.revisionKind ||
		occurrence.revisionBasis != other.revisionBasis ||
		!sameOnRegistry(occurrence.revisedAt, other.revisedAt) {
		return false
	}
	return sameMemberSet(occurrence.members, other.members)
}

func sameMemberSet(left, right []CarriedObjectReference) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[CarriedObjectReference]struct{}, len(left))
	for _, member := range left {
		seen[member] = struct{}{}
	}
	for _, member := range right {
		if _, present := seen[member]; !present {
			return false
		}
	}
	return true
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
	if !kind.valid() || !version.valid() || !basis.valid() || AtRegistryPrecision(revisedAt).IsZero() {
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

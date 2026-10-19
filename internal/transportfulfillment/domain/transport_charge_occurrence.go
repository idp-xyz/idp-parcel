package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	// ErrBookingNotAnswered：订舱还没有定局答复（没有答复，或只有候补）。订舱费的依据是已接受的承运答复，答复还没来就
	// 没有这件事实；等到承运方定局再登。
	ErrBookingNotAnswered = errors.New("transport fulfillment: the booking has no settled carrier answer")
	// ErrBookingNotAccepted：订舱的定局答复不是接受（拒绝、失效、撤回），或取消那一刻还没有接受（接受晚于取消）。没有成约的
	// 舱位，订舱费与取消费都无从谈起。
	ErrBookingNotAccepted = errors.New("transport fulfillment: the booking was not accepted by the carrier")
	// ErrBookingNotCancelled：取消发生项指名的订舱没有取消。
	ErrBookingNotCancelled = errors.New("transport fulfillment: the booking is not cancelled")
	// ErrCancelledAfterTransportStart：按业务时间，这次订舱取消到达时它所属委托下的运输已经开始、这次订舱已在开始之前提出。
	// 取消停的不是尚未开始的范围（AT-TF-092），不形成「尚未执行服务的取消费」；判法与 ADR-0204 决定三「采用与取消的先后」
	// 同一边界。
	ErrCancelledAfterTransportStart = errors.New("transport fulfillment: the booking was cancelled after transport had started")
	// ErrNotACommissionMember：订舱与取消发生项的成员必须是订舱所属委托的成员。订舱自身不列对象，它申请的是委托范围里的
	// 舱位；委托之外的对象不在这次采购里，替它开发生项就是给没采购的运输开成本来源。
	ErrNotACommissionMember = errors.New("transport fulfillment: the object is not a member of the booking's commission")
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

// Source 交回事实依据的出处：依据串第一道斜杠之前的那一词（揽收结果、派送结果、履约段各有一词，见各依据的构造
// 函数）。主要业务范围要连同出处一起认（AttemptOccurrenceScope 头注）；登记册按同一读法从存下的依据串取出处，所以
// 各依据构造函数的前缀不得含斜杠。
func (basis OccurrenceBasisReference) Source() string {
	source, _, _ := strings.Cut(basis.String(), "/")
	return source
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
// 事实条件的明确范围，才能形成运输收费发生项」；ADR-0207 决定三、ADR-0209）。
//
// 两格在册上分得开，审计要问的正是「这一项是谁判的」：登记者断言是协议还没有条件文法时由人判断、留痕的那一段
// （断言期）；按协议声明判是 ADR-0209 之后唯一会新形成的一格，判断出自采用的那一版协议。断言期的那一格只剩读回
// 的用处——册上已有的断言项照旧读得出来，不替它们补一个「按协议判过」。
type OccurrenceConditionBasis uint8

const (
	OccurrenceConditionBasisInvalid OccurrenceConditionBasis = iota
	ConditionAssertedByRegistrant
	ConditionDeclaredByAgreement
)

func (basis OccurrenceConditionBasis) String() string {
	switch basis {
	case ConditionAssertedByRegistrant:
		return "REGISTRANT_ASSERTED"
	case ConditionDeclaredByAgreement:
		return "AGREEMENT_DECLARED"
	default:
		return ""
	}
}

// RegistrantReference 指名做出这一次登记决定的操作者：取自操作者信封的主体，发行方与 sub 成对（ADR-0100 决定二
// 第三条）——sub 只在它的发行方之内唯一，单拿 sub 会让两个发行方的同一个 sub 在留痕里读成一个人。本上下文不解析它，
// 只原样留痕。
type RegistrantReference struct{ requiredValue }

func NewRegistrantReference(value string) (RegistrantReference, error) {
	required, err := newRequiredValue("registrant reference", value)
	return RegistrantReference{required}, err
}

// OccurrenceCondition 是一个发生项满足发生条件的依据，连同做出这一次登记决定的登记者。断言那一格另带原话（引哪一版
// 协议的哪一条、凭什么事实），断言人就是登记者。不是布尔：只记「断言过」，日后复核的人既不知道当时断言的是什么，也
// 不知道是谁断言的。按协议声明判的那一格没有原话——判断出自协议，协议就记在发生项上——但登记者照记：服务方、协议
// 版本与成员仍是这个人报的，审计要找得到人（ADR-0209 决定五）。
type OccurrenceCondition struct {
	basis      OccurrenceConditionBasis
	registrant RegistrantReference
	assertion  requiredValue
}

// NewAgreementDeclaredOccurrenceCondition 立一份「按采用协议的声明判过」的依据，记下登记者。
func NewAgreementDeclaredOccurrenceCondition(registrant RegistrantReference) (OccurrenceCondition, error) {
	if !registrant.valid() {
		return OccurrenceCondition{}, fmt.Errorf("%w: registrant reference", ErrBlankValue)
	}
	return OccurrenceCondition{basis: ConditionDeclaredByAgreement, registrant: registrant}, nil
}

// RegisteredBy 交回做出这一次登记决定的操作者；两格都有。断言那一格上它就是断言人。
func (condition OccurrenceCondition) RegisteredBy() RegistrantReference {
	return condition.registrant
}

// NewAssertedOccurrenceCondition 立一份登记者断言。断言人或原话缺一即拒：没有人为条件作答，就没有发生项可形成。
func NewAssertedOccurrenceCondition(registrant RegistrantReference, assertion string) (OccurrenceCondition, error) {
	if !registrant.valid() {
		return OccurrenceCondition{}, fmt.Errorf("%w: registrant reference", ErrBlankValue)
	}
	statement, err := newRequiredValue("occurrence condition assertion", assertion)
	if err != nil {
		return OccurrenceCondition{}, err
	}
	return OccurrenceCondition{basis: ConditionAssertedByRegistrant, registrant: registrant, assertion: statement}, nil
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

// AssertedBy 交回断言人；只在断言那一格上给出。
func (condition OccurrenceCondition) AssertedBy() (RegistrantReference, bool) {
	if condition.basis != ConditionAssertedByRegistrant {
		return RegistrantReference{}, false
	}
	return condition.registrant, true
}

// sameJudgmentAs 判两份条件判的是不是同一件事：依据与原话同。登记者（断言人）不比——它是这一次登记决定的出处，不是
// 发生项的内容；另一位操作者照原样再报一次，说的仍是册上那一项，答已存在并回显原登记者，而不是冲突。
func (condition OccurrenceCondition) sameJudgmentAs(other OccurrenceCondition) bool {
	return condition.basis == other.basis && condition.assertion == other.assertion
}

func (condition OccurrenceCondition) valid() bool {
	switch condition.basis {
	case ConditionAssertedByRegistrant:
		return condition.registrant.valid() && condition.assertion.valid()
	case ConditionDeclaredByAgreement:
		return condition.registrant.valid() && !condition.assertion.valid()
	default:
		return false
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
	scope, err := AttemptOccurrenceScope(result.Attempt())
	if err != nil {
		return TransportChargeOccurrence{}, err
	}
	spec.Reason = FailedAttemptOccurrence
	spec.FactBasis = basis
	spec.Scope = scope
	spec.OccurredAt = result.OccurredAt()
	return FormTransportChargeOccurrence(spec)
}

// AttemptOccurrenceScope 是失败尝试发生项的主要业务范围：那一次到场。揽收与派送同一写法——范围不进身份，只用来认
// 「这一次到场是不是已经整体计过」（按尝试计费时一次到场只计一次，ADR-0209 决定四）。
//
// 范围串就是尝试身份本身、不带侧别，而两侧的尝试身份各由自己的登记册签发，同一个串可以各指一次到场
// （FailedPickupAttemptBasis 头注）。所以「同一次到场」要连同事实依据的出处一起认（OccurrenceBasisReference.Source）：
// 只比范围串，揽收的 A1 按尝试计过之后，另一家服务方的派送 A1 就登不进去。不把侧别写进范围串，是因为断言期已落册
// 的项范围就是尝试串本身，换写法会让新登记认不出那些项已经按尝试计过。
func AttemptOccurrenceScope(attempt AttemptReference) (OccurrenceScopeReference, error) {
	scope, err := NewOccurrenceScopeReference(attempt.String())
	if err != nil {
		return OccurrenceScopeReference{}, ErrInvalidChargeOccurrence
	}
	return scope, nil
}

// SegmentOccurrenceScope 是实际履约发生项的主要业务范围：那一个实际履约段。
func SegmentOccurrenceScope(segment FulfillmentSegmentReference) (OccurrenceScopeReference, error) {
	scope, err := NewOccurrenceScopeReference(segment.String())
	if err != nil {
		return OccurrenceScopeReference{}, ErrInvalidChargeOccurrence
	}
	return scope, nil
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
	scope, err := AttemptOccurrenceScope(result.Attempt())
	if err != nil {
		return TransportChargeOccurrence{}, err
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
	scope, err := SegmentOccurrenceScope(segment.Segment())
	if err != nil {
		return TransportChargeOccurrence{}, err
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

// BookingChargeFact 是订舱与取消两种原因共用的事实：一次订舱、它所属的委托，以及这次订舱当前的那一份承运答复（可缺）。
// 三者都按身份读回、互相对得上（同租户、订舱属这份委托、答复属这次订舱），由各自的门判；这里只把它们并在一处，订舱与取消
// 两扇门取同一组事实，不各读各的。
type BookingChargeFact struct {
	Booking    BookingRequest
	Commission TransportCommission
	Answer     CarrierAcceptance
	Answered   bool
}

// BookingOccurrenceScope 是订舱与取消发生项的主要业务范围：那一次订舱（ADR-0209 决定二「订舱与取消的范围是一次订舱」）。
// 两种原因同一范围串，覆盖一问按（原因、范围）认，所以一次订舱上的订舱费与取消费互不挡。
func BookingOccurrenceScope(booking BookingReference) (OccurrenceScopeReference, error) {
	scope, err := NewOccurrenceScopeReference(booking.String())
	if err != nil {
		return OccurrenceScopeReference{}, ErrInvalidChargeOccurrence
	}
	return scope, nil
}

// BookingOccurrenceGrounds 判订舱发生项的事实是否成立，交回依据、范围与业务时间：订舱有一份已接受的定局答复，成员都是
// 委托的成员。依据指那一份接受（订舱与答复两段都在串里，从依据就找得回是哪一次订舱的哪一份答复），业务时间取接受的
// 答复时刻——成约在那一刻发生，订舱申请的时刻还没有成约。
func BookingOccurrenceGrounds(
	fact BookingChargeFact,
	members []CarriedObjectReference,
) (OccurrenceBasisReference, OccurrenceScopeReference, time.Time, error) {
	if err := bookingFactCoherent(fact, members); err != nil {
		return OccurrenceBasisReference{}, OccurrenceScopeReference{}, time.Time{}, err
	}
	if err := acceptedBooking(fact); err != nil {
		return OccurrenceBasisReference{}, OccurrenceScopeReference{}, time.Time{}, err
	}
	basis, err := NewOccurrenceBasisReference(
		"CARRIER-ACCEPTANCE/" + fact.Booking.Booking().String() + "/" + fact.Answer.Acceptance().String())
	if err != nil {
		return OccurrenceBasisReference{}, OccurrenceScopeReference{}, time.Time{}, ErrInvalidChargeOccurrence
	}
	scope, err := BookingOccurrenceScope(fact.Booking.Booking())
	if err != nil {
		return OccurrenceBasisReference{}, OccurrenceScopeReference{}, time.Time{}, err
	}
	return basis, scope, fact.Answer.DecidedAt(), nil
}

// CancellationOccurrenceGrounds 判取消发生项的事实是否成立，交回依据、范围与业务时间：订舱已取消、取消之前（不晚于取消时刻）
// 已被承运方接受，且按业务时间取消停的是尚未开始的范围；成员都是委托的成员。
//
// 取消的是订舱，不是委托：协议能给取消声明的数量形只有按订舱与按件（ADR-0209 决定二），整份委托的取消没有一形配得上，
// 委托下的舱位要逐次订舱取消、逐次登记。只认已接受过的订舱：取消费是对一份已成约舱位的毁约，没成约的订舱取消了也没有
// 可停的服务（产品策略，ADR-0207 文末补记）。
//
// 「尚未开始」按业务时间判，与 ADR-0204 决定三同一边界：委托没开始，或这次订舱在委托开始之后才提出（为剩余范围订的），
// 或取消早于委托开始（同一刻算开始在先）。取消订舱口按登记那一刻的委托判过一次，但委托的开始此后可能被更早开始的段前移
// （ADR-0204 决定二）：前移之后取消落在开始之后的，按业务时间那一程已在用这份舱位，不是尚未执行服务的取消。
func CancellationOccurrenceGrounds(
	fact BookingChargeFact,
	members []CarriedObjectReference,
) (OccurrenceBasisReference, OccurrenceScopeReference, time.Time, error) {
	if err := bookingFactCoherent(fact, members); err != nil {
		return OccurrenceBasisReference{}, OccurrenceScopeReference{}, time.Time{}, err
	}
	cancelledAt, cancelled := fact.Booking.Cancelled()
	if !cancelled {
		return OccurrenceBasisReference{}, OccurrenceScopeReference{}, time.Time{}, ErrBookingNotCancelled
	}
	if err := acceptedBooking(fact); err != nil {
		return OccurrenceBasisReference{}, OccurrenceScopeReference{}, time.Time{}, err
	}
	if fact.Answer.DecidedAt().After(cancelledAt) {
		// 按业务时间，取消那一刻这份舱位还没成约：后到的接受接的是一份已被取消的申请，不是被取消毁掉的约。
		return OccurrenceBasisReference{}, OccurrenceScopeReference{}, time.Time{}, ErrBookingNotAccepted
	}
	if _, startedAt, started := fact.Commission.TransportStarted(); started &&
		!fact.Booking.RequestedAt().After(startedAt) && !cancelledAt.Before(startedAt) {
		return OccurrenceBasisReference{}, OccurrenceScopeReference{}, time.Time{}, ErrCancelledAfterTransportStart
	}
	basis, err := NewOccurrenceBasisReference("BOOKING-CANCELLATION/" + fact.Booking.Booking().String())
	if err != nil {
		return OccurrenceBasisReference{}, OccurrenceScopeReference{}, time.Time{}, ErrInvalidChargeOccurrence
	}
	scope, err := BookingOccurrenceScope(fact.Booking.Booking())
	if err != nil {
		return OccurrenceBasisReference{}, OccurrenceScopeReference{}, time.Time{}, err
	}
	return basis, scope, cancelledAt, nil
}

// ChargeOccurrenceForBooking 从已接受的订舱形成订舱发生项（ADR-0207 决定一）。依据、范围与业务时间与 BookingOccurrenceGrounds
// 同一判据，由领域一处给出。
func ChargeOccurrenceForBooking(spec TransportChargeOccurrenceSpec, fact BookingChargeFact) (TransportChargeOccurrence, error) {
	if spec.TenantID != fact.Booking.TenantID() {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	basis, scope, at, err := BookingOccurrenceGrounds(fact, spec.Members)
	if err != nil {
		return TransportChargeOccurrence{}, err
	}
	spec.Reason = BookingOccurrence
	spec.FactBasis = basis
	spec.Scope = scope
	spec.OccurredAt = at
	return FormTransportChargeOccurrence(spec)
}

// ChargeOccurrenceForCancellation 从尚未开始范围的订舱取消形成取消发生项（ADR-0207 决定一）。判据同
// CancellationOccurrenceGrounds。取消不删、不改订舱发生项：两项各是各的事实（UC-TF-004 业务规则「订舱和取消可以分别形成
// 发生项」）。
func ChargeOccurrenceForCancellation(spec TransportChargeOccurrenceSpec, fact BookingChargeFact) (TransportChargeOccurrence, error) {
	if spec.TenantID != fact.Booking.TenantID() {
		return TransportChargeOccurrence{}, ErrInvalidChargeOccurrence
	}
	basis, scope, at, err := CancellationOccurrenceGrounds(fact, spec.Members)
	if err != nil {
		return TransportChargeOccurrence{}, err
	}
	spec.Reason = CancellationOccurrence
	spec.FactBasis = basis
	spec.Scope = scope
	spec.OccurredAt = at
	return FormTransportChargeOccurrence(spec)
}

// bookingFactCoherent 判三件事实彼此对得上、成员成形且都在委托里。对不上是读口交错了东西，不是登记方能改的输入，答
// ErrInvalidChargeOccurrence；成员不在委托里是登记方报错了成员，单独一格。
func bookingFactCoherent(fact BookingChargeFact, members []CarriedObjectReference) error {
	if !fact.Booking.booking.valid() ||
		!fact.Commission.commission.valid() ||
		fact.Booking.tenantID != fact.Commission.tenantID ||
		fact.Booking.commission != fact.Commission.commission ||
		len(members) == 0 {
		return ErrInvalidChargeOccurrence
	}
	if fact.Answered && (fact.Answer.tenantID != fact.Booking.tenantID || fact.Answer.booking != fact.Booking.booking) {
		return ErrInvalidChargeOccurrence
	}
	for _, member := range members {
		if !member.valid() {
			return ErrInvalidChargeOccurrence
		}
		if !slices.Contains(fact.Commission.members, member) {
			return ErrNotACommissionMember
		}
	}
	return nil
}

// acceptedBooking 判订舱的当前答复是不是一份接受：没有答复或只有候补是还没定局，定局而不是接受是没成约。
func acceptedBooking(fact BookingChargeFact) error {
	if !fact.Answered || !fact.Answer.Settles() {
		return ErrBookingNotAnswered
	}
	if !fact.Answer.Binds() {
		return ErrBookingNotAccepted
	}
	return nil
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
		!occurrence.condition.sameJudgmentAs(other.condition) ||
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

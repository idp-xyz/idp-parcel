package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 运输收费发生项的登记用例（ADR-0207；ADR-0098 决定一、二）。
//
// **它是一次显式的运营决定，不在任何物理编排里。** 揽收、派送、交接与有效交付各自照常落册；发生项由持「运输收费
// 发生项登记」授予的操作者另行登记，指名事实本身，由本用例取回事实、核协议、按事实派生身份与业务时间。理由同
// ADR-0098 决定一：把派生塞进必须照常成功的物理登记，就造出一个静默不发生、无人重试的步骤。
//
// 一个用例覆盖各原因，按原因取不同的事实（失败尝试取尝试结果、实际履约取段内已结束的参与）；订舱与取消两种原因的
// 事实依据随票 demo-chain-gaps/28 接上，在那之前如实答未受理，不拿别的事实凑。
//
// 采购上下文由登记方给出、经 party-commercial 核对，采购责任法人与协议快照取自协议版本（ADR-0098 决定二）；缺了
// 答未受理，**绝不答不适用**——不适用只给显式声明自营或实际承运商判断为自营的那两格（ADR-0098 决定三），把「没填」
// 读成「自营」会把一笔真实外部成本静默吞掉。

// ChargeOccurrenceOutcome 是登记结果的封闭集合。
//
// 协议的四个否定格各自成格，不折成未受理：续办各不相同（去 party-commercial 发布或补正文、换一版、取得新协议、
// 改服务方），折成一格登记方只能去猜（ADR-0029）。不适用、未决、未受理三格的恢复动作分别是什么都不用做、重试、
// 改请求，同样不合并。
type ChargeOccurrenceOutcome uint8

const (
	ChargeOccurrenceOutcomeInvalid ChargeOccurrenceOutcome = iota
	ChargeOccurrenceRegistered
	ChargeOccurrenceExisting
	ChargeOccurrenceConflict
	ChargeOccurrenceNotApplicable
	ChargeOccurrenceAgreementNotRegistered
	ChargeOccurrenceAgreementNotEffective
	ChargeOccurrenceAgreementTerminated
	ChargeOccurrenceProviderMismatch
	ChargeOccurrenceUndecided
	ChargeOccurrenceInputNotAccepted
)

func (outcome ChargeOccurrenceOutcome) String() string {
	switch outcome {
	case ChargeOccurrenceRegistered:
		return "REGISTERED"
	case ChargeOccurrenceExisting:
		return "EXISTING"
	case ChargeOccurrenceConflict:
		return "CONFLICT"
	case ChargeOccurrenceNotApplicable:
		return "NOT_APPLICABLE"
	case ChargeOccurrenceAgreementNotRegistered:
		return ports.SupplierAgreementNotRegistered.String()
	case ChargeOccurrenceAgreementNotEffective:
		return ports.SupplierAgreementNotEffective.String()
	case ChargeOccurrenceAgreementTerminated:
		return ports.SupplierAgreementTerminated.String()
	case ChargeOccurrenceProviderMismatch:
		return ports.SupplierAgreementProviderMismatch.String()
	case ChargeOccurrenceUndecided:
		return "UNDECIDED"
	case ChargeOccurrenceInputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	default:
		return ""
	}
}

// ChargeOccurrenceDetail 指名不适用、未决或未受理各自落在哪一格。登记方据它决定下一步：不适用的两格说为什么本来
// 就不该有外部成本，未决的各格说哪一口答不出、重试哪一次，未受理的各格说请求哪里要改。
type ChargeOccurrenceDetail uint8

const (
	ChargeOccurrenceDetailNone ChargeOccurrenceDetail = iota
	// 不适用：两格都是正确答案，不是缺席（ADR-0098 决定三）。
	SelfOperatedDeclared
	ActualCarrierJudgedSelfOperated
	// 未决：依赖读不动，原样重试。
	FactSourceUnavailable
	CarrierJudgmentUnavailable
	SupplierAgreementUnavailable
	OccurrenceRegistryUnavailable
	OccurrenceHandoffUnavailable
	// 未受理：改请求。
	ChargeRequestMalformed
	ReasonNotRegistrable
	ChargeFactNotFound
	AttemptDidNotFail
	ObjectNotParticipating
	ParticipationNotEnded
	ProcurementContextMissing
	SelfOperationContradicted
	ConditionNotAsserted
)

func (detail ChargeOccurrenceDetail) String() string {
	switch detail {
	case SelfOperatedDeclared:
		return "SELF_OPERATED_DECLARED"
	case ActualCarrierJudgedSelfOperated:
		return "ACTUAL_CARRIER_JUDGED_SELF_OPERATED"
	case FactSourceUnavailable:
		return "FACT_SOURCE_UNAVAILABLE"
	case CarrierJudgmentUnavailable:
		return "CARRIER_JUDGMENT_UNAVAILABLE"
	case SupplierAgreementUnavailable:
		return "SUPPLIER_AGREEMENT_UNAVAILABLE"
	case OccurrenceRegistryUnavailable:
		return "OCCURRENCE_REGISTRY_UNAVAILABLE"
	case OccurrenceHandoffUnavailable:
		return "OCCURRENCE_HANDOFF_UNAVAILABLE"
	case ChargeRequestMalformed:
		return "REQUEST_MALFORMED"
	case ReasonNotRegistrable:
		return "REASON_NOT_REGISTRABLE"
	case ChargeFactNotFound:
		return "FACT_NOT_FOUND"
	case AttemptDidNotFail:
		return "ATTEMPT_DID_NOT_FAIL"
	case ObjectNotParticipating:
		return "OBJECT_NOT_PARTICIPATING"
	case ParticipationNotEnded:
		return "PARTICIPATION_NOT_ENDED"
	case ProcurementContextMissing:
		return "PROCUREMENT_CONTEXT_MISSING"
	case SelfOperationContradicted:
		return "SELF_OPERATION_CONTRADICTED"
	case ConditionNotAsserted:
		return "CONDITION_NOT_ASSERTED"
	default:
		return ""
	}
}

// RegisterChargeOccurrenceCommand 携带一次登记的全部输入。
//
// 事实按原因各取一组：失败尝试给揽收尝试来源键或派送尝试身份两者之一、外加恰一个对象；实际履约给段与一个或多个
// 成员。事实依据、主要业务范围与业务时间不在命令里——它们取自事实本身（ADR-0207 决定一），调用方报来的只会是
// 第二个口径。发生项引用与有效性版本同样不在命令里，由事实派生（ADR-0207 决定四）。
//
// 旅程、数量与单位由登记方声明（ADR-0207 决定四、三）：旅程没有本上下文的权威来源，数量形在协议能声明之前由断言
// 一并给出。SelfOperated 为真是一种声明，此时服务方与协议必须留空——两者同时给出是自相矛盾，不替登记方挑一个。
type RegisterChargeOccurrenceCommand struct {
	TenantID domain.TenantID
	Reason   domain.ChargeOccurrenceReason

	PickupAttemptSource string
	DeliveryAttempt     string
	Segment             string
	Objects             []string

	Journey          string
	SelfOperated     bool
	Provider         string
	AgreementObject  string
	AgreementVersion string
	Quantity         int64
	Unit             string

	ConditionAssertion string
}

type RegisterChargeOccurrenceResult struct {
	outcome        ChargeOccurrenceOutcome
	detail         ChargeOccurrenceDetail
	record         ports.ChargeOccurrenceRecord
	hasRecord      bool
	agreedSupplier string
	continuation   string
}

func (result RegisterChargeOccurrenceResult) Outcome() ChargeOccurrenceOutcome { return result.outcome }

func (result RegisterChargeOccurrenceResult) Detail() ChargeOccurrenceDetail { return result.detail }

// Record 在已登记、已存在与冲突三格交回发生项：已登记是这一次落的那一份，后两格是册上原有的那一份（冲突时回显
// 原项，登记方据它看出两次报的哪里不同）。
func (result RegisterChargeOccurrenceResult) Record() (ports.ChargeOccurrenceRecord, bool) {
	return result.record, result.hasRecord
}

// AgreedSupplier 在服务方不符那一格交回这一版协议约定的供应商。
func (result RegisterChargeOccurrenceResult) AgreedSupplier() string { return result.agreedSupplier }

// Continuation 在未决格交回续办引用：同一请求原样重交即可。
func (result RegisterChargeOccurrenceResult) Continuation() string { return result.continuation }

// RegisterChargeOccurrenceDeps 是本用例的依赖。只读口一律取窄契约：本用例只读揽收、派送、段与承运商判断，写的只有
// 发生项册与它的意图。
type RegisterChargeOccurrenceDeps struct {
	PickupAttempts   ports.FailedAttemptSource
	DeliveryAttempts ports.DeliveryAttemptView
	Segments         ports.FulfillmentSegmentView
	CarrierJudgments ports.ActualCarrierJudgmentView
	Agreements       ports.SupplierAgreementSource
	Occurrences      ports.ChargeOccurrenceRegistry
	Handoff          ports.ChargeOccurrenceHandoff
	Clock            ports.Clock
}

type RegisterChargeOccurrenceHandler struct {
	deps RegisterChargeOccurrenceDeps
}

func NewRegisterChargeOccurrenceHandler(deps RegisterChargeOccurrenceDeps) *RegisterChargeOccurrenceHandler {
	return &RegisterChargeOccurrenceHandler{deps: deps}
}

// chargeOccurrenceFact 是按原因取回的事实：依据、成员、业务时间，以及形成发生项的那一扇领域门。段只在实际履约上有，
// 用来问这一段的实际承运商是不是自营。
type chargeOccurrenceFact struct {
	basis      domain.OccurrenceBasisReference
	members    []domain.CarriedObjectReference
	occurredAt time.Time
	segment    domain.FulfillmentSegmentReference
	hasSegment bool
	form       func(domain.TransportChargeOccurrenceSpec) (domain.TransportChargeOccurrence, error)
}

// Register 取事实 → 派生身份 → 判自营 → 核协议 → 形成 → 落册并交意图。
//
// 身份在采购上下文之前派生：声明自营的那一格也要先问册上有没有同一件事已经登成了外包，否则同一件事会同时被答成
// 「有外部成本」与「不适用」。协议按业务时间核——协议在那一刻是否支持采购，后来到期或终止不改写已发生的事实。
func (handler *RegisterChargeOccurrenceHandler) Register(
	ctx context.Context,
	command RegisterChargeOccurrenceCommand,
) (RegisterChargeOccurrenceResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return chargeNotAccepted(ChargeRequestMalformed), nil
	}

	var fact chargeOccurrenceFact
	var answer RegisterChargeOccurrenceResult
	var answered bool
	switch command.Reason {
	case domain.FailedAttemptOccurrence:
		fact, answer, answered = handler.failedAttemptFact(ctx, command)
	case domain.ActualFulfillmentOccurrence:
		fact, answer, answered = handler.actualFulfillmentFact(ctx, command)
	case domain.BookingOccurrence, domain.CancellationOccurrence:
		return chargeNotAccepted(ReasonNotRegistrable), nil
	default:
		return chargeNotAccepted(ChargeRequestMalformed), nil
	}
	if answered {
		return answer, nil
	}

	reference, err := domain.DeriveChargeOccurrenceReference(command.TenantID, command.Reason, fact.basis, fact.members)
	if err != nil {
		return chargeNotAccepted(ChargeRequestMalformed), nil
	}
	key := ports.ChargeOccurrenceKey{
		TenantID:   command.TenantID,
		Occurrence: reference,
		Validity:   domain.FirstOccurrenceValidity(),
	}
	continuation := chargeOccurrenceContinuation(command.TenantID.String(), reference.String())

	procurementGiven := strings.TrimSpace(command.Provider) != "" ||
		strings.TrimSpace(command.AgreementObject) != "" ||
		strings.TrimSpace(command.AgreementVersion) != ""
	if command.SelfOperated {
		if procurementGiven {
			return chargeNotAccepted(SelfOperationContradicted), nil
		}
		return handler.selfOperated(ctx, key, continuation)
	}

	if fact.hasSegment {
		judgedSelf, err := handler.segmentJudgedSelfOperated(ctx, command.TenantID, fact.segment)
		if err != nil {
			return chargeUndecided(CarrierJudgmentUnavailable, continuation), nil
		}
		if judgedSelf {
			return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceNotApplicable, detail: ActualCarrierJudgedSelfOperated}, nil
		}
	}

	if strings.TrimSpace(command.Provider) == "" ||
		strings.TrimSpace(command.AgreementObject) == "" ||
		strings.TrimSpace(command.AgreementVersion) == "" {
		return chargeNotAccepted(ProcurementContextMissing), nil
	}
	if strings.TrimSpace(command.ConditionAssertion) == "" {
		return chargeNotAccepted(ConditionNotAsserted), nil
	}
	spec, agreement, err := chargeOccurrenceSpecFrom(command, key, fact)
	if err != nil {
		return chargeNotAccepted(ChargeRequestMalformed), nil
	}

	check, err := handler.deps.Agreements.CheckSupplierAgreement(ctx, command.TenantID, agreement, spec.Provider, fact.occurredAt)
	if err != nil {
		return chargeUndecided(SupplierAgreementUnavailable, continuation), nil
	}
	switch check.Standing {
	case ports.SupplierAgreementRegistered:
	case ports.SupplierAgreementNotRegistered:
		return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceAgreementNotRegistered}, nil
	case ports.SupplierAgreementNotEffective:
		return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceAgreementNotEffective}, nil
	case ports.SupplierAgreementTerminated:
		return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceAgreementTerminated}, nil
	case ports.SupplierAgreementProviderMismatch:
		return RegisterChargeOccurrenceResult{
			outcome:        ChargeOccurrenceProviderMismatch,
			agreedSupplier: check.Supplier.String(),
		}, nil
	default:
		// 核对口交回集外的一格是适配器缺陷，不是业务答案；重试不会变，响亮上抛。
		return RegisterChargeOccurrenceResult{}, errors.New("register charge occurrence: supplier agreement check answered outside its closed set")
	}
	spec.LegalEntity = check.LegalEntity
	spec.Agreement = check.Snapshot

	occurrence, err := fact.form(spec)
	if err != nil {
		return chargeNotAccepted(ChargeRequestMalformed), nil
	}
	record := ports.ChargeOccurrenceRecord{
		Key:        key,
		Occurrence: occurrence,
		RecordedAt: handler.deps.Clock.Now().UTC(),
	}
	saved, err := handler.deps.Occurrences.Save(ctx, record)
	if err != nil {
		return chargeUndecided(OccurrenceRegistryUnavailable, continuation), nil
	}
	if saved == ports.ChargeOccurrenceAlreadyRegistered {
		return handler.replay(ctx, key, occurrence, continuation)
	}
	if err := handler.deps.Handoff.HandOffChargeOccurrence(ctx, ports.ChargeOccurrenceIntent{Record: record}); err != nil {
		return chargeUndecided(OccurrenceHandoffUnavailable, continuation), nil
	}
	return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceRegistered, record: record, hasRecord: true}, nil
}

// selfOperated 答声明自营的那一格。册上已有同一件事的发生项时答冲突并回显它：同一件事不能既有外部成本又不适用，
// 要改的是先前那一次登记（更正走有效性修订），不是这一次声明。
func (handler *RegisterChargeOccurrenceHandler) selfOperated(
	ctx context.Context,
	key ports.ChargeOccurrenceKey,
	continuation string,
) (RegisterChargeOccurrenceResult, error) {
	existing, found, err := handler.deps.Occurrences.FindByKey(ctx, key)
	if err != nil {
		return chargeUndecided(OccurrenceRegistryUnavailable, continuation), nil
	}
	if found {
		return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceConflict, record: existing, hasRecord: true}, nil
	}
	return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceNotApplicable, detail: SelfOperatedDeclared}, nil
}

// segmentJudgedSelfOperated 问这一段当前的实际承运商判断是不是自营法人（ADR-0098 决定三的第二种读法）。没有判断、
// 判断待确认或判为外部参与方都答否——那几格不说明自营，自营与外包不由判断缺席推断。
func (handler *RegisterChargeOccurrenceHandler) segmentJudgedSelfOperated(
	ctx context.Context,
	tenant domain.TenantID,
	segment domain.FulfillmentSegmentReference,
) (bool, error) {
	record, found, err := handler.deps.CarrierJudgments.FindByKey(ctx, ports.ActualCarrierJudgmentKey{TenantID: tenant, Segment: segment})
	if err != nil || !found {
		return false, err
	}
	subject, identified := record.Judgment.Current().Verdict().Identified()
	return identified && subject.Kind() == domain.OwnOperatingLegalEntity, nil
}

// replay 答撞键的那一次：同内容是已存在，异内容是冲突，两格都回显册上原有的那一份。已存在那一格重交同一份意图：
// 意图按信封 ID 认领（ADR-0043），已入队的那一封不会多出第二封，而先前若有一次交接没落成，这一次把它补上。
func (handler *RegisterChargeOccurrenceHandler) replay(
	ctx context.Context,
	key ports.ChargeOccurrenceKey,
	formed domain.TransportChargeOccurrence,
	continuation string,
) (RegisterChargeOccurrenceResult, error) {
	existing, found, err := handler.deps.Occurrences.FindByKey(ctx, key)
	if err != nil || !found {
		return chargeUndecided(OccurrenceRegistryUnavailable, continuation), nil
	}
	if !existing.Occurrence.SameContentAs(formed) {
		return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceConflict, record: existing, hasRecord: true}, nil
	}
	if err := handler.deps.Handoff.HandOffChargeOccurrence(ctx, ports.ChargeOccurrenceIntent{Record: existing}); err != nil {
		return chargeUndecided(OccurrenceHandoffUnavailable, continuation), nil
	}
	return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceExisting, record: existing, hasRecord: true}, nil
}

// actualFulfillmentFact 取段，判成员的参与是否都已结束。成员重复是坏请求：成员是集合，重复报来说不清数量该算几份。
func (handler *RegisterChargeOccurrenceHandler) actualFulfillmentFact(
	ctx context.Context,
	command RegisterChargeOccurrenceCommand,
) (chargeOccurrenceFact, RegisterChargeOccurrenceResult, bool) {
	if strings.TrimSpace(command.PickupAttemptSource) != "" || strings.TrimSpace(command.DeliveryAttempt) != "" ||
		len(command.Objects) == 0 {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}
	segment, err := domain.NewFulfillmentSegmentReference(command.Segment)
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}
	members, ok := distinctMembers(command.Objects)
	if !ok {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}

	record, found, err := handler.deps.Segments.FindByKey(ctx, ports.FulfillmentSegmentKey{TenantID: command.TenantID, Segment: segment})
	if err != nil {
		return chargeOccurrenceFact{}, chargeUndecided(FactSourceUnavailable,
			chargeOccurrenceContinuation(command.TenantID.String(), command.Reason.String(), segment.String())), true
	}
	if !found {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeFactNotFound), true
	}
	endedAt, err := domain.ActualFulfillmentEndedAt(record.Segment, members)
	switch {
	case errors.Is(err, domain.ErrNotAParticipant):
		return chargeOccurrenceFact{}, chargeNotAccepted(ObjectNotParticipating), true
	case errors.Is(err, domain.ErrParticipationNotEnded):
		return chargeOccurrenceFact{}, chargeNotAccepted(ParticipationNotEnded), true
	case err != nil:
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}
	basis, err := domain.ActualFulfillmentBasis(segment)
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true
	}
	established := record.Segment
	return chargeOccurrenceFact{
		basis:      basis,
		members:    members,
		occurredAt: endedAt,
		segment:    segment,
		hasSegment: true,
		form: func(spec domain.TransportChargeOccurrenceSpec) (domain.TransportChargeOccurrence, error) {
			return domain.ChargeOccurrenceForActualFulfillment(spec, established)
		},
	}, RegisterChargeOccurrenceResult{}, false
}

func distinctMembers(objects []string) ([]domain.CarriedObjectReference, bool) {
	members := make([]domain.CarriedObjectReference, 0, len(objects))
	seen := make(map[domain.CarriedObjectReference]struct{}, len(objects))
	for _, raw := range objects {
		member, err := domain.NewCarriedObjectReference(raw)
		if err != nil {
			return nil, false
		}
		if _, duplicate := seen[member]; duplicate {
			return nil, false
		}
		seen[member] = struct{}{}
		members = append(members, member)
	}
	return members, true
}

// chargeOccurrenceSpecFrom 把命令里登记方声明的几格逐个过构造门，连同派生的身份与事实的成员组成规格。法人与协议
// 快照留空，由协议核对口填；原因、依据、范围与业务时间由领域门从事实取。
func chargeOccurrenceSpecFrom(
	command RegisterChargeOccurrenceCommand,
	key ports.ChargeOccurrenceKey,
	fact chargeOccurrenceFact,
) (domain.TransportChargeOccurrenceSpec, domain.SupplierAgreementVersion, error) {
	none := domain.TransportChargeOccurrenceSpec{}
	spec := domain.TransportChargeOccurrenceSpec{
		TenantID:   command.TenantID,
		Occurrence: key.Occurrence,
		Validity:   key.Validity,
		Members:    fact.members,
		Quantity:   command.Quantity,
	}
	if command.Quantity <= 0 {
		return none, domain.SupplierAgreementVersion{}, domain.ErrInvalidChargeOccurrence
	}
	var err error
	if spec.Journey, err = domain.NewJourneyReference(command.Journey); err != nil {
		return none, domain.SupplierAgreementVersion{}, err
	}
	if spec.Provider, err = domain.NewServiceProviderReference(command.Provider); err != nil {
		return none, domain.SupplierAgreementVersion{}, err
	}
	if spec.Unit, err = domain.NewQuantityUnitReference(command.Unit); err != nil {
		return none, domain.SupplierAgreementVersion{}, err
	}
	if spec.Condition, err = domain.NewAssertedOccurrenceCondition(command.ConditionAssertion); err != nil {
		return none, domain.SupplierAgreementVersion{}, err
	}
	agreement, err := domain.NewSupplierAgreementVersion(command.AgreementObject, command.AgreementVersion)
	if err != nil {
		return none, domain.SupplierAgreementVersion{}, err
	}
	return spec, agreement, nil
}

func chargeNotAccepted(detail ChargeOccurrenceDetail) RegisterChargeOccurrenceResult {
	return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceInputNotAccepted, detail: detail}
}

func chargeUndecided(detail ChargeOccurrenceDetail, continuation string) RegisterChargeOccurrenceResult {
	return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceUndecided, detail: detail, continuation: continuation}
}

func chargeOccurrenceContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
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
// 一个用例覆盖各原因，按原因取不同的事实：失败尝试取尝试结果，实际履约取段内已结束的参与，订舱取已接受的承运答复，
// 取消取尚未开始范围的订舱取消（ADR-0207 决定一与文末补记）。
//
// 采购上下文由登记方给出、经 party-commercial 核对，采购责任法人与协议快照取自协议版本（ADR-0098 决定二）；缺了
// 答未受理，**绝不答不适用**——不适用只给显式声明自营或实际承运商判断为自营的那两格（ADR-0098 决定三），把「没填」
// 读成「自营」会把一笔真实外部成本静默吞掉。事实带着一份委托时例外：订舱与取消属于订舱所属的委托，实际履约段已采用
// 委托时属于那份委托，采购上下文取委托的快照，登记方可以不给，给了而与委托不一致答冲突（ADR-0207 决定二末条）。
//
// 发生条件按那一版协议声明的条件判（ADR-0209）：没声明答`未配置`，声明不计费答「协议不计此原因」，计费的按协议约定
// 的数量形数出数量与单位。登记方不再断言条件、也不再报数量与单位——那是断言期的做法（ADR-0207 决定三），协议能
// 声明之后，由人判就成了与协议并行的第二个口径。

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
	// ChargeOccurrenceNotConfigured：采用的那一版协议没声明这个原因的收费发生条件（ADR-0209）。是租户还没配，不是
	// 依赖读不动：入账、不重投，续办是去 party-commercial 发一版声明了条件的协议（ADR-0209 决定四）。
	ChargeOccurrenceNotConfigured
	// ChargeOccurrenceNotCharged：协议明说这个原因不计费。是正确答案不是缺席，也不是不适用——不适用说本来就没有外部
	// 成本来源（自营），这一格说外部供应商在、协议约定这一种事实不收费；登记方若认为该收，续办是发新版协议。
	ChargeOccurrenceNotCharged
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
	case ChargeOccurrenceNotConfigured:
		return "NOT_CONFIGURED"
	case ChargeOccurrenceNotCharged:
		return "NOT_CHARGED"
	default:
		return ""
	}
}

// ChargeOccurrenceDetail 指名不适用、冲突、未决或未受理各自落在哪一格。登记方据它决定下一步：不适用的两格说为什么
// 本来就不该有外部成本，冲突那一格说撞的不是同一身份而是成员已被另一项计过，未决的各格说哪一口答不出、重试哪一次，
// 未受理的各格说请求哪里要改。
type ChargeOccurrenceDetail uint8

const (
	ChargeOccurrenceDetailNone ChargeOccurrenceDetail = iota
	// 不适用：两格都是正确答案，不是缺席（ADR-0098 决定三）。
	SelfOperatedDeclared
	ActualCarrierJudgedSelfOperated
	// 冲突：同一身份异内容不带细格；成员与同一事实上另一项仍有效的发生项有交集带这一格（ADR-0207 决定四）。
	MembersAlreadyCovered
	// 冲突：这个范围已被一项按范围计的发生项整体计过，或本次按范围计而范围上已有别的项（ADR-0209 决定四）。
	ScopeAlreadyCharged
	// 未配置：协议没声明这个原因。
	ChargeConditionNotDeclared
	// 未决：依赖读不动，原样重试。
	FactSourceUnavailable
	CarrierJudgmentUnavailable
	ChargeSupplierAgreementUnavailable
	OccurrenceRegistryUnavailable
	OccurrenceHandoffUnavailable
	// 未受理：改请求。
	ChargeRequestMalformed
	ChargeFactNotFound
	AttemptDidNotFail
	ObjectNotParticipating
	ParticipationNotEnded
	ProcurementContextMissing
	SelfOperationContradicted
	// 冲突：事实带着一份委托，登记方给的采购上下文（或自营声明）与委托快照不一致；回显那份委托。
	ProcurementContextConflict
	// 未决：采用连线或委托读不动。
	ProcurementSourceUnavailable
	// 未受理：订舱与取消的事实不成立。续办各不相同——还没定局的等承运方答复；没成约的本就没有这笔费；没取消的不是取消；
	// 取消晚于运输开始的不是尚未执行服务的取消；成员不在委托里的改成员。
	BookingNotAnswered
	BookingNotAccepted
	BookingNotCancelled
	CancelledAfterTransportStart
	ObjectNotInCommission
)

func (detail ChargeOccurrenceDetail) String() string {
	switch detail {
	case SelfOperatedDeclared:
		return "SELF_OPERATED_DECLARED"
	case ActualCarrierJudgedSelfOperated:
		return "ACTUAL_CARRIER_JUDGED_SELF_OPERATED"
	case MembersAlreadyCovered:
		return "MEMBERS_ALREADY_COVERED"
	case ScopeAlreadyCharged:
		return "SCOPE_ALREADY_CHARGED"
	case ChargeConditionNotDeclared:
		return "CHARGE_CONDITION_NOT_DECLARED"
	case FactSourceUnavailable:
		return "FACT_SOURCE_UNAVAILABLE"
	case CarrierJudgmentUnavailable:
		return "CARRIER_JUDGMENT_UNAVAILABLE"
	case ChargeSupplierAgreementUnavailable:
		return "SUPPLIER_AGREEMENT_UNAVAILABLE"
	case OccurrenceRegistryUnavailable:
		return "OCCURRENCE_REGISTRY_UNAVAILABLE"
	case OccurrenceHandoffUnavailable:
		return "OCCURRENCE_HANDOFF_UNAVAILABLE"
	case ChargeRequestMalformed:
		return "REQUEST_MALFORMED"
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
	case ProcurementContextConflict:
		return "PROCUREMENT_CONTEXT_CONFLICT"
	case ProcurementSourceUnavailable:
		return "PROCUREMENT_SOURCE_UNAVAILABLE"
	case BookingNotAnswered:
		return "BOOKING_NOT_ANSWERED"
	case BookingNotAccepted:
		return "BOOKING_NOT_ACCEPTED"
	case BookingNotCancelled:
		return "BOOKING_NOT_CANCELLED"
	case CancelledAfterTransportStart:
		return "CANCELLED_AFTER_TRANSPORT_START"
	case ObjectNotInCommission:
		return "OBJECT_NOT_IN_COMMISSION"
	default:
		return ""
	}
}

// RegisterChargeOccurrenceCommand 携带一次登记的全部输入。
//
// 事实按原因各取一组：失败尝试给揽收尝试来源键或派送尝试身份两者之一、外加恰一个对象；实际履约给段与一个或多个
// 成员；订舱与取消给订舱与一个或多个成员（都须是订舱所属委托的成员）。事实依据、主要业务范围与业务时间不在命令里
// ——它们取自事实本身（ADR-0207 决定一），调用方报来的只会是第二个口径。发生项引用与有效性版本同样不在命令里，由事实
// 派生（ADR-0207 决定四）。
//
// 旅程由登记方声明（ADR-0207 决定四）：本上下文没有它的权威来源。数量与单位不在命令里，由采用协议的数量形定
// （ADR-0209 决定四）。SelfOperated 为真是一种声明，此时服务方与协议必须留空——两者同时给出是自相矛盾，不替登记方
// 挑一个。事实带着委托时，服务方与协议可以留空，给了就要与委托一致。
//
// Registrant 与租户一样取自认证结果，不来自载荷：它是做出这一次登记决定的操作者，随发生项记为登记者
// （ADR-0209 决定五）。
type RegisterChargeOccurrenceCommand struct {
	TenantID   domain.TenantID
	Registrant string
	Reason     domain.ChargeOccurrenceReason

	PickupAttemptSource string
	DeliveryAttempt     string
	Segment             string
	Booking             string
	Objects             []string

	Journey          string
	SelfOperated     bool
	Provider         string
	AgreementObject  string
	AgreementVersion string
}

type RegisterChargeOccurrenceResult struct {
	outcome        ChargeOccurrenceOutcome
	detail         ChargeOccurrenceDetail
	record         ports.ChargeOccurrenceRecord
	hasRecord      bool
	agreedSupplier string
	agreement      string
	continuation   string
	commission     ports.TransportCommissionRecord
	hasCommission  bool
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

// Agreement 在未配置与协议不计此原因两格交回作答的那一版协议（「对象/版本」）：登记方据它知道该去改哪一版。
func (result RegisterChargeOccurrenceResult) Agreement() string { return result.agreement }

// Continuation 在未决格交回续办引用：同一请求原样重交即可。
func (result RegisterChargeOccurrenceResult) Continuation() string { return result.continuation }

// Commission 在采购上下文冲突那一格交回事实所属的那份委托：登记方据它看出委托上的服务方与协议是什么，改的是自己报的那几格，
// 或者去查是不是指错了段与订舱。
func (result RegisterChargeOccurrenceResult) Commission() (ports.TransportCommissionRecord, bool) {
	return result.commission, result.hasCommission
}

// RegisterChargeOccurrenceDeps 是本用例的依赖。只读口一律取窄契约：本用例只读揽收、派送、段、承运商判断、采用连线、
// 委托、订舱与承运答复，写的只有发生项册与它的意图。
type RegisterChargeOccurrenceDeps struct {
	PickupAttempts   ports.FailedAttemptSource
	DeliveryAttempts ports.DeliveryAttemptView
	Segments         ports.FulfillmentSegmentView
	CarrierJudgments ports.ActualCarrierJudgmentView
	Adoptions        ports.SegmentCommissionAdoptionView
	Commissions      ports.TransportCommissionView
	Bookings         ports.BookingView
	BookingAnswers   ports.BookingAnswerView
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

// chargeOccurrenceFact 是按原因取回的事实：依据、主要业务范围、成员、业务时间，以及形成发生项的那一扇领域门。范围
// 由领域按事实给出（与形成时同一处），登记前问「这个范围是不是已经计过」要用它。段只在实际履约上有，用来问这一段的
// 实际承运商是不是自营。委托在订舱与取消上总有（订舱属于它），在实际履约上只当段已采用委托时有；有它，采购上下文就
// 取它的快照。
type chargeOccurrenceFact struct {
	basis         domain.OccurrenceBasisReference
	scope         domain.OccurrenceScopeReference
	members       []domain.CarriedObjectReference
	occurredAt    time.Time
	segment       domain.FulfillmentSegmentReference
	hasSegment    bool
	commission    ports.TransportCommissionRecord
	hasCommission bool
	form          func(domain.TransportChargeOccurrenceSpec) (domain.TransportChargeOccurrence, error)
}

// Register 取事实 → 派生身份 → 判委托冲突或自营 → 定采购上下文 → 核协议 → 按协议判条件、数数量 → 形成 → 查覆盖 → 落册
// 并交意图。
//
// 身份在采购上下文之前派生：自营的两格也要先问册上有没有同一件事已经登成了外包，否则同一件事会同时被答成
// 「有外部成本」与「不适用」。协议按业务时间核——协议在那一刻是否支持采购，后来到期或终止不改写已发生的事实；采购上下文
// 取自委托时同样按业务时间核（ADR-0207 文末补记）。
func (handler *RegisterChargeOccurrenceHandler) Register(
	ctx context.Context,
	command RegisterChargeOccurrenceCommand,
) (RegisterChargeOccurrenceResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" || strings.TrimSpace(command.Registrant) == "" {
		return chargeNotAccepted(ChargeRequestMalformed), nil
	}

	var fact chargeOccurrenceFact
	var answer RegisterChargeOccurrenceResult
	var answered bool
	var err error
	switch command.Reason {
	case domain.FailedAttemptOccurrence:
		fact, answer, answered = handler.failedAttemptFact(ctx, command)
	case domain.ActualFulfillmentOccurrence:
		fact, answer, answered, err = handler.actualFulfillmentFact(ctx, command)
	case domain.BookingOccurrence, domain.CancellationOccurrence:
		fact, answer, answered, err = handler.bookingFact(ctx, command)
	default:
		return chargeNotAccepted(ChargeRequestMalformed), nil
	}
	if err != nil {
		return RegisterChargeOccurrenceResult{}, err
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
	coverage := ports.ChargeOccurrenceCoverageQuery{
		TenantID:   command.TenantID,
		Reason:     command.Reason,
		Scope:      fact.scope,
		FactBasis:  fact.basis,
		Members:    fact.members,
		Except:     reference,
		ScopeUnits: domain.ScopeCountingUnits(),
	}

	procurementGiven := strings.TrimSpace(command.Provider) != "" ||
		strings.TrimSpace(command.AgreementObject) != "" ||
		strings.TrimSpace(command.AgreementVersion) != ""
	if fact.hasCommission {
		// 委托说这件事外包给了它的服务方：声明自营、或报来另一家服务方与另一版协议，都与它相抵。不替任何一方改，摆出委托交
		// 登记方处置；判在承运商判断之前——登记方自己报错了，先告诉他。
		if declaredAgainstCommission(command, fact.commission.Commission) {
			return procurementConflict(fact.commission), nil
		}
	} else if command.SelfOperated {
		if procurementGiven {
			return chargeNotAccepted(SelfOperationContradicted), nil
		}
		return handler.selfOperated(ctx, key, coverage, continuation, SelfOperatedDeclared)
	}

	// 段已采用委托时不问承运商判断：成本来源跟着采购走，不跟着谁实际承运走——签约服务商可以转包，实际承运商与它不同不改
	// 这一段是按那份委托采购的（CONTEXT「签约服务商不因签约或接受运输委托而自动成为实际承运商」，反过来同理）。
	if fact.hasSegment && !fact.hasCommission {
		judgedSelf, err := handler.segmentJudgedSelfOperated(ctx, command.TenantID, fact.segment)
		if err != nil {
			return chargeUndecided(CarrierJudgmentUnavailable, continuation), nil
		}
		if judgedSelf {
			return handler.selfOperated(ctx, key, coverage, continuation, ActualCarrierJudgedSelfOperated)
		}
	}

	provider, agreement, missing, err := procurementOf(command, fact)
	if missing {
		return chargeNotAccepted(ProcurementContextMissing), nil
	}
	if err != nil {
		return chargeNotAccepted(ChargeRequestMalformed), nil
	}
	spec, registrant, err := chargeOccurrenceSpecFrom(command, key, fact)
	if err != nil {
		return chargeNotAccepted(ChargeRequestMalformed), nil
	}
	spec.Provider = provider

	check, err := handler.deps.Agreements.CheckSupplierAgreement(ctx, command.TenantID, agreement, spec.Provider, fact.occurredAt)
	if err != nil {
		return chargeUndecided(ChargeSupplierAgreementUnavailable, continuation), nil
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
	if fact.hasCommission {
		commission := fact.commission.Commission
		if check.Snapshot != commission.Agreement() {
			// 委托记着它那一版协议时，问的就是那一版，PC 拼回的快照只能与委托上的同一串；不同是两边数据走岔了，重试不会变，
			// 响亮上抛。委托没记下（迁移 0038 之前提交的）时问的是登记方给的那一版，快照不同就是登记方报的与委托采用的
			// 不是同一版，答冲突。
			if _, recorded := commission.AgreementVersion(); recorded {
				return RegisterChargeOccurrenceResult{}, fmt.Errorf(
					"register charge occurrence: the agreement check answered snapshot %s for commission %s, which adopted %s",
					check.Snapshot, commission.Commission(), commission.Agreement())
			}
			return procurementConflict(fact.commission), nil
		}
		// 法人与快照取委托上的那一份（ADR-0204 决定一：服务方、协议快照与采购责任法人的权威是委托那一行）；PC 这一问只答
		// 那一版在业务时间是否支持采购、声明了哪些条件。
		spec.LegalEntity = commission.LegalEntity()
		spec.Agreement = commission.Agreement()
	}

	// 条件按这一版协议的声明判（ADR-0209 决定四）。没声明与不计费都不落册、不问册：两格说的是这一版协议怎么约定，
	// 与册上有没有别的项无关。
	declared, present := check.ChargeConditions.For(command.Reason)
	if !present {
		return RegisterChargeOccurrenceResult{
			outcome: ChargeOccurrenceNotConfigured, detail: ChargeConditionNotDeclared, agreement: check.Snapshot.String(),
		}, nil
	}
	form, charged := declared.QuantityForm()
	if !charged {
		return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceNotCharged, agreement: check.Snapshot.String()}, nil
	}
	if spec.Quantity, spec.Unit, err = form.Measure(fact.members); err != nil {
		return RegisterChargeOccurrenceResult{}, fmt.Errorf("register charge occurrence: measuring by %s: %w", form, err)
	}
	if spec.Condition, err = domain.NewAgreementDeclaredOccurrenceCondition(registrant); err != nil {
		return RegisterChargeOccurrenceResult{}, fmt.Errorf("register charge occurrence: %w", err)
	}
	coverage.WholeScope = form.CountsTheScope()

	// 请求能出错的每一格都已在协议核对之前判过：登记方声明的几格过了构造门，事实与成员过了事实那一半。走到这里
	// 还形成不了，只剩核对口交回了不成形的法人或快照、或事实源自相矛盾，都是适配器缺陷；答未受理会让登记方去改一份
	// 本来没错的请求，重试也不会变，所以响亮上抛，由装配点回滚整笔。
	occurrence, err := fact.form(spec)
	if err != nil {
		return RegisterChargeOccurrenceResult{}, fmt.Errorf("register charge occurrence: forming after the agreement check: %w", err)
	}
	if covering, covered, err := handler.deps.Occurrences.FindCovering(ctx, coverage); err != nil {
		return chargeUndecided(OccurrenceRegistryUnavailable, continuation), nil
	} else if covered {
		return alreadyCovered(covering, coverage), nil
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

// selfOperated 答自营的两格（声明自营、实际承运商判为自营）。册上已有同一件事的发生项（同一身份，或同一事实上含
// 同一成员的另一项）时答冲突并回显它：同一件事不能既有外部成本又不适用，要改的是先前那一次登记（更正走有效性
// 修订），不是这一次答复。判断那一格同样先问册：承运商判断可以在登记之后才改判成自营法人，那时直接答不适用，就把
// 册上那一份外部成本静默地说成了没有。
//
// 交集一问在前、按身份查在后，次序不能倒：交集一问先取（租户、原因、主要业务范围）的串行锁。另一笔事务正在登记同一
// 身份的外部发生项、握着这把锁而行还没提交时，先按身份查看不见那一行，等到锁之后的交集一问又按身份把它排除，两问
// 都落空，自营就答成不适用，而册上随即有了同一件事的外部成本。锁之后再按身份查，读到的是对方已提交的那一行。
func (handler *RegisterChargeOccurrenceHandler) selfOperated(
	ctx context.Context,
	key ports.ChargeOccurrenceKey,
	coverage ports.ChargeOccurrenceCoverageQuery,
	continuation string,
	detail ChargeOccurrenceDetail,
) (RegisterChargeOccurrenceResult, error) {
	covering, covered, err := handler.deps.Occurrences.FindCovering(ctx, coverage)
	if err != nil {
		return chargeUndecided(OccurrenceRegistryUnavailable, continuation), nil
	}
	if covered {
		return alreadyCovered(covering, coverage), nil
	}
	existing, found, err := handler.deps.Occurrences.FindByKey(ctx, key)
	if err != nil {
		return chargeUndecided(OccurrenceRegistryUnavailable, continuation), nil
	}
	if found {
		return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceConflict, record: existing, hasRecord: true}, nil
	}
	return RegisterChargeOccurrenceResult{outcome: ChargeOccurrenceNotApplicable, detail: detail}, nil
}

// alreadyCovered 答这件事已被另一项仍有效的发生项计过：那一项回显，登记方要改的是它（失效之后再登），不是这一次
// 换个成员组合重报——身份按成员集合派生，组合一换就是另一项，同一成员或同一范围的成本便被算了两遍。细格说是哪一种：
// 同一事实上成员有交集，还是范围已整体计过（或本次按范围计、范围上已有别的项）。
func alreadyCovered(covering ports.ChargeOccurrenceRecord, query ports.ChargeOccurrenceCoverageQuery) RegisterChargeOccurrenceResult {
	detail := ScopeAlreadyCharged
	if covering.Occurrence.FactBasis() == query.FactBasis && sharesAMember(covering.Occurrence.Members(), query.Members) {
		detail = MembersAlreadyCovered
	}
	return RegisterChargeOccurrenceResult{
		outcome:   ChargeOccurrenceConflict,
		detail:    detail,
		record:    covering,
		hasRecord: true,
	}
}

func sharesAMember(left, right []domain.CarriedObjectReference) bool {
	for _, member := range left {
		if slices.Contains(right, member) {
			return true
		}
	}
	return false
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

// actualFulfillmentFact 取段，判成员的参与是否都已结束，再按段读采用连线。成员重复是坏请求：成员是集合，重复报来说不清
// 数量该算几份。
func (handler *RegisterChargeOccurrenceHandler) actualFulfillmentFact(
	ctx context.Context,
	command RegisterChargeOccurrenceCommand,
) (chargeOccurrenceFact, RegisterChargeOccurrenceResult, bool, error) {
	if strings.TrimSpace(command.PickupAttemptSource) != "" || strings.TrimSpace(command.DeliveryAttempt) != "" ||
		strings.TrimSpace(command.Booking) != "" || len(command.Objects) == 0 {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true, nil
	}
	segment, err := domain.NewFulfillmentSegmentReference(command.Segment)
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true, nil
	}
	members, ok := distinctMembers(command.Objects)
	if !ok {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true, nil
	}

	continuation := chargeOccurrenceContinuation(command.TenantID.String(), command.Reason.String(), segment.String())
	record, found, err := handler.deps.Segments.FindByKey(ctx, ports.FulfillmentSegmentKey{TenantID: command.TenantID, Segment: segment})
	if err != nil {
		return chargeOccurrenceFact{}, chargeUndecided(FactSourceUnavailable, continuation), true, nil
	}
	if !found {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeFactNotFound), true, nil
	}
	endedAt, err := domain.ActualFulfillmentEndedAt(record.Segment, members)
	switch {
	case errors.Is(err, domain.ErrNotAParticipant):
		return chargeOccurrenceFact{}, chargeNotAccepted(ObjectNotParticipating), true, nil
	case errors.Is(err, domain.ErrParticipationNotEnded):
		return chargeOccurrenceFact{}, chargeNotAccepted(ParticipationNotEnded), true, nil
	case err != nil:
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true, nil
	}
	basis, err := domain.ActualFulfillmentBasis(segment)
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true, nil
	}
	scope, err := domain.SegmentOccurrenceScope(segment)
	if err != nil {
		return chargeOccurrenceFact{}, chargeNotAccepted(ChargeRequestMalformed), true, nil
	}
	commission, adopted, err := handler.commissionAdoptedBy(ctx, command.TenantID, segment)
	if errors.Is(err, errProcurementSourceUnreadable) {
		return chargeOccurrenceFact{}, chargeUndecided(ProcurementSourceUnavailable, continuation), true, nil
	}
	if err != nil {
		return chargeOccurrenceFact{}, RegisterChargeOccurrenceResult{}, false, err
	}
	established := record.Segment
	return chargeOccurrenceFact{
		basis:         basis,
		scope:         scope,
		members:       members,
		occurredAt:    endedAt,
		segment:       segment,
		hasSegment:    true,
		commission:    commission,
		hasCommission: adopted,
		form: func(spec domain.TransportChargeOccurrenceSpec) (domain.TransportChargeOccurrence, error) {
			return domain.ChargeOccurrenceForActualFulfillment(spec, established)
		},
	}, RegisterChargeOccurrenceResult{}, false, nil
}

// errProcurementSourceUnreadable 只在本包流转：采用连线或委托读不动，由调用方答未决。
var errProcurementSourceUnreadable = errors.New("register charge occurrence: adoption or commission unreadable")

// ErrAdoptionWithoutCommission 说明段的采用连线指名的委托读不回来。连线落册时委托在册（迁移 0035 的外键），委托又不删，读不回
// 就是库里的数据违约；答「没有采用」会让这件外包的事改按登记方报的采购上下文登，答未受理会让登记方去改一份本来没错的请求。
var ErrAdoptionWithoutCommission = errors.New("transport fulfillment: segment adoption names a commission that is not registered")

// commissionAdoptedBy 按段读采用连线，有就读那份委托（ADR-0204 决定二末条）。没有连线答 adopted=false：没有连线的段不读作
// 自营也不读作外包（ADR-0204 决定四），采购上下文照旧由登记方给。
func (handler *RegisterChargeOccurrenceHandler) commissionAdoptedBy(
	ctx context.Context,
	tenant domain.TenantID,
	segment domain.FulfillmentSegmentReference,
) (ports.TransportCommissionRecord, bool, error) {
	adoption, adopted, err := handler.deps.Adoptions.FindBySegment(ctx, ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: segment})
	if err != nil {
		return ports.TransportCommissionRecord{}, false, fmt.Errorf("%w: %v", errProcurementSourceUnreadable, err)
	}
	if !adopted {
		return ports.TransportCommissionRecord{}, false, nil
	}
	commissionRef := adoption.Adoption.Commission()
	commission, found, err := handler.deps.Commissions.FindByKey(ctx, ports.TransportCommissionKey{TenantID: tenant, Commission: commissionRef})
	if err != nil {
		return ports.TransportCommissionRecord{}, false, fmt.Errorf("%w: %v", errProcurementSourceUnreadable, err)
	}
	if !found {
		return ports.TransportCommissionRecord{}, false, fmt.Errorf("%w: segment %s, commission %s", ErrAdoptionWithoutCommission, segment, commissionRef)
	}
	return commission, true, nil
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

// chargeOccurrenceSpecFrom 把命令里登记方声明的旅程与认证给的登记者过构造门，连同派生的身份与事实的成员组成规格。服务方
// 由 procurementOf 定；法人与协议快照留空，由协议核对口或委托填；数量、单位与发生条件留空，由协议声明的条件定；原因、
// 依据、范围与业务时间由领域门从事实取。
func chargeOccurrenceSpecFrom(
	command RegisterChargeOccurrenceCommand,
	key ports.ChargeOccurrenceKey,
	fact chargeOccurrenceFact,
) (domain.TransportChargeOccurrenceSpec, domain.RegistrantReference, error) {
	spec := domain.TransportChargeOccurrenceSpec{
		TenantID:   command.TenantID,
		Occurrence: key.Occurrence,
		Validity:   key.Validity,
		Members:    fact.members,
	}
	var err error
	if spec.Journey, err = domain.NewJourneyReference(command.Journey); err != nil {
		return domain.TransportChargeOccurrenceSpec{}, domain.RegistrantReference{}, err
	}
	registrant, err := domain.NewRegistrantReference(command.Registrant)
	if err != nil {
		return domain.TransportChargeOccurrenceSpec{}, domain.RegistrantReference{}, err
	}
	return spec, registrant, nil
}

// procurementOf 定这一次登记问协议的服务方与那一版协议。missing 为真答未受理「缺采购上下文」，err 非空答坏请求。
//
// 事实带着委托时，服务方取委托的，协议取委托提交时记下的那一版；委托没记下（迁移 0038 之前提交的）就要登记方给出协议
// 对象与版本，核过之后按快照与委托比（Register 里那一步）。服务方与协议不从快照串里拆：快照是 PC 一处拼出的指称
// （domain.SupplierAgreementVersion 头注）。与委托不一致的报值在此之前已答冲突，走到这里给了的都与委托相同。
//
// 事实不带委托时三格都由登记方给，缺一格答缺采购上下文（ADR-0207 决定二）。
func procurementOf(
	command RegisterChargeOccurrenceCommand,
	fact chargeOccurrenceFact,
) (domain.ServiceProviderReference, domain.SupplierAgreementVersion, bool, error) {
	if fact.hasCommission {
		commission := fact.commission.Commission
		if version, recorded := commission.AgreementVersion(); recorded {
			return commission.Provider(), version, false, nil
		}
		if strings.TrimSpace(command.AgreementObject) == "" || strings.TrimSpace(command.AgreementVersion) == "" {
			return domain.ServiceProviderReference{}, domain.SupplierAgreementVersion{}, true, nil
		}
		agreement, err := domain.NewSupplierAgreementVersion(command.AgreementObject, command.AgreementVersion)
		return commission.Provider(), agreement, false, err
	}
	if strings.TrimSpace(command.Provider) == "" ||
		strings.TrimSpace(command.AgreementObject) == "" ||
		strings.TrimSpace(command.AgreementVersion) == "" {
		return domain.ServiceProviderReference{}, domain.SupplierAgreementVersion{}, true, nil
	}
	provider, err := domain.NewServiceProviderReference(command.Provider)
	if err != nil {
		return domain.ServiceProviderReference{}, domain.SupplierAgreementVersion{}, false, err
	}
	agreement, err := domain.NewSupplierAgreementVersion(command.AgreementObject, command.AgreementVersion)
	return provider, agreement, false, err
}

// declaredAgainstCommission 答登记方报来的采购上下文是否与委托相抵：声明自营，或给了服务方、协议对象、版本标签之一而与委托
// 上的不是同一串。留空不算相抵——委托在，留空就是取委托的。按同一串字面比，不比名称、不归一（ADR-0202 决定四同一条
// 纪律）。委托没记下那一版的两格时，协议只能在核过之后按快照比，这里只比服务方。
func declaredAgainstCommission(command RegisterChargeOccurrenceCommand, commission domain.TransportCommission) bool {
	if command.SelfOperated {
		return true
	}
	if differs(command.Provider, commission.Provider().String()) {
		return true
	}
	if version, recorded := commission.AgreementVersion(); recorded {
		return differs(command.AgreementObject, version.Object()) || differs(command.AgreementVersion, version.Version())
	}
	return false
}

func differs(declared, recorded string) bool {
	return strings.TrimSpace(declared) != "" && declared != recorded
}

func procurementConflict(commission ports.TransportCommissionRecord) RegisterChargeOccurrenceResult {
	return RegisterChargeOccurrenceResult{
		outcome:       ChargeOccurrenceConflict,
		detail:        ProcurementContextConflict,
		commission:    commission,
		hasCommission: true,
	}
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

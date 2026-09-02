package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 失败尝试费发生项的登记用例（ADR-0098）。
//
// **它不在揽收编排里。** ADR-0098 决定一给了两条理由：CONTEXT 禁「任一对象的成功、失败或
// 取消被压缩成其他对象的状态」；而揽收登记必须在采购上下文不可得时照常成功，把这一步塞进去
// 就会造出一个静默不发生且无人重试的派生步骤。
//
// 采购上下文由调用方显式给出，本上下文不推导——自营履约没有协议快照，那不是"读不到"而是
// "不该有"（ADR-0098 决定二）。

// FailedAttemptChargeOutcome 是登记结果的封闭集合。
//
// `不适用`独立于`未决`与`输入未受理`：三者的恢复动作分别是**什么都不用做**、**重试**、
// **改请求**。压成一格就等于让调用方去猜该做哪一件。
type FailedAttemptChargeOutcome uint8

const (
	FailedAttemptChargeOutcomeInvalid FailedAttemptChargeOutcome = iota
	FailedAttemptChargeRegistered
	FailedAttemptChargeExisting
	FailedAttemptChargeNotApplicable
	FailedAttemptChargeUndecided
	FailedAttemptChargeInputNotAccepted
)

func (outcome FailedAttemptChargeOutcome) String() string {
	switch outcome {
	case FailedAttemptChargeRegistered:
		return "CHARGE_REGISTERED"
	case FailedAttemptChargeExisting:
		return "CHARGE_EXISTING"
	case FailedAttemptChargeNotApplicable:
		return "NOT_APPLICABLE"
	case FailedAttemptChargeUndecided:
		return "CHARGE_UNDECIDED"
	case FailedAttemptChargeInputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	default:
		return ""
	}
}

// FailedAttemptChargeReason 指名`不适用`或`未决`各自落在哪一格。
type FailedAttemptChargeReason uint8

const (
	FailedAttemptChargeReasonNone FailedAttemptChargeReason = iota
	// SelfOperatedHasNoExternalCostSource 是 ADR-0098 决定三那一格。**它是正确答案不是
	// 缺席**：自营履约不虚构外部供应商与协议，因而没有外部成本来源可言。
	SelfOperatedHasNoExternalCostSource
	AttemptObjectDidNotFail
	AttemptStoreUnavailable
	ChargeRegistryUnavailable
)

func (reason FailedAttemptChargeReason) String() string {
	switch reason {
	case SelfOperatedHasNoExternalCostSource:
		return "SELF_OPERATED_NO_EXTERNAL_COST_SOURCE"
	case AttemptObjectDidNotFail:
		return "ATTEMPT_DID_NOT_FAIL"
	case AttemptStoreUnavailable:
		return "ATTEMPT_STORE_UNAVAILABLE"
	case ChargeRegistryUnavailable:
		return "CHARGE_REGISTRY_UNAVAILABLE"
	default:
		return ""
	}
}

// RegisterFailedAttemptChargeCommand 携带一次登记的全部输入。
//
// `Agreement` 留空表示自营履约——**那是一种声明，不是漏填**。用例据此答`不适用`，而不是
// 把它当成缺件去拒。
type RegisterFailedAttemptChargeCommand struct {
	TenantID domain.TenantID
	SourceID string
	Object   string

	Occurrence  string
	Journey     string
	LegalEntity string
	Provider    string
	Agreement   string
	Scope       string
	Quantity    int64
	Unit        string
	Validity    string
}

type RegisterFailedAttemptChargeResult struct {
	outcome      FailedAttemptChargeOutcome
	reason       FailedAttemptChargeReason
	record       ports.ChargeOccurrenceRecord
	hasRecord    bool
	continuation string
}

func (result RegisterFailedAttemptChargeResult) Outcome() FailedAttemptChargeOutcome {
	return result.outcome
}

func (result RegisterFailedAttemptChargeResult) Reason() FailedAttemptChargeReason {
	return result.reason
}

func (result RegisterFailedAttemptChargeResult) Record() (ports.ChargeOccurrenceRecord, bool) {
	return result.record, result.hasRecord
}

func (result RegisterFailedAttemptChargeResult) Continuation() string {
	return result.continuation
}

// FailedAttemptChargeDeps 是本用例的依赖。
type FailedAttemptChargeDeps struct {
	Attempts    ports.FailedAttemptSource
	Occurrences ports.ChargeOccurrenceRegistry
	Clock       ports.Clock
}

type RegisterFailedAttemptChargeHandler struct {
	deps FailedAttemptChargeDeps
}

func NewRegisterFailedAttemptChargeHandler(deps FailedAttemptChargeDeps) *RegisterFailedAttemptChargeHandler {
	return &RegisterFailedAttemptChargeHandler{deps: deps}
}

// Register 按尝试来源取回对象结果，判失败，再按调用方给的采购上下文形成发生项。
//
// 业务时间与事实依据取自对象结果本身而不是命令或时钟——`ChargeOccurrenceForFailedAttempt`
// 在领域里就是这么定的，编排不覆盖它。
func (handler *RegisterFailedAttemptChargeHandler) Register(
	ctx context.Context,
	command RegisterFailedAttemptChargeCommand,
) (RegisterFailedAttemptChargeResult, error) {
	// 最小身份先于任何权威读取。
	if strings.TrimSpace(command.TenantID.String()) == "" || strings.TrimSpace(command.SourceID) == "" {
		return notAcceptedCharge(), nil
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return notAcceptedCharge(), nil
	}

	record, found, err := handler.deps.Attempts.FindByKey(ctx, ports.PickupAttemptKey{
		TenantID: command.TenantID,
		SourceID: command.SourceID,
	})
	if err != nil {
		return RegisterFailedAttemptChargeResult{
			outcome:      FailedAttemptChargeUndecided,
			reason:       AttemptStoreUnavailable,
			continuation: failedAttemptChargeContinuation(command.TenantID.String(), command.SourceID, command.Object),
		}, nil
	}
	if !found {
		return notAcceptedCharge(), nil
	}

	result, present := attemptResultFor(record, object)
	if !present {
		// 「这个对象不在这次尝试里」与「读不回来」分开：前者回去查是不是问错了对象，
		// 后者重试同一次调用。
		return notAcceptedCharge(), nil
	}
	if !result.Outcome().Failed() {
		return RegisterFailedAttemptChargeResult{
			outcome: FailedAttemptChargeNotApplicable,
			reason:  AttemptObjectDidNotFail,
		}, nil
	}

	// ADR-0098 决定三：自营履约无外部成本来源，这一格是正确答案。放在形成之前判，
	// 因为构造门会把它报成「缺协议快照」——那是同一件事的错误措辞。
	if strings.TrimSpace(command.Agreement) == "" {
		return RegisterFailedAttemptChargeResult{
			outcome: FailedAttemptChargeNotApplicable,
			reason:  SelfOperatedHasNoExternalCostSource,
		}, nil
	}

	spec, err := chargeSpecFrom(command, result.Object())
	if err != nil {
		return notAcceptedCharge(), nil
	}
	occurrence, err := domain.ChargeOccurrenceForFailedAttempt(spec, result)
	if err != nil {
		return notAcceptedCharge(), nil
	}

	stored := ports.ChargeOccurrenceRecord{
		Key: ports.ChargeOccurrenceKey{
			TenantID:   occurrence.TenantID(),
			Occurrence: occurrence.Occurrence(),
			Validity:   occurrence.Validity(),
		},
		Occurrence: occurrence,
		RecordedAt: handler.deps.Clock.Now().UTC(),
	}
	outcome, err := handler.deps.Occurrences.Save(ctx, stored)
	if err != nil {
		return RegisterFailedAttemptChargeResult{
			outcome:      FailedAttemptChargeUndecided,
			reason:       ChargeRegistryUnavailable,
			continuation: failedAttemptChargeContinuation(command.TenantID.String(), command.Occurrence, command.Validity),
		}, nil
	}
	if outcome == ports.ChargeOccurrenceAlreadyRegistered {
		return RegisterFailedAttemptChargeResult{outcome: FailedAttemptChargeExisting}, nil
	}
	return RegisterFailedAttemptChargeResult{
		outcome:   FailedAttemptChargeRegistered,
		record:    stored,
		hasRecord: true,
	}, nil
}

func attemptResultFor(
	record ports.PickupAttemptRecord,
	object domain.CarriedObjectReference,
) (domain.AttemptObjectResult, bool) {
	for _, result := range record.Results {
		if result.Object() == object {
			return result, true
		}
	}
	return domain.AttemptObjectResult{}, false
}

// chargeSpecFrom 把命令里的采购上下文逐个过构造门。原因、事实依据与业务时间不在这里设置
// ——它们由 ChargeOccurrenceForFailedAttempt 从对象结果取，编排给什么都会被它覆盖。
func chargeSpecFrom(
	command RegisterFailedAttemptChargeCommand,
	object domain.CarriedObjectReference,
) (domain.TransportChargeOccurrenceSpec, error) {
	spec := domain.TransportChargeOccurrenceSpec{
		TenantID: command.TenantID,
		Members:  []domain.CarriedObjectReference{object},
		Quantity: command.Quantity,
	}
	var err error
	if spec.Occurrence, err = domain.NewChargeOccurrenceReference(command.Occurrence); err != nil {
		return domain.TransportChargeOccurrenceSpec{}, err
	}
	if spec.Journey, err = domain.NewJourneyReference(command.Journey); err != nil {
		return domain.TransportChargeOccurrenceSpec{}, err
	}
	if spec.LegalEntity, err = domain.NewProcurementLegalEntityReference(command.LegalEntity); err != nil {
		return domain.TransportChargeOccurrenceSpec{}, err
	}
	if spec.Provider, err = domain.NewServiceProviderReference(command.Provider); err != nil {
		return domain.TransportChargeOccurrenceSpec{}, err
	}
	if spec.Agreement, err = domain.NewAgreementSnapshotReference(command.Agreement); err != nil {
		return domain.TransportChargeOccurrenceSpec{}, err
	}
	if spec.Scope, err = domain.NewOccurrenceScopeReference(command.Scope); err != nil {
		return domain.TransportChargeOccurrenceSpec{}, err
	}
	if spec.Unit, err = domain.NewQuantityUnitReference(command.Unit); err != nil {
		return domain.TransportChargeOccurrenceSpec{}, err
	}
	if spec.Validity, err = domain.NewOccurrenceValidityVersion(command.Validity); err != nil {
		return domain.TransportChargeOccurrenceSpec{}, err
	}
	return spec, nil
}

func notAcceptedCharge() RegisterFailedAttemptChargeResult {
	return RegisterFailedAttemptChargeResult{outcome: FailedAttemptChargeInputNotAccepted}
}

func failedAttemptChargeContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

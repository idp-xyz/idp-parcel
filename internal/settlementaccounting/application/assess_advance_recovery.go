package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	// ErrUnexpectedAssessmentSave 说明评估库交回了封闭集合以外的写入结果。
	ErrUnexpectedAssessmentSave = errors.New("settlement accounting: unexpected advance assessment save outcome")
	// ErrUnexpectedRecoverySave 说明回收库交回了封闭集合以外的写入结果。
	ErrUnexpectedRecoverySave = errors.New("settlement accounting: unexpected advance recovery save outcome")
)

// AdvanceOutcome 是代垫评估与客户回收编排的应用处理结果。`代垫未成立`是业务负向格
// ——非成立判断上形成不了回收（领域哨兵 ErrAdvanceNotEstablished），恢复动作是等
// 新证据重评，不是改单也不是等实例参数（ADR-0029）。
type AdvanceOutcome uint8

const (
	AdvanceOutcomeInvalid AdvanceOutcome = iota
	AdvanceAssessed
	AssessmentExistingResult
	AssessmentConflict
	RecoveryFormed
	RecoveryExistingResult
	RecoveryConflict
	RecoveryNotEstablished
	AdjustmentFormed
	AdjustmentExistingResult
	AdjustmentConflict
	AdvanceNotAccepted
	AdvanceUndecidedOutcome
)

func (outcome AdvanceOutcome) String() string {
	switch outcome {
	case AdvanceAssessed:
		return "ADVANCE_ASSESSED"
	case AssessmentExistingResult:
		return "EXISTING_ASSESSMENT"
	case AssessmentConflict:
		return "ASSESSMENT_CONFLICT"
	case RecoveryFormed:
		return "RECOVERY_FORMED"
	case RecoveryExistingResult:
		return "EXISTING_RECOVERY"
	case RecoveryConflict:
		return "RECOVERY_CONFLICT"
	case RecoveryNotEstablished:
		return "ADVANCE_NOT_ESTABLISHED"
	case AdjustmentFormed:
		return "ADJUSTMENT_FORMED"
	case AdjustmentExistingResult:
		return "EXISTING_ADJUSTMENT"
	case AdjustmentConflict:
		return "ADJUSTMENT_CONFLICT"
	case AdvanceNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case AdvanceUndecidedOutcome:
		return "ADVANCE_UNDECIDED"
	default:
		return ""
	}
}

// AdvanceUndecidedReason 指名提交停在哪一步等谁。合同责任目录未配置是实例半边的
// 一格——回收需要合同依据，不默认可回收。
type AdvanceUndecidedReason uint8

const (
	AdvanceUndecidedReasonNone AdvanceUndecidedReason = iota
	AssessmentStoreUnavailable
	RecoveryStoreUnavailable
	AdjustmentStoreUnavailable
	ContractViewUnavailable
	ContractUnconfigured
)

func (reason AdvanceUndecidedReason) String() string {
	switch reason {
	case AssessmentStoreUnavailable:
		return "ASSESSMENT_STORE_UNAVAILABLE"
	case RecoveryStoreUnavailable:
		return "RECOVERY_STORE_UNAVAILABLE"
	case AdjustmentStoreUnavailable:
		return "ADJUSTMENT_STORE_UNAVAILABLE"
	case ContractViewUnavailable:
		return "CONTRACT_VIEW_UNAVAILABLE"
	case ContractUnconfigured:
		return "CONTRACT_UNCONFIGURED"
	default:
		return ""
	}
}

// AssessAdvanceCommand 携带一次实际代垫评估：四值裁决各守完备性由领域把门（成立要
// 资金事实+付款方+责任，其余带依据）。
type AssessAdvanceCommand struct {
	TenantID       domain.TenantID
	Assessment     string
	Obligation     string
	Verdict        domain.AdvanceVerdict
	FundsFact      string
	Payer          string
	Responsibility string
	Basis          string
	Currency       string
	AmountMinor    int64
	Version        string
	JudgedAt       time.Time
}

// FormRecoveryCommand 携带一次客户回收形成。刻意没有合同依据字段——依据由合同责任
// 视图给出，不由调用方口头声称（同费用确认的条件核对纪律）。
type FormRecoveryCommand struct {
	TenantID    domain.TenantID
	Assessment  string
	Recovery    string
	Customer    string
	Account     string
	AmountMinor int64
	FormedAt    time.Time
}

// AdjustRecoveryCommand 携带一次回收调整：带原因与新依据追加，不改写原回收。
type AdjustRecoveryCommand struct {
	TenantID    domain.TenantID
	Adjustment  string
	Recovery    string
	Reason      domain.RecoveryAdjustmentReason
	NewBasis    string
	Direction   domain.AdjustmentDirection
	Currency    string
	AmountMinor int64
	Period      string
	FormedAt    time.Time
}

type AdvanceResult struct {
	outcome      AdvanceOutcome
	reason       AdvanceUndecidedReason
	assessment   ports.AdvanceAssessmentRecord
	recovery     ports.AdvanceRecoveryRecord
	adjustment   ports.RecoveryAdjustmentRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result AdvanceResult) Outcome() AdvanceOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result AdvanceResult) UndecidedReason() AdvanceUndecidedReason {
	return result.reason
}

func (result AdvanceResult) Assessment() (ports.AdvanceAssessmentRecord, bool) {
	return result.assessment, result.hasRecord && result.assessment.Key.Assessment.String() != ""
}

func (result AdvanceResult) Recovery() (ports.AdvanceRecoveryRecord, bool) {
	return result.recovery, result.hasRecord && result.recovery.Key.Recovery.String() != ""
}

func (result AdvanceResult) Adjustment() (ports.RecoveryAdjustmentRecord, bool) {
	return result.adjustment, result.hasRecord && result.adjustment.Key.Adjustment.String() != ""
}

func (result AdvanceResult) ContinuationReference() string {
	return result.continuation
}

// RecoveryHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result AdvanceResult) RecoveryHandoffReference() string {
	return result.handoff
}

type AssessAdvanceRecoveryDeps struct {
	Assessments ports.AdvanceAssessmentStore
	Recoveries  ports.AdvanceRecoveryStore
	Adjustments ports.RecoveryAdjustmentStore
	Contracts   ports.ContractResponsibilityView
	Downstream  ports.AdvanceRecoveryHandoff
	// Inputs 是结算输入版本里付款核对一格的登记册（UC-SA-001 步 2），只由 AdoptDutyPaymentVerification
	// 走；Assess / FormRecovery / Adjust 三条路不碰它——采用输入与形成判断是两步，共用一只处理方只为
	// 让「谁在推进 UC-SA-001」在装配上是一处。
	Inputs ports.DutyPaymentVerificationAdoptionStore
	Clock  ports.Clock
}

type AssessAdvanceRecoveryHandler struct {
	deps AssessAdvanceRecoveryDeps
}

func NewAssessAdvanceRecoveryHandler(deps AssessAdvanceRecoveryDeps) *AssessAdvanceRecoveryHandler {
	return &AssessAdvanceRecoveryHandler{deps: deps}
}

// Assess 登记一次实际代垫评估：AssessActualAdvance 领域把门（成立要三引用、其余带
// 依据，SA 不重算税费不造付款）→ 幂等按（租户+评估标识）分重放/冲突。评估本身不交
// 意图——回收才是对账单纳入的上游。
func (handler *AssessAdvanceRecoveryHandler) Assess(
	ctx context.Context,
	command AssessAdvanceCommand,
) (AdvanceResult, error) {
	assessment, err := assessmentFrom(command)
	if err != nil {
		return AdvanceResult{outcome: AdvanceNotAccepted}, nil
	}

	key := ports.AdvanceAssessmentKey{TenantID: command.TenantID, Assessment: assessment.ID()}
	digest := assessDigest(command)
	existing, found, err := handler.deps.Assessments.FindByKey(ctx, key)
	if err != nil {
		return advanceUndecided(AssessmentStoreUnavailable, command.Assessment), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一评估标识携带不同裁决或引用：冲突保留原判断——重评换新评估标识。
			return AdvanceResult{outcome: AssessmentConflict}, nil
		}
		return AdvanceResult{outcome: AssessmentExistingResult, assessment: existing, hasRecord: true}, nil
	}

	record := ports.AdvanceAssessmentRecord{Key: key, ContentDigest: digest, Assessment: assessment, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Assessments.Save(ctx, record)
	if err != nil {
		return advanceUndecided(AssessmentStoreUnavailable, command.Assessment), nil
	}
	switch saved {
	case ports.AdvanceAssessmentSaved:
		return AdvanceResult{outcome: AdvanceAssessed, assessment: record, hasRecord: true}, nil
	case ports.AdvanceAssessmentAlreadyRecorded:
		winner, found, err := handler.deps.Assessments.FindByKey(ctx, key)
		if err != nil || !found {
			return advanceUndecided(AssessmentStoreUnavailable, command.Assessment), nil
		}
		return AdvanceResult{outcome: AssessmentExistingResult, assessment: winner, hasRecord: true}, nil
	default:
		return AdvanceResult{}, fmt.Errorf("%w: %d", ErrUnexpectedAssessmentSave, saved)
	}
}

// FormRecovery 依据已成立的评估形成客户回收：合同责任由视图核对（未配置→未决不默认
// 可回收）；非成立评估 → ADVANCE_NOT_ESTABLISHED 业务负向（领域哨兵编排分格）；意图
// 交对账单纳入。
func (handler *AssessAdvanceRecoveryHandler) FormRecovery(
	ctx context.Context,
	command FormRecoveryCommand,
) (AdvanceResult, error) {
	assessmentID, err := domain.NewAdvanceAssessmentID(command.Assessment)
	if err != nil {
		return AdvanceResult{outcome: AdvanceNotAccepted}, nil
	}
	recoveryID, err := domain.NewAdvanceRecoveryID(command.Recovery)
	if err != nil {
		return AdvanceResult{outcome: AdvanceNotAccepted}, nil
	}
	customer, err := domain.NewRecoveryCustomerReference(command.Customer)
	if err != nil {
		return AdvanceResult{outcome: AdvanceNotAccepted}, nil
	}
	account, err := domain.NewSettlementAccountID(command.Account)
	if err != nil {
		return AdvanceResult{outcome: AdvanceNotAccepted}, nil
	}

	assessmentRecord, found, err := handler.deps.Assessments.FindByKey(ctx,
		ports.AdvanceAssessmentKey{TenantID: command.TenantID, Assessment: assessmentID})
	if err != nil {
		return advanceUndecided(AssessmentStoreUnavailable, command.Assessment), nil
	}
	if !found {
		// 指名了不存在的评估：提交矛盾，改单重来。
		return AdvanceResult{outcome: AdvanceNotAccepted}, nil
	}

	contract, configured, err := handler.deps.Contracts.LoadContractResponsibility(
		ctx, command.TenantID, customer, assessmentRecord.Assessment.Obligation())
	if err != nil {
		return advanceUndecided(ContractViewUnavailable, command.Recovery), nil
	}
	if !configured {
		// 合同责任目录是实例半边：未配置停在未决，不默认可回收。
		return advanceUndecided(ContractUnconfigured, command.Recovery), nil
	}

	recovery, err := domain.FormCustomerAdvanceRecovery(
		assessmentRecord.Assessment, recoveryID, customer, contract, account,
		command.AmountMinor, command.FormedAt)
	if errors.Is(err, domain.ErrAdvanceNotEstablished) {
		// 代垫未成立：回收无从形成——业务负向，等新证据重评，不是改单。
		return AdvanceResult{outcome: RecoveryNotEstablished}, nil
	}
	if err != nil {
		return AdvanceResult{outcome: AdvanceNotAccepted}, nil
	}

	key := ports.AdvanceRecoveryKey{TenantID: command.TenantID, Recovery: recoveryID}
	digest := recoveryDigest(command)
	existing, alreadyFormed, err := handler.deps.Recoveries.FindByKey(ctx, key)
	if err != nil {
		return advanceUndecided(RecoveryStoreUnavailable, command.Recovery), nil
	}
	if alreadyFormed {
		if existing.ContentDigest != digest {
			return AdvanceResult{outcome: RecoveryConflict}, nil
		}
		return handler.existingRecovery(ctx, existing), nil
	}

	record := ports.AdvanceRecoveryRecord{Key: key, ContentDigest: digest, Recovery: recovery, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Recoveries.Save(ctx, record)
	if err != nil {
		return advanceUndecided(RecoveryStoreUnavailable, command.Recovery), nil
	}
	switch saved {
	case ports.AdvanceRecoverySaved:
		result := AdvanceResult{outcome: RecoveryFormed, recovery: record, hasRecord: true}
		result.handoff = handler.handOffRecovery(ctx, record)
		return result, nil
	case ports.AdvanceRecoveryAlreadyFormed:
		winner, found, err := handler.deps.Recoveries.FindByKey(ctx, key)
		if err != nil || !found {
			return advanceUndecided(RecoveryStoreUnavailable, command.Recovery), nil
		}
		return handler.existingRecovery(ctx, winner), nil
	default:
		return AdvanceResult{}, fmt.Errorf("%w: %d", ErrUnexpectedRecoverySave, saved)
	}
}

// Adjust 对已形成的回收追加调整：带原因与新依据（领域把门），不改写原回收；意图随
// 调整重新交对账单纳入。
func (handler *AssessAdvanceRecoveryHandler) Adjust(
	ctx context.Context,
	command AdjustRecoveryCommand,
) (AdvanceResult, error) {
	adjustment, err := adjustmentFrom(command)
	if err != nil {
		return AdvanceResult{outcome: AdvanceNotAccepted}, nil
	}
	recoveryID, err := domain.NewAdvanceRecoveryID(command.Recovery)
	if err != nil {
		return AdvanceResult{outcome: AdvanceNotAccepted}, nil
	}

	_, recoveryFound, err := handler.deps.Recoveries.FindByKey(ctx,
		ports.AdvanceRecoveryKey{TenantID: command.TenantID, Recovery: recoveryID})
	if err != nil {
		return advanceUndecided(RecoveryStoreUnavailable, command.Recovery), nil
	}
	if !recoveryFound {
		// 没有可调整的回收：调整不出无中生有的金额。
		return AdvanceResult{outcome: AdvanceNotAccepted}, nil
	}

	key := ports.RecoveryAdjustmentKey{TenantID: command.TenantID, Adjustment: adjustment.ID()}
	digest := adjustDigest(command)
	existing, found, err := handler.deps.Adjustments.FindByKey(ctx, key)
	if err != nil {
		return advanceUndecided(AdjustmentStoreUnavailable, command.Adjustment), nil
	}
	if found {
		if existing.ContentDigest != digest {
			return AdvanceResult{outcome: AdjustmentConflict}, nil
		}
		return handler.existingAdjustment(ctx, existing), nil
	}

	record := ports.RecoveryAdjustmentRecord{Key: key, ContentDigest: digest, Adjustment: adjustment, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Adjustments.Save(ctx, record)
	if err != nil {
		return advanceUndecided(AdjustmentStoreUnavailable, command.Adjustment), nil
	}
	switch saved {
	case ports.RecoveryAdjustmentSaved:
		result := AdvanceResult{outcome: AdjustmentFormed, adjustment: record, hasRecord: true}
		result.handoff = handler.handOffAdjustment(ctx, record)
		return result, nil
	case ports.RecoveryAdjustmentAlreadyFormed:
		winner, found, err := handler.deps.Adjustments.FindByKey(ctx, key)
		if err != nil || !found {
			return advanceUndecided(AdjustmentStoreUnavailable, command.Adjustment), nil
		}
		return handler.existingAdjustment(ctx, winner), nil
	default:
		return AdvanceResult{}, fmt.Errorf("%w: %d", ErrUnexpectedRecoverySave, saved)
	}
}

func assessmentFrom(command AssessAdvanceCommand) (domain.ActualAdvanceAssessment, error) {
	spec := domain.ActualAdvanceAssessmentSpec{
		Verdict:     command.Verdict,
		AmountMinor: command.AmountMinor,
		JudgedAt:    command.JudgedAt,
	}
	var err error
	if spec.ID, err = domain.NewAdvanceAssessmentID(command.Assessment); err != nil {
		return domain.ActualAdvanceAssessment{}, err
	}
	if spec.Obligation, err = domain.NewTaxObligationReference(command.Obligation); err != nil {
		return domain.ActualAdvanceAssessment{}, err
	}
	if spec.Currency, err = domain.NewCurrencyCode(command.Currency); err != nil {
		return domain.ActualAdvanceAssessment{}, err
	}
	if spec.Version, err = domain.NewAdvanceAssessmentVersion(command.Version); err != nil {
		return domain.ActualAdvanceAssessment{}, err
	}
	if strings.TrimSpace(command.FundsFact) != "" {
		if spec.FundsFact, err = domain.NewFundsFactReference(command.FundsFact); err != nil {
			return domain.ActualAdvanceAssessment{}, err
		}
	}
	if strings.TrimSpace(command.Payer) != "" {
		if spec.Payer, err = domain.NewAdvancePayerReference(command.Payer); err != nil {
			return domain.ActualAdvanceAssessment{}, err
		}
	}
	if strings.TrimSpace(command.Responsibility) != "" {
		if spec.Responsibility, err = domain.NewAdvanceResponsibilityReference(command.Responsibility); err != nil {
			return domain.ActualAdvanceAssessment{}, err
		}
	}
	if strings.TrimSpace(command.Basis) != "" {
		if spec.Basis, err = domain.NewAssessmentBasisReference(command.Basis); err != nil {
			return domain.ActualAdvanceAssessment{}, err
		}
	}
	return domain.AssessActualAdvance(spec)
}

func adjustmentFrom(command AdjustRecoveryCommand) (domain.RecoveryAdjustment, error) {
	spec := domain.RecoveryAdjustmentSpec{
		Reason:      command.Reason,
		Direction:   command.Direction,
		AmountMinor: command.AmountMinor,
		FormedAt:    command.FormedAt,
	}
	var err error
	if spec.ID, err = domain.NewRecoveryAdjustmentID(command.Adjustment); err != nil {
		return domain.RecoveryAdjustment{}, err
	}
	if spec.Recovery, err = domain.NewAdvanceRecoveryID(command.Recovery); err != nil {
		return domain.RecoveryAdjustment{}, err
	}
	if spec.NewBasis, err = domain.NewAssessmentBasisReference(command.NewBasis); err != nil {
		return domain.RecoveryAdjustment{}, err
	}
	if spec.Currency, err = domain.NewCurrencyCode(command.Currency); err != nil {
		return domain.RecoveryAdjustment{}, err
	}
	if spec.Period, err = domain.NewBillingPeriodReference(command.Period); err != nil {
		return domain.RecoveryAdjustment{}, err
	}
	return domain.FormRecoveryAdjustment(spec)
}

func advanceUndecided(reason AdvanceUndecidedReason, subject string) AdvanceResult {
	return AdvanceResult{
		outcome:      AdvanceUndecidedOutcome,
		reason:       reason,
		continuation: advanceContinuation(reason.String(), subject),
	}
}

// existingRecovery / existingAdjustment 按已有记录作答并重发同一份意图。
func (handler *AssessAdvanceRecoveryHandler) existingRecovery(
	ctx context.Context,
	record ports.AdvanceRecoveryRecord,
) AdvanceResult {
	return AdvanceResult{
		outcome:   RecoveryExistingResult,
		recovery:  record,
		hasRecord: true,
		handoff:   handler.handOffRecovery(ctx, record),
	}
}

func (handler *AssessAdvanceRecoveryHandler) existingAdjustment(
	ctx context.Context,
	record ports.RecoveryAdjustmentRecord,
) AdvanceResult {
	return AdvanceResult{
		outcome:    AdjustmentExistingResult,
		adjustment: record,
		hasRecord:  true,
		handoff:    handler.handOffAdjustment(ctx, record),
	}
}

// handOffRecovery / handOffAdjustment 把回收与调整交给对账单纳入。投递失败不翻结果，
// 留续办引用重放时重发同一份。
func (handler *AssessAdvanceRecoveryHandler) handOffRecovery(
	ctx context.Context,
	record ports.AdvanceRecoveryRecord,
) string {
	if err := handler.deps.Downstream.HandOffAdvanceRecovery(ctx, ports.AdvanceRecoveryIntent{Recovery: record}); err == nil {
		return ""
	}
	return advanceContinuation("ADVANCE_RECOVERY_HANDOFF", record.Key.TenantID.String(), record.Key.Recovery.String())
}

func (handler *AssessAdvanceRecoveryHandler) handOffAdjustment(
	ctx context.Context,
	record ports.RecoveryAdjustmentRecord,
) string {
	if err := handler.deps.Downstream.HandOffAdvanceRecovery(ctx, ports.AdvanceRecoveryIntent{Adjustment: record}); err == nil {
		return ""
	}
	return advanceContinuation("RECOVERY_ADJUSTMENT_HANDOFF", record.Key.TenantID.String(), record.Key.Adjustment.String())
}

func advanceContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

func assessDigest(command AssessAdvanceCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.Verdict),
		command.Obligation,
		command.FundsFact,
		command.Payer,
		command.Responsibility,
		command.Basis,
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.Version,
		command.JudgedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func recoveryDigest(command FormRecoveryCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Assessment,
		command.Customer,
		command.Account,
		fmt.Sprintf("%d", command.AmountMinor),
		command.FormedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func adjustDigest(command AdjustRecoveryCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Recovery,
		fmt.Sprintf("%d", command.Reason),
		command.NewBasis,
		fmt.Sprintf("%d", command.Direction),
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.Period,
		command.FormedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

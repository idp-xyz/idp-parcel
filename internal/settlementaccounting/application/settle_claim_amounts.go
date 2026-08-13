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

// ErrUnexpectedClaimSave 说明索赔结算库交回了封闭集合以外的写入结果。
var ErrUnexpectedClaimSave = errors.New("settlement accounting: unexpected claim settlement save outcome")

// ClaimSettlementOutcome 是赔付与追偿金额编排的应用处理结果。金额各层独立不互推
// （赔付、应追偿、认可、到账、核销分层），本编排只造前三层，真实资金归 UC-SA-005。
type ClaimSettlementOutcome uint8

const (
	ClaimSettlementOutcomeInvalid ClaimSettlementOutcome = iota
	ClaimAmountFormed
	ClaimAmountExisting
	ClaimAmountConflict
	ReceivableFormed
	ReceivableExisting
	ReceivableConflict
	RecoveryAcknowledged
	AcknowledgementExisting
	AcknowledgementConflict
	ClaimAdjustmentFormed
	ClaimAdjustmentExisting
	ClaimAdjustmentConflict
	ClaimNotAccepted
	ClaimUndecided
)

func (outcome ClaimSettlementOutcome) String() string {
	switch outcome {
	case ClaimAmountFormed:
		return "CLAIM_AMOUNT_FORMED"
	case ClaimAmountExisting:
		return "EXISTING_CLAIM_AMOUNT"
	case ClaimAmountConflict:
		return "CLAIM_AMOUNT_CONFLICT"
	case ReceivableFormed:
		return "RECEIVABLE_FORMED"
	case ReceivableExisting:
		return "EXISTING_RECEIVABLE"
	case ReceivableConflict:
		return "RECEIVABLE_CONFLICT"
	case RecoveryAcknowledged:
		return "RECOVERY_ACKNOWLEDGED"
	case AcknowledgementExisting:
		return "EXISTING_ACKNOWLEDGEMENT"
	case AcknowledgementConflict:
		return "ACKNOWLEDGEMENT_CONFLICT"
	case ClaimAdjustmentFormed:
		return "CLAIM_ADJUSTMENT_FORMED"
	case ClaimAdjustmentExisting:
		return "EXISTING_CLAIM_ADJUSTMENT"
	case ClaimAdjustmentConflict:
		return "CLAIM_ADJUSTMENT_CONFLICT"
	case ClaimNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case ClaimUndecided:
		return "CLAIM_UNDECIDED"
	default:
		return ""
	}
}

// ClaimUndecidedReason 指名提交停在哪一步等谁。金额规则目录未配置是实例半边的一格
// ——没有规则版本不形成金额，不默认限额。
type ClaimUndecidedReason uint8

const (
	ClaimUndecidedReasonNone ClaimUndecidedReason = iota
	ClaimAmountStoreUnavailable
	ReceivableStoreUnavailable
	AcknowledgementStoreUnavailable
	ClaimAdjustmentStoreUnavailable
	ClaimRuleViewUnavailable
	ClaimRuleUnconfigured
)

func (reason ClaimUndecidedReason) String() string {
	switch reason {
	case ClaimAmountStoreUnavailable:
		return "CLAIM_AMOUNT_STORE_UNAVAILABLE"
	case ReceivableStoreUnavailable:
		return "RECEIVABLE_STORE_UNAVAILABLE"
	case AcknowledgementStoreUnavailable:
		return "ACKNOWLEDGEMENT_STORE_UNAVAILABLE"
	case ClaimAdjustmentStoreUnavailable:
		return "CLAIM_ADJUSTMENT_STORE_UNAVAILABLE"
	case ClaimRuleViewUnavailable:
		return "CLAIM_RULE_VIEW_UNAVAILABLE"
	case ClaimRuleUnconfigured:
		return "CLAIM_RULE_UNCONFIGURED"
	default:
		return ""
	}
}

// FormClaimAmountCommand 携带一笔客户方向金额。刻意没有规则字段——规则版本由视图
// 给出（AT-SA-147）；责任结论引用必备（AT-SA-144 没有结论就没有金额）。
type FormClaimAmountCommand struct {
	TenantID       domain.TenantID
	Amount         string
	Kind           domain.CustomerClaimAmountKind
	ClaimItem      string
	Responsibility string
	LegalEntity    string
	OriginalCharge string
	Currency       string
	AmountMinor    int64
	Period         string
	FormedAt       time.Time
}

// FormReceivableCommand 携带一笔应追偿金额：责任条件满足即可独立形成，不等对方响应
// （AT-SA-149）。
type FormReceivableCommand struct {
	TenantID       domain.TenantID
	Receivable     string
	Matter         string
	Responsibility string
	Counterparty   string
	LegalEntity    string
	Currency       string
	AmountMinor    int64
	FormedAt       time.Time
}

// AcknowledgeCommand 携带一次追偿认可：立场封闭二值（要求补充/审核中/拒绝/无响应
// 没有格），全额认可等额、部分认可小于额（领域把门）。
type AcknowledgeCommand struct {
	TenantID          domain.TenantID
	Acknowledgement   string
	Receivable        string
	Response          string
	Standing          domain.ResponseStanding
	AcknowledgedMinor int64
	AcknowledgedAt    time.Time
}

// AdjustClaimAmountCommand 携带一次追加调整：带原因与新责任依据，不改写原金额
// （AT-SA-152）。
type AdjustClaimAmountCommand struct {
	TenantID    domain.TenantID
	Adjustment  string
	TargetKind  domain.AdjustedAmountKind
	Target      string
	Reason      domain.ClaimAdjustmentReason
	Basis       string
	Direction   domain.AdjustmentDirection
	Currency    string
	AmountMinor int64
	Period      string
	FormedAt    time.Time
}

type ClaimSettlementResult struct {
	outcome         ClaimSettlementOutcome
	reason          ClaimUndecidedReason
	amount          ports.ClaimAmountRecord
	receivable      ports.ReceivableRecord
	acknowledgement ports.AcknowledgementRecord
	adjustment      ports.ClaimAdjustmentRecord
	hasRecord       bool
	continuation    string
	handoff         string
}

func (result ClaimSettlementResult) Outcome() ClaimSettlementOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result ClaimSettlementResult) UndecidedReason() ClaimUndecidedReason {
	return result.reason
}

func (result ClaimSettlementResult) ClaimAmount() (ports.ClaimAmountRecord, bool) {
	return result.amount, result.hasRecord && result.amount.Key.Amount.String() != ""
}

func (result ClaimSettlementResult) Receivable() (ports.ReceivableRecord, bool) {
	return result.receivable, result.hasRecord && result.receivable.Key.Receivable.String() != ""
}

func (result ClaimSettlementResult) Acknowledgement() (ports.AcknowledgementRecord, bool) {
	return result.acknowledgement, result.hasRecord && result.acknowledgement.Key.Acknowledgement.String() != ""
}

func (result ClaimSettlementResult) Adjustment() (ports.ClaimAdjustmentRecord, bool) {
	return result.adjustment, result.hasRecord && result.adjustment.Key.Adjustment.String() != ""
}

func (result ClaimSettlementResult) ContinuationReference() string {
	return result.continuation
}

// ClaimHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result ClaimSettlementResult) ClaimHandoffReference() string {
	return result.handoff
}

type SettleClaimAmountsDeps struct {
	Amounts          ports.CustomerClaimAmountStore
	Receivables      ports.RecoveryReceivableStore
	Acknowledgements ports.RecoveryAcknowledgementStore
	Adjustments      ports.ClaimAmountAdjustmentStore
	Rules            ports.ClaimAmountRuleView
	Downstream       ports.ClaimSettlementHandoff
	Clock            ports.Clock
}

type SettleClaimAmountsHandler struct {
	deps SettleClaimAmountsDeps
}

func NewSettleClaimAmountsHandler(deps SettleClaimAmountsDeps) *SettleClaimAmountsHandler {
	return &SettleClaimAmountsHandler{deps: deps}
}

// FormClaimAmount 形成一笔赔付/退款金额：规则版本由视图核对（未配置→未决不默认限额）
// → FormCustomerClaimAmount（退款要原费用、赔付不带原费用——两族不混，领域把门）→
// 幂等 → 意图交对账单纳入。
func (handler *SettleClaimAmountsHandler) FormClaimAmount(
	ctx context.Context,
	command FormClaimAmountCommand,
) (ClaimSettlementResult, error) {
	amountID, err := domain.NewCustomerClaimAmountID(command.Amount)
	if err != nil {
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}
	responsibility, err := domain.NewResponsibilityConclusionReference(command.Responsibility)
	if err != nil {
		// 没有责任结论就没有金额（AT-SA-144）——缺结论是提交矛盾。
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}

	rule, configured, err := handler.deps.Rules.LoadClaimAmountRule(ctx, command.TenantID, responsibility)
	if err != nil {
		return claimUndecided(ClaimRuleViewUnavailable, command.Amount), nil
	}
	if !configured {
		// 金额规则目录是实例半边：未配置停在未决，不默认限额。
		return claimUndecided(ClaimRuleUnconfigured, command.Amount), nil
	}

	amount, err := claimAmountFrom(command, amountID, responsibility, rule)
	if err != nil {
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}

	key := ports.ClaimAmountKey{TenantID: command.TenantID, Amount: amountID}
	digest := claimAmountDigest(command)
	existing, found, err := handler.deps.Amounts.FindByKey(ctx, key)
	if err != nil {
		return claimUndecided(ClaimAmountStoreUnavailable, command.Amount), nil
	}
	if found {
		if existing.ContentDigest != digest {
			return ClaimSettlementResult{outcome: ClaimAmountConflict}, nil
		}
		return handler.existingAmount(ctx, existing), nil
	}

	record := ports.ClaimAmountRecord{Key: key, ContentDigest: digest, Amount: amount, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Amounts.Save(ctx, record)
	if err != nil {
		return claimUndecided(ClaimAmountStoreUnavailable, command.Amount), nil
	}
	switch saved {
	case ports.ClaimAmountSaved:
		result := ClaimSettlementResult{outcome: ClaimAmountFormed, amount: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.ClaimSettlementIntent{ClaimAmount: record}, command.Amount)
		return result, nil
	case ports.ClaimAmountAlreadyFormed:
		winner, found, err := handler.deps.Amounts.FindByKey(ctx, key)
		if err != nil || !found {
			return claimUndecided(ClaimAmountStoreUnavailable, command.Amount), nil
		}
		return handler.existingAmount(ctx, winner), nil
	default:
		return ClaimSettlementResult{}, fmt.Errorf("%w: %d", ErrUnexpectedClaimSave, saved)
	}
}

// FormReceivable 形成一笔应追偿：责任条件满足即可独立形成（不等对方响应，AT-SA-149），
// 规则版本由视图核对；意图交追偿链。
func (handler *SettleClaimAmountsHandler) FormReceivable(
	ctx context.Context,
	command FormReceivableCommand,
) (ClaimSettlementResult, error) {
	receivableID, err := domain.NewRecoveryReceivableID(command.Receivable)
	if err != nil {
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}
	responsibility, err := domain.NewResponsibilityConclusionReference(command.Responsibility)
	if err != nil {
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}

	rule, configured, err := handler.deps.Rules.LoadClaimAmountRule(ctx, command.TenantID, responsibility)
	if err != nil {
		return claimUndecided(ClaimRuleViewUnavailable, command.Receivable), nil
	}
	if !configured {
		return claimUndecided(ClaimRuleUnconfigured, command.Receivable), nil
	}

	receivable, err := receivableFrom(command, receivableID, responsibility, rule)
	if err != nil {
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}

	key := ports.ReceivableKey{TenantID: command.TenantID, Receivable: receivableID}
	digest := receivableDigest(command)
	existing, found, err := handler.deps.Receivables.FindByKey(ctx, key)
	if err != nil {
		return claimUndecided(ReceivableStoreUnavailable, command.Receivable), nil
	}
	if found {
		if existing.ContentDigest != digest {
			return ClaimSettlementResult{outcome: ReceivableConflict}, nil
		}
		return ClaimSettlementResult{outcome: ReceivableExisting, receivable: existing, hasRecord: true}, nil
	}

	record := ports.ReceivableRecord{Key: key, ContentDigest: digest, Receivable: receivable, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Receivables.Save(ctx, record)
	if err != nil {
		return claimUndecided(ReceivableStoreUnavailable, command.Receivable), nil
	}
	switch saved {
	case ports.ReceivableSaved:
		result := ClaimSettlementResult{outcome: ReceivableFormed, receivable: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.ClaimSettlementIntent{Receivable: record}, command.Receivable)
		return result, nil
	case ports.ReceivableAlreadyFormed:
		winner, found, err := handler.deps.Receivables.FindByKey(ctx, key)
		if err != nil || !found {
			return claimUndecided(ReceivableStoreUnavailable, command.Receivable), nil
		}
		return ClaimSettlementResult{outcome: ReceivableExisting, receivable: winner, hasRecord: true}, nil
	default:
		return ClaimSettlementResult{}, fmt.Errorf("%w: %d", ErrUnexpectedClaimSave, saved)
	}
}

// Acknowledge 登记一次追偿认可：认可依附既有应追偿（无中生有拒），立场封闭二值与
// 额度关系由 AcknowledgeRecovery 把门；认可≠到账（真实资金归 UC-SA-005）。
func (handler *SettleClaimAmountsHandler) Acknowledge(
	ctx context.Context,
	command AcknowledgeCommand,
) (ClaimSettlementResult, error) {
	acknowledgementID, err := domain.NewAcknowledgementID(command.Acknowledgement)
	if err != nil {
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}
	receivableID, err := domain.NewRecoveryReceivableID(command.Receivable)
	if err != nil {
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}
	response, err := domain.NewCounterpartyResponseReference(command.Response)
	if err != nil {
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}

	receivableRecord, found, err := handler.deps.Receivables.FindByKey(ctx,
		ports.ReceivableKey{TenantID: command.TenantID, Receivable: receivableID})
	if err != nil {
		return claimUndecided(ReceivableStoreUnavailable, command.Receivable), nil
	}
	if !found {
		// 没有应追偿就没有可认可的范围。
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}

	acknowledgement, err := domain.AcknowledgeRecovery(
		receivableRecord.Receivable, acknowledgementID, response,
		command.Standing, command.AcknowledgedMinor, command.AcknowledgedAt)
	if err != nil {
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}

	key := ports.AcknowledgementKey{TenantID: command.TenantID, Acknowledgement: acknowledgementID}
	digest := acknowledgeDigest(command)
	existing, recorded, err := handler.deps.Acknowledgements.FindByKey(ctx, key)
	if err != nil {
		return claimUndecided(AcknowledgementStoreUnavailable, command.Acknowledgement), nil
	}
	if recorded {
		if existing.ContentDigest != digest {
			return ClaimSettlementResult{outcome: AcknowledgementConflict}, nil
		}
		return ClaimSettlementResult{outcome: AcknowledgementExisting, acknowledgement: existing, hasRecord: true}, nil
	}

	record := ports.AcknowledgementRecord{Key: key, ContentDigest: digest, Acknowledgement: acknowledgement, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Acknowledgements.Save(ctx, record)
	if err != nil {
		return claimUndecided(AcknowledgementStoreUnavailable, command.Acknowledgement), nil
	}
	switch saved {
	case ports.AcknowledgementSaved:
		result := ClaimSettlementResult{outcome: RecoveryAcknowledged, acknowledgement: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.ClaimSettlementIntent{Acknowledgement: record}, command.Acknowledgement)
		return result, nil
	case ports.AcknowledgementAlreadyRecorded:
		winner, found, err := handler.deps.Acknowledgements.FindByKey(ctx, key)
		if err != nil || !found {
			return claimUndecided(AcknowledgementStoreUnavailable, command.Acknowledgement), nil
		}
		return ClaimSettlementResult{outcome: AcknowledgementExisting, acknowledgement: winner, hasRecord: true}, nil
	default:
		return ClaimSettlementResult{}, fmt.Errorf("%w: %d", ErrUnexpectedClaimSave, saved)
	}
}

// Adjust 形成一次追加调整：目标按类别在各自库核对存在（无中生有拒），带原因与新责任
// 依据不改写原金额（AT-SA-152）；意图随调整交下游。
func (handler *SettleClaimAmountsHandler) Adjust(
	ctx context.Context,
	command AdjustClaimAmountCommand,
) (ClaimSettlementResult, error) {
	adjustment, err := adjustmentSpecFrom(command)
	if err != nil {
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}

	targetExists, undecided := handler.targetExists(ctx, command)
	if undecided != nil {
		return *undecided, nil
	}
	if !targetExists {
		// 调整不出无中生有的金额：目标金额必须已经存在。
		return ClaimSettlementResult{outcome: ClaimNotAccepted}, nil
	}

	key := ports.ClaimAdjustmentKey{TenantID: command.TenantID, Adjustment: adjustment.ID()}
	digest := claimAdjustDigest(command)
	existing, found, err := handler.deps.Adjustments.FindByKey(ctx, key)
	if err != nil {
		return claimUndecided(ClaimAdjustmentStoreUnavailable, command.Adjustment), nil
	}
	if found {
		if existing.ContentDigest != digest {
			return ClaimSettlementResult{outcome: ClaimAdjustmentConflict}, nil
		}
		return ClaimSettlementResult{outcome: ClaimAdjustmentExisting, adjustment: existing, hasRecord: true}, nil
	}

	record := ports.ClaimAdjustmentRecord{Key: key, ContentDigest: digest, Adjustment: adjustment, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Adjustments.Save(ctx, record)
	if err != nil {
		return claimUndecided(ClaimAdjustmentStoreUnavailable, command.Adjustment), nil
	}
	switch saved {
	case ports.ClaimAdjustmentSaved:
		result := ClaimSettlementResult{outcome: ClaimAdjustmentFormed, adjustment: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.ClaimSettlementIntent{Adjustment: record}, command.Adjustment)
		return result, nil
	case ports.ClaimAdjustmentAlreadyFormed:
		winner, found, err := handler.deps.Adjustments.FindByKey(ctx, key)
		if err != nil || !found {
			return claimUndecided(ClaimAdjustmentStoreUnavailable, command.Adjustment), nil
		}
		return ClaimSettlementResult{outcome: ClaimAdjustmentExisting, adjustment: winner, hasRecord: true}, nil
	default:
		return ClaimSettlementResult{}, fmt.Errorf("%w: %d", ErrUnexpectedClaimSave, saved)
	}
}

// targetExists 按目标类别在各自库核对目标金额存在。
func (handler *SettleClaimAmountsHandler) targetExists(
	ctx context.Context,
	command AdjustClaimAmountCommand,
) (bool, *ClaimSettlementResult) {
	switch command.TargetKind {
	case domain.AdjustsCustomerClaimAmount:
		amountID, err := domain.NewCustomerClaimAmountID(command.Target)
		if err != nil {
			return false, nil
		}
		_, found, err := handler.deps.Amounts.FindByKey(ctx, ports.ClaimAmountKey{TenantID: command.TenantID, Amount: amountID})
		if err != nil {
			undecided := claimUndecided(ClaimAmountStoreUnavailable, command.Target)
			return false, &undecided
		}
		return found, nil
	case domain.AdjustsRecoveryReceivable:
		receivableID, err := domain.NewRecoveryReceivableID(command.Target)
		if err != nil {
			return false, nil
		}
		_, found, err := handler.deps.Receivables.FindByKey(ctx, ports.ReceivableKey{TenantID: command.TenantID, Receivable: receivableID})
		if err != nil {
			undecided := claimUndecided(ReceivableStoreUnavailable, command.Target)
			return false, &undecided
		}
		return found, nil
	case domain.AdjustsAcknowledgement:
		acknowledgementID, err := domain.NewAcknowledgementID(command.Target)
		if err != nil {
			return false, nil
		}
		_, found, err := handler.deps.Acknowledgements.FindByKey(ctx, ports.AcknowledgementKey{TenantID: command.TenantID, Acknowledgement: acknowledgementID})
		if err != nil {
			undecided := claimUndecided(AcknowledgementStoreUnavailable, command.Target)
			return false, &undecided
		}
		return found, nil
	default:
		return false, nil
	}
}

func claimAmountFrom(
	command FormClaimAmountCommand,
	amountID domain.CustomerClaimAmountID,
	responsibility domain.ResponsibilityConclusionReference,
	rule domain.AmountRuleVersionReference,
) (domain.CustomerClaimAmount, error) {
	spec := domain.CustomerClaimAmountSpec{
		ID:             amountID,
		Kind:           command.Kind,
		Responsibility: responsibility,
		RuleVersion:    rule,
		AmountMinor:    command.AmountMinor,
		FormedAt:       command.FormedAt,
	}
	var err error
	if spec.ClaimItem, err = domain.NewClaimItemReference(command.ClaimItem); err != nil {
		return domain.CustomerClaimAmount{}, err
	}
	if spec.LegalEntity, err = domain.NewLegalEntityReference(command.LegalEntity); err != nil {
		return domain.CustomerClaimAmount{}, err
	}
	if spec.Currency, err = domain.NewCurrencyCode(command.Currency); err != nil {
		return domain.CustomerClaimAmount{}, err
	}
	if spec.Period, err = domain.NewBillingPeriodReference(command.Period); err != nil {
		return domain.CustomerClaimAmount{}, err
	}
	if strings.TrimSpace(command.OriginalCharge) != "" {
		if spec.OriginalCharge, err = domain.NewCustomerChargeID(command.OriginalCharge); err != nil {
			return domain.CustomerClaimAmount{}, err
		}
	}
	return domain.FormCustomerClaimAmount(spec)
}

func receivableFrom(
	command FormReceivableCommand,
	receivableID domain.RecoveryReceivableID,
	responsibility domain.ResponsibilityConclusionReference,
	rule domain.AmountRuleVersionReference,
) (domain.RecoveryReceivable, error) {
	spec := domain.RecoveryReceivableSpec{
		ID:             receivableID,
		Responsibility: responsibility,
		RuleVersion:    rule,
		AmountMinor:    command.AmountMinor,
		FormedAt:       command.FormedAt,
	}
	var err error
	if spec.Matter, err = domain.NewRecoveryMatterReference(command.Matter); err != nil {
		return domain.RecoveryReceivable{}, err
	}
	if spec.Counterparty, err = domain.NewRecoveryCounterpartyReference(command.Counterparty); err != nil {
		return domain.RecoveryReceivable{}, err
	}
	if spec.LegalEntity, err = domain.NewLegalEntityReference(command.LegalEntity); err != nil {
		return domain.RecoveryReceivable{}, err
	}
	if spec.Currency, err = domain.NewCurrencyCode(command.Currency); err != nil {
		return domain.RecoveryReceivable{}, err
	}
	return domain.FormRecoveryReceivable(spec)
}

func adjustmentSpecFrom(command AdjustClaimAmountCommand) (domain.ClaimAmountAdjustment, error) {
	spec := domain.ClaimAmountAdjustmentSpec{
		TargetKind:  command.TargetKind,
		Reason:      command.Reason,
		Direction:   command.Direction,
		AmountMinor: command.AmountMinor,
		FormedAt:    command.FormedAt,
	}
	var err error
	if spec.ID, err = domain.NewClaimAmountAdjustmentID(command.Adjustment); err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	if spec.Target, err = domain.NewAdjustedAmountReference(command.Target); err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	if spec.Basis, err = domain.NewResponsibilityConclusionReference(command.Basis); err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	if spec.Currency, err = domain.NewCurrencyCode(command.Currency); err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	if spec.Period, err = domain.NewBillingPeriodReference(command.Period); err != nil {
		return domain.ClaimAmountAdjustment{}, err
	}
	return domain.FormClaimAmountAdjustment(spec)
}

func claimUndecided(reason ClaimUndecidedReason, subject string) ClaimSettlementResult {
	return ClaimSettlementResult{
		outcome:      ClaimUndecided,
		reason:       reason,
		continuation: claimContinuation(reason.String(), subject),
	}
}

// existingAmount 按已有金额作答并重发同一份意图。
func (handler *SettleClaimAmountsHandler) existingAmount(
	ctx context.Context,
	record ports.ClaimAmountRecord,
) ClaimSettlementResult {
	return ClaimSettlementResult{
		outcome:   ClaimAmountExisting,
		amount:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, ports.ClaimSettlementIntent{ClaimAmount: record}, record.Key.Amount.String()),
	}
}

// handOff 把记录交给下游（赔付/调整→对账单纳入，应追偿/认可→追偿链）。投递失败不翻
// 结果，留续办引用重放时重发同一份。
func (handler *SettleClaimAmountsHandler) handOff(
	ctx context.Context,
	intent ports.ClaimSettlementIntent,
	subject string,
) string {
	if err := handler.deps.Downstream.HandOffClaimSettlement(ctx, intent); err == nil {
		return ""
	}
	return claimContinuation("CLAIM_SETTLEMENT_HANDOFF", subject)
}

func claimContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

func claimAmountDigest(command FormClaimAmountCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.Kind),
		command.ClaimItem,
		command.Responsibility,
		command.LegalEntity,
		command.OriginalCharge,
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.Period,
		command.FormedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func receivableDigest(command FormReceivableCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Matter,
		command.Responsibility,
		command.Counterparty,
		command.LegalEntity,
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.FormedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func acknowledgeDigest(command AcknowledgeCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Receivable,
		command.Response,
		fmt.Sprintf("%d", command.Standing),
		fmt.Sprintf("%d", command.AcknowledgedMinor),
		command.AcknowledgedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func claimAdjustDigest(command AdjustClaimAmountCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.TargetKind),
		command.Target,
		fmt.Sprintf("%d", command.Reason),
		command.Basis,
		fmt.Sprintf("%d", command.Direction),
		command.Currency,
		fmt.Sprintf("%d", command.AmountMinor),
		command.Period,
		command.FormedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

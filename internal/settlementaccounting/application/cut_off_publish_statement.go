package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrUnexpectedStatementSave 说明对账单库交回了封闭集合以外的写入结果。
var ErrUnexpectedStatementSave = errors.New("settlement accounting: unexpected statement save outcome")

// StatementOutcome 是对账单编排的应用处理结果。三个业务负向格（ADR-0029）：勾稽不平
// （改申报总额再来）、费用未确认（先确认再截单）、纳入回填（只能归后续账期）——都是
// 已知答案，不是改单形状也不是等依赖。
type StatementOutcome uint8

const (
	StatementOutcomeInvalid StatementOutcome = iota
	StatementPublished
	StatementExistingResult
	StatementConflict
	StatementImbalanceOutcome
	StatementChargeNotConfirmed
	StatementVoidedOutcome
	StatementAlreadyVoided
	LateChargeIncluded
	AdjustmentIncluded
	InclusionExistingResult
	InclusionConflict
	InclusionBackfillsOutcome
	DisputeOpened
	DisputeExistingResult
	DisputeConflict
	DisputeResolvedOutcome
	DisputeAlreadyResolved
	StatementNotAccepted
	StatementUndecided
)

func (outcome StatementOutcome) String() string {
	switch outcome {
	case StatementPublished:
		return "STATEMENT_PUBLISHED"
	case StatementExistingResult:
		return "EXISTING_STATEMENT"
	case StatementConflict:
		return "STATEMENT_CONFLICT"
	case StatementImbalanceOutcome:
		return "STATEMENT_IMBALANCE"
	case StatementChargeNotConfirmed:
		return "CHARGE_NOT_CONFIRMED"
	case StatementVoidedOutcome:
		return "STATEMENT_VOIDED"
	case StatementAlreadyVoided:
		return "ALREADY_VOIDED"
	case LateChargeIncluded:
		return "LATE_CHARGE_INCLUDED"
	case AdjustmentIncluded:
		return "ADJUSTMENT_INCLUDED"
	case InclusionExistingResult:
		return "EXISTING_INCLUSION"
	case InclusionConflict:
		return "INCLUSION_CONFLICT"
	case InclusionBackfillsOutcome:
		return "INCLUSION_BACKFILLS_PERIOD"
	case DisputeOpened:
		return "DISPUTE_OPENED"
	case DisputeExistingResult:
		return "EXISTING_DISPUTE"
	case DisputeConflict:
		return "DISPUTE_CONFLICT"
	case DisputeResolvedOutcome:
		return "DISPUTE_RESOLVED"
	case DisputeAlreadyResolved:
		return "DISPUTE_ALREADY_RESOLVED"
	case StatementNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case StatementUndecided:
		return "STATEMENT_UNDECIDED"
	default:
		return ""
	}
}

// StatementUndecidedReason 指名提交停在哪一步等谁。
type StatementUndecidedReason uint8

const (
	StatementUndecidedReasonNone StatementUndecidedReason = iota
	StatementStoreUnavailable
	StatementChargeStoreUnavailable
	InclusionStoreUnavailable
	DisputeStoreUnavailable
	AdjustmentViewUnavailable
)

func (reason StatementUndecidedReason) String() string {
	switch reason {
	case StatementStoreUnavailable:
		return "STATEMENT_STORE_UNAVAILABLE"
	case StatementChargeStoreUnavailable:
		return "CHARGE_STORE_UNAVAILABLE"
	case InclusionStoreUnavailable:
		return "INCLUSION_STORE_UNAVAILABLE"
	case DisputeStoreUnavailable:
		return "DISPUTE_STORE_UNAVAILABLE"
	case AdjustmentViewUnavailable:
		return "ADJUSTMENT_VIEW_UNAVAILABLE"
	default:
		return ""
	}
}

// PublishStatementCommand 携带截单+发布：指名费用集合（从费用库读回，只收已确认），
// 申报总额与行勾稽不平即阻断。
type PublishStatementCommand struct {
	TenantID           domain.TenantID
	Number             string
	Account            string
	Period             string
	Version            string
	Currency           string
	ChargeIDs          []string
	DeclaredTotalMinor int64
	CutOffAt           time.Time
	PublishedAt        time.Time
}

// VoidStatementCommand 携带整单作废：依据必备、原单保留、替代单用新单号。
type VoidStatementCommand struct {
	TenantID domain.TenantID
	Number   string
	Basis    string
	VoidedAt time.Time
}

// IncludeLateChargeCommand 携带后到费用的后续账期纳入（AT-SA-055 后半）。
type IncludeLateChargeCommand struct {
	TenantID         domain.TenantID
	Inclusion        string
	Number           string
	ChargeID         string
	SubsequentPeriod string
	IncludedAt       time.Time
}

// IncludeAdjustmentCommand 携带既有调整的后续账期纳入（UC-SA-003 步 7 的调整半边，
// AT-SA-076/077）。命令只指名调整，不带金额、方向或原因——那些都在调整本体上，由它的
// 唯一创建用例形成；本用例复述一份就是第二处口径。
type IncludeAdjustmentCommand struct {
	TenantID         domain.TenantID
	Inclusion        string
	Number           string
	AdjustmentID     string
	SubsequentPeriod string
	IncludedAt       time.Time
}

// OpenDisputeCommand 携带客户异议：异议是独立对象，不改写对账单。
type OpenDisputeCommand struct {
	TenantID      domain.TenantID
	Dispute       string
	Number        string
	ChargeID      string
	DisputedMinor int64
	Reason        string
	OpenedAt      time.Time
}

// ResolveDisputeCommand 携带异议裁定。
type ResolveDisputeCommand struct {
	TenantID   domain.TenantID
	Dispute    string
	Kind       domain.DisputeResolutionKind
	Basis      string
	ResolvedAt time.Time
}

type StatementResult struct {
	outcome      StatementOutcome
	reason       StatementUndecidedReason
	statement    ports.StatementRecord
	inclusion    ports.InclusionRecord
	dispute      ports.DisputeRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result StatementResult) Outcome() StatementOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result StatementResult) UndecidedReason() StatementUndecidedReason {
	return result.reason
}

func (result StatementResult) Statement() (ports.StatementRecord, bool) {
	return result.statement, result.hasRecord && result.statement.Key.Number.String() != ""
}

func (result StatementResult) Inclusion() (ports.InclusionRecord, bool) {
	return result.inclusion, result.hasRecord && result.inclusion.Key.Inclusion.String() != ""
}

func (result StatementResult) Dispute() (ports.DisputeRecord, bool) {
	return result.dispute, result.hasRecord && result.dispute.Key.Dispute.String() != ""
}

func (result StatementResult) ContinuationReference() string {
	return result.continuation
}

// StatementHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result StatementResult) StatementHandoffReference() string {
	return result.handoff
}

// CutOffPublishStatementDeps 是截单编排的依赖。Adjustments 是只读口：本用例对调整只有
// 纳入关系的所有权（CONTEXT「调整类型与唯一所有权」表），依赖上放不下写口，理由与
// 那个只读接口分名的理由是同一条，写在 ports.ChargeAdjustmentView 上。
type CutOffPublishStatementDeps struct {
	Statements  ports.PublishedStatementStore
	Charges     ports.CustomerChargeStore
	Adjustments ports.ChargeAdjustmentView
	Inclusions  ports.SubsequentInclusionStore
	Disputes    ports.StatementDisputeStore
	Downstream  ports.StatementHandoff
	Clock       ports.Clock
}

type CutOffPublishStatementHandler struct {
	deps CutOffPublishStatementDeps
}

func NewCutOffPublishStatementHandler(deps CutOffPublishStatementDeps) *CutOffPublishStatementHandler {
	return &CutOffPublishStatementHandler{deps: deps}
}

// Publish 截单并发布：费用从库读回（只收已确认，未确认 → CHARGE_NOT_CONFIRMED 业务
// 负向）→ CutStatementDraft+PublishStatement（勾稽不平 → STATEMENT_IMBALANCE 阻断，
// 不修正为约等于）→ 幂等按（租户+单号）→ 意图交下游。
func (handler *CutOffPublishStatementHandler) Publish(
	ctx context.Context,
	command PublishStatementCommand,
) (StatementResult, error) {
	number, err := domain.NewStatementNumber(command.Number)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	key := ports.StatementKey{TenantID: command.TenantID, Number: number}
	digest := publishDigest(command)
	existing, found, err := handler.deps.Statements.FindByKey(ctx, key)
	if err != nil {
		return statementUndecided(StatementStoreUnavailable, command.Number), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一单号携带不同截单集合或总额：已发布快照不可顶替，替代走作废+新单号。
			return StatementResult{outcome: StatementConflict}, nil
		}
		return handler.existingStatement(ctx, existing), nil
	}

	charges := make([]domain.CustomerCharge, 0, len(command.ChargeIDs))
	for _, raw := range command.ChargeIDs {
		chargeID, err := domain.NewCustomerChargeID(raw)
		if err != nil {
			return StatementResult{outcome: StatementNotAccepted}, nil
		}
		charge, chargeFound, err := handler.deps.Charges.FindByID(ctx, command.TenantID, chargeID)
		if err != nil {
			return statementUndecided(StatementChargeStoreUnavailable, raw), nil
		}
		if !chargeFound {
			return StatementResult{outcome: StatementNotAccepted}, nil
		}
		charges = append(charges, charge)
	}

	draft, err := draftFrom(command, charges)
	if errors.Is(err, domain.ErrChargeNotConfirmed) {
		// 未确认费用进不了截单快照：先确认再来——后到费用归后续账期，不回填。
		return StatementResult{outcome: StatementChargeNotConfirmed}, nil
	}
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	statement, err := domain.PublishStatement(draft, number, command.DeclaredTotalMinor, command.PublishedAt)
	if errors.Is(err, domain.ErrStatementImbalance) {
		// 申报总额与行勾稽不平：阻断，不修正为「约等于」（AT-SA-065）。
		return StatementResult{outcome: StatementImbalanceOutcome}, nil
	}
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	record := ports.StatementRecord{Key: key, ContentDigest: digest, Statement: statement, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Statements.Save(ctx, record)
	if err != nil {
		return statementUndecided(StatementStoreUnavailable, command.Number), nil
	}
	switch saved {
	case ports.StatementSaved:
		result := StatementResult{outcome: StatementPublished, statement: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.StatementIntent{Statement: record}, command.Number)
		return result, nil
	case ports.StatementAlreadyPublished:
		winner, found, err := handler.deps.Statements.FindByKey(ctx, key)
		if err != nil || !found {
			return statementUndecided(StatementStoreUnavailable, command.Number), nil
		}
		return handler.existingStatement(ctx, winner), nil
	default:
		return StatementResult{}, fmt.Errorf("%w: %d", ErrUnexpectedStatementSave, saved)
	}
}

// Void 依据作废整单：原单保留（Replace 换值、版链在本体），已作废重放返原不二废。
func (handler *CutOffPublishStatementHandler) Void(
	ctx context.Context,
	command VoidStatementCommand,
) (StatementResult, error) {
	number, err := domain.NewStatementNumber(command.Number)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	basis, err := domain.NewStatementVoidBasisReference(command.Basis)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	key := ports.StatementKey{TenantID: command.TenantID, Number: number}
	existing, found, err := handler.deps.Statements.FindByKey(ctx, key)
	if err != nil {
		return statementUndecided(StatementStoreUnavailable, command.Number), nil
	}
	if !found {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	if _, _, voided := existing.Statement.Voided(); voided {
		// 已作废重放：返回原作废，不二废。
		return StatementResult{outcome: StatementAlreadyVoided, statement: existing, hasRecord: true}, nil
	}

	voidedStatement, err := existing.Statement.Void(basis, command.VoidedAt)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	existing.Statement = voidedStatement
	existing.RecordedAt = handler.deps.Clock.Now()
	ok, err := handler.deps.Statements.Replace(ctx, existing)
	if err != nil {
		return statementUndecided(StatementStoreUnavailable, command.Number), nil
	}
	if !ok {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	result := StatementResult{outcome: StatementVoidedOutcome, statement: existing, hasRecord: true}
	result.handoff = handler.handOff(ctx, ports.StatementIntent{Statement: existing}, command.Number)
	return result, nil
}

// IncludeLateCharge 把后到的已确认费用纳入后续账期并关联原账单（AT-SA-055 后半）：
// 回填原周期 → INCLUSION_BACKFILLS_PERIOD 业务负向；未确认 → CHARGE_NOT_CONFIRMED。
func (handler *CutOffPublishStatementHandler) IncludeLateCharge(
	ctx context.Context,
	command IncludeLateChargeCommand,
) (StatementResult, error) {
	inclusionRef, err := domain.NewInclusionReference(command.Inclusion)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	number, err := domain.NewStatementNumber(command.Number)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	chargeID, err := domain.NewCustomerChargeID(command.ChargeID)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	period, err := domain.NewBillingPeriodReference(command.SubsequentPeriod)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	statementRecord, found, err := handler.deps.Statements.FindByKey(ctx,
		ports.StatementKey{TenantID: command.TenantID, Number: number})
	if err != nil {
		return statementUndecided(StatementStoreUnavailable, command.Number), nil
	}
	if !found {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	charge, chargeFound, err := handler.deps.Charges.FindByID(ctx, command.TenantID, chargeID)
	if err != nil {
		return statementUndecided(StatementChargeStoreUnavailable, command.ChargeID), nil
	}
	if !chargeFound {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	inclusion, err := domain.IncludeLateChargeInSubsequentPeriod(
		statementRecord.Statement, charge, inclusionRef, period, command.IncludedAt)
	if errors.Is(err, domain.ErrInclusionBackfillsPeriod) {
		// 纳入原周期就是回填已发布快照：后到费用只能归后续账期。
		return StatementResult{outcome: InclusionBackfillsOutcome}, nil
	}
	if errors.Is(err, domain.ErrChargeNotConfirmed) {
		return StatementResult{outcome: StatementChargeNotConfirmed}, nil
	}
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	return handler.commitInclusion(ctx, inclusionCommit{
		key:       ports.InclusionKey{TenantID: command.TenantID, Inclusion: inclusionRef},
		digest:    inclusionDigest(command),
		inclusion: inclusion,
		subject:   command.Inclusion,
		formed:    LateChargeIncluded,
	})
}

// IncludeAdjustment 把既有调整纳入后续账期并关联原账单（UC-SA-003 步 7 的调整半边，
// AT-SA-076/077）。调整从它的唯一创建用例的册里**读**回来——本编排拿到的只是读口，
// 金额、方向与原因一律在调整本体上，这里不创建也不复述；调整挂在单外费用上 → 未受理
// （别人的数不进这个账户）；纳入原周期 → INCLUSION_BACKFILLS_PERIOD 业务负向。
func (handler *CutOffPublishStatementHandler) IncludeAdjustment(
	ctx context.Context,
	command IncludeAdjustmentCommand,
) (StatementResult, error) {
	inclusionRef, err := domain.NewInclusionReference(command.Inclusion)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	number, err := domain.NewStatementNumber(command.Number)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	adjustmentID, err := domain.NewChargeAdjustmentID(command.AdjustmentID)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	period, err := domain.NewBillingPeriodReference(command.SubsequentPeriod)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	statementRecord, found, err := handler.deps.Statements.FindByKey(ctx,
		ports.StatementKey{TenantID: command.TenantID, Number: number})
	if err != nil {
		return statementUndecided(StatementStoreUnavailable, command.Number), nil
	}
	if !found {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	adjustmentRecord, adjustmentFound, err := handler.deps.Adjustments.FindByKey(ctx,
		ports.ChargeAdjustmentKey{TenantID: command.TenantID, Adjustment: adjustmentID})
	if err != nil {
		return statementUndecided(AdjustmentViewUnavailable, command.AdjustmentID), nil
	}
	if !adjustmentFound {
		// 指名了不存在的调整：提交矛盾，改单重来。调整还没形成不是「等」——形成它的是
		// 另一个用例，本用例等不来它，也不替它形成（AT-SA-072）。
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	inclusion, err := domain.IncludeAdjustmentInSubsequentPeriod(
		statementRecord.Statement, adjustmentRecord.Adjustment, inclusionRef, period, command.IncludedAt)
	if errors.Is(err, domain.ErrInclusionBackfillsPeriod) {
		return StatementResult{outcome: InclusionBackfillsOutcome}, nil
	}
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	return handler.commitInclusion(ctx, inclusionCommit{
		key:       ports.InclusionKey{TenantID: command.TenantID, Inclusion: inclusionRef},
		digest:    adjustmentInclusionDigest(command),
		inclusion: inclusion,
		subject:   command.Inclusion,
		formed:    AdjustmentIncluded,
	})
}

// inclusionCommit 是两种纳入（迟到费用、既有调整）越过提交边界前共有的那几件：幂等键、
// 内容指纹、纳入本体、续办主体与成功时的结果格。两条路在领域门之前各走各的，门之后
// 的幂等/冲突/并发/意图纪律只有一份。
type inclusionCommit struct {
	key       ports.InclusionKey
	digest    string
	inclusion domain.SubsequentInclusion
	subject   string
	formed    StatementOutcome
}

// commitInclusion 提交一次纳入：同键重放按内容指纹分重放与冲突（同一纳入标识不得指向
// 另一个周期或另一笔金额对象）；并发落败读回赢家；意图交下游，投递失败不翻结果。
func (handler *CutOffPublishStatementHandler) commitInclusion(
	ctx context.Context,
	commit inclusionCommit,
) (StatementResult, error) {
	existing, alreadyIncluded, err := handler.deps.Inclusions.FindByKey(ctx, commit.key)
	if err != nil {
		return statementUndecided(InclusionStoreUnavailable, commit.subject), nil
	}
	if alreadyIncluded {
		if existing.ContentDigest != commit.digest {
			return StatementResult{outcome: InclusionConflict}, nil
		}
		result := StatementResult{outcome: InclusionExistingResult, inclusion: existing, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.StatementIntent{Inclusion: existing}, commit.subject)
		return result, nil
	}

	record := ports.InclusionRecord{Key: commit.key, ContentDigest: commit.digest, Inclusion: commit.inclusion, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Inclusions.Save(ctx, record)
	if err != nil {
		return statementUndecided(InclusionStoreUnavailable, commit.subject), nil
	}
	switch saved {
	case ports.InclusionSaved:
		result := StatementResult{outcome: commit.formed, inclusion: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.StatementIntent{Inclusion: record}, commit.subject)
		return result, nil
	case ports.InclusionAlreadyRecorded:
		winner, found, err := handler.deps.Inclusions.FindByKey(ctx, commit.key)
		if err != nil || !found {
			return statementUndecided(InclusionStoreUnavailable, commit.subject), nil
		}
		result := StatementResult{outcome: InclusionExistingResult, inclusion: winner, hasRecord: true}
		result.handoff = handler.handOff(ctx, ports.StatementIntent{Inclusion: winner}, commit.subject)
		return result, nil
	default:
		return StatementResult{}, fmt.Errorf("%w: %d", ErrUnexpectedStatementSave, saved)
	}
}

// OpenDispute 开立客户异议：异议是独立对象、限单内行、金额不越行（领域把门），不改写
// 对账单。
func (handler *CutOffPublishStatementHandler) OpenDispute(
	ctx context.Context,
	command OpenDisputeCommand,
) (StatementResult, error) {
	disputeID, err := domain.NewDisputeID(command.Dispute)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	number, err := domain.NewStatementNumber(command.Number)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	chargeID, err := domain.NewCustomerChargeID(command.ChargeID)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	reason, err := domain.NewDisputeBasisReference(command.Reason)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	statementRecord, found, err := handler.deps.Statements.FindByKey(ctx,
		ports.StatementKey{TenantID: command.TenantID, Number: number})
	if err != nil {
		return statementUndecided(StatementStoreUnavailable, command.Number), nil
	}
	if !found {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	dispute, err := domain.OpenStatementDispute(
		statementRecord.Statement, chargeID, command.DisputedMinor, reason, disputeID, command.OpenedAt)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	key := ports.DisputeKey{TenantID: command.TenantID, Dispute: disputeID}
	digest := disputeDigest(command)
	existing, opened, err := handler.deps.Disputes.FindByKey(ctx, key)
	if err != nil {
		return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
	}
	if opened {
		if existing.ContentDigest != digest {
			return StatementResult{outcome: DisputeConflict}, nil
		}
		return StatementResult{outcome: DisputeExistingResult, dispute: existing, hasRecord: true}, nil
	}

	record := ports.DisputeRecord{Key: key, ContentDigest: digest, Dispute: dispute, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Disputes.Save(ctx, record)
	if err != nil {
		return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
	}
	switch saved {
	case ports.DisputeSaved:
		return StatementResult{outcome: DisputeOpened, dispute: record, hasRecord: true}, nil
	case ports.DisputeAlreadyOpened:
		winner, found, err := handler.deps.Disputes.FindByKey(ctx, key)
		if err != nil || !found {
			return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
		}
		return StatementResult{outcome: DisputeExistingResult, dispute: winner, hasRecord: true}, nil
	default:
		return StatementResult{}, fmt.Errorf("%w: %d", ErrUnexpectedStatementSave, saved)
	}
}

// ResolveDispute 裁定异议：待审可再裁、终局不二裁（领域哨兵 ErrDisputeResolved 分格）。
func (handler *CutOffPublishStatementHandler) ResolveDispute(
	ctx context.Context,
	command ResolveDisputeCommand,
) (StatementResult, error) {
	disputeID, err := domain.NewDisputeID(command.Dispute)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	basis, err := domain.NewDisputeBasisReference(command.Basis)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	key := ports.DisputeKey{TenantID: command.TenantID, Dispute: disputeID}
	existing, found, err := handler.deps.Disputes.FindByKey(ctx, key)
	if err != nil {
		return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
	}
	if !found {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	resolved, err := existing.Dispute.Resolve(command.Kind, basis, command.ResolvedAt)
	if errors.Is(err, domain.ErrDisputeResolved) {
		// 终局裁定不二裁：返回既有裁定——改判不在这里发生。
		return StatementResult{outcome: DisputeAlreadyResolved, dispute: existing, hasRecord: true}, nil
	}
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	existing.Dispute = resolved
	existing.RecordedAt = handler.deps.Clock.Now()
	ok, err := handler.deps.Disputes.Replace(ctx, existing)
	if err != nil {
		return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
	}
	if !ok {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	return StatementResult{outcome: DisputeResolvedOutcome, dispute: existing, hasRecord: true}, nil
}

func draftFrom(command PublishStatementCommand, charges []domain.CustomerCharge) (domain.StatementDraft, error) {
	spec := domain.StatementDraftSpec{
		CutOffAt: command.CutOffAt,
		Charges:  charges,
	}
	var err error
	if spec.Account, err = domain.NewSettlementAccountID(command.Account); err != nil {
		return domain.StatementDraft{}, err
	}
	if spec.Period, err = domain.NewBillingPeriodReference(command.Period); err != nil {
		return domain.StatementDraft{}, err
	}
	if spec.Version, err = domain.NewStatementDraftVersion(command.Version); err != nil {
		return domain.StatementDraft{}, err
	}
	if spec.Currency, err = domain.NewCurrencyCode(command.Currency); err != nil {
		return domain.StatementDraft{}, err
	}
	return domain.CutStatementDraft(spec)
}

func statementUndecided(reason StatementUndecidedReason, subject string) StatementResult {
	return StatementResult{
		outcome:      StatementUndecided,
		reason:       reason,
		continuation: statementContinuation(reason.String(), subject),
	}
}

// existingStatement 按已有发布作答并重发同一份意图。
func (handler *CutOffPublishStatementHandler) existingStatement(
	ctx context.Context,
	record ports.StatementRecord,
) StatementResult {
	return StatementResult{
		outcome:   StatementExistingResult,
		statement: record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, ports.StatementIntent{Statement: record}, record.Key.Number.String()),
	}
}

// handOff 交发布意图。投递失败不翻结果，留续办引用重放时重发同一份。
func (handler *CutOffPublishStatementHandler) handOff(
	ctx context.Context,
	intent ports.StatementIntent,
	subject string,
) string {
	if err := handler.deps.Downstream.HandOffStatement(ctx, intent); err == nil {
		return ""
	}
	return statementContinuation("STATEMENT_HANDOFF", subject)
}

func statementContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

func publishDigest(command PublishStatementCommand) string {
	charges := append([]string(nil), command.ChargeIDs...)
	sort.Strings(charges)
	digest := sha256.Sum256([]byte(strings.Join(append([]string{
		command.Account,
		command.Period,
		command.Version,
		command.Currency,
		fmt.Sprintf("%d", command.DeclaredTotalMinor),
		command.CutOffAt.UTC().Format(time.RFC3339Nano),
	}, charges...), "\x00")))
	return hex.EncodeToString(digest[:])
}

func inclusionDigest(command IncludeLateChargeCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Number,
		command.ChargeID,
		command.SubsequentPeriod,
		command.IncludedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// adjustmentInclusionDigest 与 inclusionDigest 分开算：两种纳入指向的金额对象不同类
// （费用 / 调整），同一个纳入标识先纳费用再纳调整是冲突，不能因为两段字符串恰好相同而
// 被读成重放。
func adjustmentInclusionDigest(command IncludeAdjustmentCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"adjustment",
		command.Number,
		command.AdjustmentID,
		command.SubsequentPeriod,
		command.IncludedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

func disputeDigest(command OpenDisputeCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Number,
		command.ChargeID,
		fmt.Sprintf("%d", command.DisputedMinor),
		command.Reason,
		command.OpenedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

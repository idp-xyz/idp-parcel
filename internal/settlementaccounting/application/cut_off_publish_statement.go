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

// ErrStatementNumberingTimeZoneMissing 说明要发布的对账单取不到单号：结算账户读不到对账安排，租户也没登单据编号时区。
// 对账单号非空、是寻址键，不能像别的单据那样不带号开立；而能入单的费用都有归属日、归属日只在对账安排之下形成
// （ADR-0355 决定七），走到这一步就是两侧说法矛盾，交回错误、整笔回滚。
var ErrStatementNumberingTimeZoneMissing = errors.New("settlement accounting: the statement account has no business time zone to number the statement")

// ErrNumberedIssueInterrupted 说明取到单据号之后、单据落库之前出了岔子（库写不进，或撞上了本该排队的同一开立意图）。
// 编排交回错误而不是未决答复：调用方据此回滚开立事务，号随之释放；答一个未决而让事务照常提交，取走的号就成了空号
// （ADR-0355 决定三）。
var ErrNumberedIssueInterrupted = errors.New("settlement accounting: issuing stopped after a document number was taken")

// StatementOutcome 是对账单编排的应用处理结果。业务负向格（ADR-0029）：勾稽不平
// （改申报总额再来）、费用未确认（先确认再截单）、纳入回填（只能归后续账期）、本期无费用
// （列出的费用都晚于本期或已出账，换本期或换清单再来）——都是已知答案，不是改单形状也不是
// 等依赖。截单未配置是另一种恢复动作：补登对账安排或归属日形态之后再截。
type StatementOutcome uint8

const (
	StatementOutcomeInvalid StatementOutcome = iota
	StatementPublished
	StatementExistingResult
	// StatementPublicationIntentConflict：同一发布意图（账户、周期、截止版本）已发布过另一份内容（ADR-0355 决定九）。
	StatementPublicationIntentConflict
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
	StatementNoChargeInPeriod
	StatementCutUnconfigured
	// DisputeOnVoidedStatement：对一张已作废的单登记争议（ADR-0354 决定三）。是答案不是故障：单已不应继续有效，异议要对着替代单提。
	DisputeOnVoidedStatement
	// StatementNumberingRuleCollision：发布对账单或登记争议时，单据编号规则跳号到上限仍撞上已用的号（ADR-0355 决定八）。是答案不是
	// 故障：这一次不落单，要去规则册改格式。调用方必须回滚这一笔事务，取过的流水才释放。
	StatementNumberingRuleCollision
)

func (outcome StatementOutcome) String() string {
	switch outcome {
	case StatementPublished:
		return "STATEMENT_PUBLISHED"
	case StatementExistingResult:
		return "EXISTING_STATEMENT"
	case StatementPublicationIntentConflict:
		return "PUBLICATION_INTENT_CONFLICT"
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
	case StatementNoChargeInPeriod:
		return "NO_CHARGE_IN_PERIOD"
	case StatementCutUnconfigured:
		return "STATEMENT_CUT_UNCONFIGURED"
	case DisputeOnVoidedStatement:
		return "STATEMENT_VOIDED_TAKES_NO_DISPUTE"
	case StatementNumberingRuleCollision:
		return "NUMBERING_RULE_COLLISION"
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
	StatementAttributionViewUnavailable
	// StatementArrangementViewUnavailable：读不动结算账户的对账安排，定不了单据号的日期时区。
	StatementArrangementViewUnavailable
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
	case StatementAttributionViewUnavailable:
		return "ATTRIBUTION_VIEW_UNAVAILABLE"
	case StatementArrangementViewUnavailable:
		return "ARRANGEMENT_VIEW_UNAVAILABLE"
	default:
		return ""
	}
}

// PublishStatementCommand 携带截单+发布：指名候选费用集合（从费用库读回，只收已确认），点名本期最后
// 一个归属日（YYYY-MM-DD）——列出的费用里只有归属日不晚于它、尚未出账的入本期，其余逐件指名，不按
// 列出的清单放行（ADR-0269 决定四）。申报总额与入本期的行勾稽，不平即阻断。CutOffAt 是截单这一动作
// 的业务时点，记在草稿上，不参与判哪一件入本期。
//
// 命令里没有单号：单号在发布事务里按编号规则取得（ADR-0355 决定九）。账户、周期与截止版本（Version）三格就是
// 发布意图，幂等按它判。
type PublishStatementCommand struct {
	TenantID            domain.TenantID
	Account             string
	Period              string
	Version             string
	Currency            string
	ChargeIDs           []string
	LastAttributionDate string
	DeclaredTotalMinor  int64
	CutOffAt            time.Time
	PublishedAt         time.Time
}

// StatementExclusionCause 是命令列出的一件费用没入本期的缘由。
type StatementExclusionCause uint8

const (
	StatementExclusionCauseInvalid StatementExclusionCause = iota
	// ExcludedAfterPeriod：归属日晚于本期最后一个归属日，归后续账期。
	ExcludedAfterPeriod
	// ExcludedAlreadyBilled：已在一张未作废的已发布对账单上，不再纳入。
	ExcludedAlreadyBilled
	// ExcludedAttributionUnconfigured：没有归属日、当场也判不出——对账安排或归属日形态没登，Missing 指名
	// 缺哪一册。码沿用归属日判定的那一格，截单不另起一个词。
	ExcludedAttributionUnconfigured
	// ExcludedSourceInstantMissing：两册都登了仍判不出——形态要来源发生时点，这件费用上没有。补登哪一册都
	// 补不上，不计入未配置；只拿掉这一件，其余照常入本期（ADR-0029：调用方要从取值里读出接下来做什么）。
	ExcludedSourceInstantMissing
)

func (cause StatementExclusionCause) String() string {
	switch cause {
	case ExcludedAfterPeriod:
		return "ATTRIBUTED_AFTER_PERIOD"
	case ExcludedAlreadyBilled:
		return "ALREADY_BILLED"
	case ExcludedAttributionUnconfigured:
		return ChargeAttributionUnconfigured.String()
	case ExcludedSourceInstantMissing:
		return "SOURCE_INSTANT_MISSING"
	default:
		return ""
	}
}

// StatementExclusion 逐件指名一件没入本期的费用。AttributionDate 只在晚于本期时有，BilledOn 只在已出账时有，
// Missing 只在归属日判不出时有。
type StatementExclusion struct {
	Charge          domain.CustomerChargeID
	Cause           StatementExclusionCause
	AttributionDate domain.AttributionDate
	BilledOn        domain.StatementNumber
	Missing         []domain.AttributionRegister
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
	// OperationEvidence 记经哪个渠道、由谁代登记（运营决定口是 `OPERATOR/<发行方>#<sub>`，ADR-0354 决定四）。只作审计：不进摘要、
	// 不进重放比对；编排直调与存量行为空。
	OperationEvidence string
}

// ResolveDisputeCommand 携带异议裁定。
type ResolveDisputeCommand struct {
	TenantID   domain.TenantID
	Dispute    string
	Kind       domain.DisputeResolutionKind
	Basis      string
	ResolvedAt time.Time
	// OperationEvidence 记由谁形成这一次处理，判据同 OpenDisputeCommand 的同名格。
	OperationEvidence string
}

type StatementResult struct {
	outcome      StatementOutcome
	reason       StatementUndecidedReason
	exclusions   []StatementExclusion
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

// Exclusions 逐件指名这一次截单里没入本期的费用，按命令列出的次序。只在本次截单走到判纳入那一步时有：
// 发布、勾稽不平、本期无费用与截单未配置。重放答已有对账单，不重判——此刻再判可能已是另一个答案。
func (result StatementResult) Exclusions() []StatementExclusion {
	return append([]StatementExclusion(nil), result.exclusions...)
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
// 那个只读接口分名的理由是同一条，写在 ports.ChargeAdjustmentView 上。Attributions 与
// 确认编排用的是同一份判定；AttributionDates 只落截单当场判出的那几件。Numbers 给对账单与
// 争议取单据号，Arrangements 给它们定日期所用的结算账户业务时区。
type CutOffPublishStatementDeps struct {
	Statements       ports.PublishedStatementStore
	Charges          ports.CustomerChargeStore
	Billing          ports.ChargeBillingLock
	Attributions     *JudgeChargeAttributionHandler
	AttributionDates ports.ChargeAttributionDateRecorder
	Adjustments      ports.ChargeAdjustmentView
	Inclusions       ports.SubsequentInclusionStore
	Disputes         ports.StatementDisputeStore
	Numbers          ports.DocumentNumberAssigner
	Arrangements     ports.StatementArrangementView
	Downstream       ports.StatementHandoff
	Clock            ports.Clock
}

type CutOffPublishStatementHandler struct {
	deps CutOffPublishStatementDeps
}

// NewCutOffPublishStatementHandler 在装配时核齐依赖：缺件在这里报，不等到截单那一刻才撞空指针。
func NewCutOffPublishStatementHandler(deps CutOffPublishStatementDeps) (*CutOffPublishStatementHandler, error) {
	if deps.Statements == nil || deps.Charges == nil || deps.Billing == nil || deps.Attributions == nil ||
		deps.AttributionDates == nil || deps.Adjustments == nil || deps.Inclusions == nil ||
		deps.Disputes == nil || deps.Numbers == nil || deps.Arrangements == nil ||
		deps.Downstream == nil || deps.Clock == nil {
		return nil, fmt.Errorf("%w: cut-off publish statement", ErrNilDependency)
	}
	return &CutOffPublishStatementHandler{deps: deps}, nil
}

// Publish 截单并发布：排队同一发布意图 → 按意图查重（同内容重放答原单与原号，异内容答发布意图冲突）→ 费用从库读回
// （只收已确认，未确认 → CHARGE_NOT_CONFIRMED 业务负向）→ 逐件定是否入本期（admit）→ 草稿勾稽（不平 →
// STATEMENT_IMBALANCE 阻断，不修正为约等于）→ 取单号 → 落单 → 意图交下游。
//
// 单号只在勾稽过了、确定要落单的那一步取（ADR-0355 决定三）：本期无费用、勾稽不平、截单未配置都答业务答案而照常
// 提交（自动截单还照记截止日），那几路先取号就留下空号。取到号之后再出岔子一律交回错误，让调用方回滚、号随之释放。
func (handler *CutOffPublishStatementHandler) Publish(
	ctx context.Context,
	command PublishStatementCommand,
) (StatementResult, error) {
	last, err := domain.ParseAttributionDate(command.LastAttributionDate)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	intent, err := publicationIntentOf(command)
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	subject := intentSubject(intent)

	digest := publishDigest(command)
	if err := handler.deps.Statements.LockPublicationIntent(ctx, intent); err != nil {
		return statementUndecided(StatementStoreUnavailable, subject), nil
	}
	existing, found, err := handler.findPublished(ctx, intent, command, digest)
	if err != nil {
		return statementUndecided(StatementStoreUnavailable, subject), nil
	}
	if found {
		return handler.answerExisting(ctx, existing, command, digest, subject), nil
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
		if charge.Stage() != domain.ChargeConfirmed {
			// 未确认费用进不了截单快照：先确认再来——后到费用归后续账期，不回填。先于判归属日拒，
			// 未确认的费用没有确认时点也没有所属账户，判不出的缘由会被错指成缺册。
			return StatementResult{outcome: StatementChargeNotConfirmed}, nil
		}
		if facts, _ := charge.ConfirmedFacts(); facts.SettlementAccount != intent.Account {
			// 列了别户的费用是提交矛盾，整笔拒。先于判归属日：否则会照别户的对账安排判出一个归属日并落库。
			return StatementResult{outcome: StatementNotAccepted}, nil
		}
		charges = append(charges, charge)
	}

	admission, stopped, stop := handler.admit(ctx, command, subject, charges, last)
	if stop {
		return stopped, nil
	}
	if len(admission.inPeriod) == 0 {
		// 一件都入不了本期不形成草稿。有判不出归属日的，整笔答未配置——补登之后再截，那几件可能就入了本期；
		// 没有的，答本期无费用，不答未配置：补登什么都改变不了这个答案（ADR-0269 决定四）。
		if admission.unconfigured {
			return StatementResult{outcome: StatementCutUnconfigured, exclusions: admission.exclusions}, nil
		}
		return StatementResult{outcome: StatementNoChargeInPeriod, exclusions: admission.exclusions}, nil
	}

	draft, err := draftFrom(command, admission.inPeriod, last)
	if errors.Is(err, domain.ErrChargeNotConfirmed) {
		return StatementResult{outcome: StatementChargeNotConfirmed}, nil
	}
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}
	if err := draft.CheckPublication(command.DeclaredTotalMinor, command.PublishedAt); errors.Is(err, domain.ErrStatementImbalance) {
		// 申报总额与入本期的行勾稽不平：阻断，不修正为「约等于」（AT-SA-065）。不入本期的那几件照样指名，
		// 申报方据此知道差在哪几件。
		return StatementResult{outcome: StatementImbalanceOutcome, exclusions: admission.exclusions}, nil
	} else if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	zone, zoneRead := handler.accountTimeZone(ctx, command.TenantID, intent.Account)
	if !zoneRead {
		return statementUndecided(StatementArrangementViewUnavailable, subject), nil
	}
	assigned, err := handler.deps.Numbers.AssignDocumentNumber(ctx, ports.DocumentNumberRequest{
		Tenant:         command.TenantID,
		Type:           domain.CustomerStatementDocumentNumberType.Code,
		IssuerTimeZone: zone,
		IsUsed: func(ctx context.Context, number string) (bool, error) {
			return handler.deps.Statements.StatementNumberUsed(ctx, command.TenantID, number)
		},
	})
	if err != nil {
		return StatementResult{}, fmt.Errorf("settlement accounting: number statement %s: %w", subject, err)
	}
	switch assigned.Outcome {
	case ports.DocumentNumberAssigned:
	case ports.DocumentNumberRuleCollision:
		return StatementResult{outcome: StatementNumberingRuleCollision, exclusions: admission.exclusions}, nil
	case ports.DocumentNumberTimeZoneUnconfigured:
		return StatementResult{}, fmt.Errorf("%w: %s", ErrStatementNumberingTimeZoneMissing, subject)
	default:
		return StatementResult{}, fmt.Errorf("settlement accounting: number statement %s: unnamed outcome %d", subject, assigned.Outcome)
	}
	number, err := domain.NewStatementNumber(assigned.Number)
	if err != nil {
		return StatementResult{}, fmt.Errorf("settlement accounting: number statement %s: %w", subject, err)
	}
	statement, err := domain.PublishStatement(draft, number, command.DeclaredTotalMinor, command.PublishedAt)
	if err != nil {
		// 勾稽与时刻上面已判过：这里失败是号已取走之后的矛盾，交回错误让事务回滚。
		return StatementResult{}, fmt.Errorf("settlement accounting: publish numbered statement %s: %w", number, err)
	}

	record := ports.StatementRecord{
		Key:           ports.StatementKey{TenantID: command.TenantID, Number: number},
		ContentDigest: digest,
		Statement:     statement,
		DraftVersion:  intent.Version,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Statements.Save(ctx, record)
	if err != nil {
		return StatementResult{}, fmt.Errorf("%w: save statement %s: %w", ErrNumberedIssueInterrupted, number, err)
	}
	switch saved {
	case ports.StatementSaved:
		result := StatementResult{outcome: StatementPublished, statement: record, hasRecord: true, exclusions: admission.exclusions}
		result.handoff = handler.handOff(ctx, ports.StatementIntent{Statement: record}, number.String())
		return result, nil
	case ports.StatementAlreadyPublished:
		// 同一意图已在 LockPublicationIntent 上排了队，走到这里说明有人没排队就写了库。号已取走，读回赢家作答会让这个号
		// 随事务提交成为空号，所以交回错误、整笔回滚，重来时按意图查到赢家。
		return StatementResult{}, fmt.Errorf("%w: statement %s was saved by another transaction", ErrNumberedIssueInterrupted, subject)
	default:
		return StatementResult{}, fmt.Errorf("%w: %d", ErrUnexpectedStatementSave, saved)
	}
}

// publicationIntentOf 把命令里的账户、周期与截止版本收成发布意图；任一格形状不成立即未受理。
func publicationIntentOf(command PublishStatementCommand) (ports.StatementPublicationIntent, error) {
	intent := ports.StatementPublicationIntent{TenantID: command.TenantID}
	var err error
	if intent.Account, err = domain.NewSettlementAccountID(command.Account); err != nil {
		return ports.StatementPublicationIntent{}, err
	}
	if intent.Period, err = domain.NewBillingPeriodReference(command.Period); err != nil {
		return ports.StatementPublicationIntent{}, err
	}
	if intent.Version, err = domain.NewStatementDraftVersion(command.Version); err != nil {
		return ports.StatementPublicationIntent{}, err
	}
	return intent, nil
}

// intentSubject 是续办引用与错误正文里指这一次发布意图的那一串。
func intentSubject(intent ports.StatementPublicationIntent) string {
	return intent.Account.String() + "/" + intent.Period.String() + "/" + intent.Version.String()
}

// findPublished 按发布意图找已有的单。先按记下的截止版本找；找不到，再看本决定之前手工发布、没记下截止版本的同账户
// 同账期的单（ADR-0355 Consequences：之前截出的单按意图同样查得到）。那几张认不出截止版本，取保守：内容相同（摘要里
// 含截止版本，相同即同一意图）就是重放，交回那一张答原单原号；内容不同而同账户同账期还有一张未作废的这种旧单，也交回
// 它——answerExisting 比出内容不同，答发布意图冲突、不落单不取号，宁可让人换账期或先作废旧单，也不在同一账期旁边悄悄再发
// 一张。旧单都已作废时不挡：作废之后要替代单重新出具（CONTEXT「已作废 → 已替代」）。
func (handler *CutOffPublishStatementHandler) findPublished(
	ctx context.Context,
	intent ports.StatementPublicationIntent,
	command PublishStatementCommand,
	digest string,
) (ports.StatementRecord, bool, error) {
	record, found, err := handler.deps.Statements.FindByIntent(ctx, intent)
	if err != nil || found {
		return record, found, err
	}
	legacy, err := handler.deps.Statements.FindUnversionedInPeriod(ctx, intent.TenantID, intent.Account, intent.Period)
	if err != nil {
		return ports.StatementRecord{}, false, err
	}
	for _, candidate := range legacy {
		if domain.CompareStoredPublishDigest(candidate.ContentDigest, digest, sac1PublishDigest(command), unversionedPublishDigest(command)) == domain.SamePayload {
			return candidate, true, nil
		}
	}
	for _, candidate := range legacy {
		if _, _, voided := candidate.Statement.Voided(); !voided {
			return candidate, true, nil
		}
	}
	return ports.StatementRecord{}, false, nil
}

// accountTimeZone 读对账单号与争议号日期所用的业务时区：结算账户对账安排最新一版的业务时区（ADR-0269、ADR-0355
// 决定七）。账户没登对账安排答空串，由编号组件退到租户的单据编号时区；第二个返回值为假是读不动。
func (handler *CutOffPublishStatementHandler) accountTimeZone(
	ctx context.Context,
	tenant domain.TenantID,
	account domain.SettlementAccountID,
) (string, bool) {
	arrangement, found, err := handler.deps.Arrangements.LoadStatementArrangement(ctx, tenant, account)
	if err != nil {
		return "", false
	}
	if !found {
		return "", true
	}
	return arrangement.TimeZone(), true
}

// answerExisting 按同一发布意图已有的那张单作答：同内容是重放，答原单与原号；异内容是发布意图冲突——已发布快照
// 不可顶替，替代走作废+新单号。
func (handler *CutOffPublishStatementHandler) answerExisting(
	ctx context.Context,
	existing ports.StatementRecord,
	command PublishStatementCommand,
	digest string,
	subject string,
) StatementResult {
	switch domain.CompareStoredPublishDigest(existing.ContentDigest, digest, sac1PublishDigest(command), unversionedPublishDigest(command)) {
	case domain.DifferentPayload:
		return StatementResult{outcome: StatementPublicationIntentConflict}
	case domain.UnknownPayloadShape:
		return statementUndecided(StatementStoreUnavailable, subject)
	}
	return handler.existingStatement(ctx, existing)
}

// cutAdmission 是命令列出的已确认费用逐件定下的去向：入本期的按列出次序，没入的逐件指名。
type cutAdmission struct {
	inPeriod     []domain.CustomerCharge
	exclusions   []StatementExclusion
	unconfigured bool
}

// admit 逐件定命令列出的费用是否入本期：已出账的不再纳入；没有归属日的当场判，判出即落库，判不出的指名
// 缺哪一册；归属日晚于本期最后一个归属日的归后续账期（ADR-0269 决定三、决定四）。交回的布尔为真时
// 截单就此作答，答复是交回的 StatementResult。
//
// 同一发布意图的并发重放已在 LockPublicationIntent 上排过队，这里读到的「已出账」都是别的意图的单。
func (handler *CutOffPublishStatementHandler) admit(
	ctx context.Context,
	command PublishStatementCommand,
	subject string,
	charges []domain.CustomerCharge,
	last domain.AttributionDate,
) (cutAdmission, StatementResult, bool) {
	// 出账锁按费用标识的次序取：两笔截单以不同次序列了同几件费用，按列出次序加锁会互等成死锁。
	order := make([]int, len(charges))
	for index := range order {
		order[index] = index
	}
	sort.Slice(order, func(a, b int) bool { return charges[order[a]].ID().String() < charges[order[b]].ID().String() })
	billedOn := make(map[int]domain.StatementNumber, len(charges))
	for _, index := range order {
		billed, isBilled, err := handler.deps.Billing.LockChargeBilling(ctx, command.TenantID, charges[index].ID())
		if err != nil {
			return cutAdmission{}, statementUndecided(StatementStoreUnavailable, subject), true
		}
		if isBilled {
			billedOn[index] = billed
		}
	}

	var admission cutAdmission
	for index, charge := range charges {
		if billed, isBilled := billedOn[index]; isBilled {
			admission.exclusions = append(admission.exclusions,
				StatementExclusion{Charge: charge.ID(), Cause: ExcludedAlreadyBilled, BilledOn: billed})
			continue
		}
		date, attributed := charge.AttributionDate()
		if !attributed {
			judged := handler.deps.Attributions.JudgeConfirmedCharge(ctx, command.TenantID, charge)
			switch judged.Outcome() {
			case ChargeAttributionJudged:
				date, _ = judged.Date()
			case ChargeAttributionUnconfigured:
				admission.unconfigured = true
				admission.exclusions = append(admission.exclusions, StatementExclusion{
					Charge: charge.ID(), Cause: ExcludedAttributionUnconfigured, Missing: judged.Missing(),
				})
				continue
			case ChargeAttributionViewUnavailable:
				return cutAdmission{}, statementUndecided(StatementAttributionViewUnavailable, charge.ID().String()), true
			default:
				// 册都登了仍判不出，只剩形态要的时点费用上没有：费用本身缺格，不当缺册指名，也不计入未配置。
				admission.exclusions = append(admission.exclusions,
					StatementExclusion{Charge: charge.ID(), Cause: ExcludedSourceInstantMissing})
				continue
			}
			recorded, err := handler.deps.AttributionDates.RecordAttributionDate(ctx, command.TenantID, charge.ID(), date)
			if err != nil {
				return cutAdmission{}, statementUndecided(StatementChargeStoreUnavailable, charge.ID().String()), true
			}
			if charge, err = charge.WithAttributionDate(recorded); err != nil {
				return cutAdmission{}, StatementResult{outcome: StatementNotAccepted}, true
			}
			date = recorded
		}
		if date.After(last) {
			admission.exclusions = append(admission.exclusions,
				StatementExclusion{Charge: charge.ID(), Cause: ExcludedAfterPeriod, AttributionDate: date})
			continue
		}
		admission.inPeriod = append(admission.inPeriod, charge)
	}
	return admission, StatementResult{}, false
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
		key:         ports.InclusionKey{TenantID: command.TenantID, Inclusion: inclusionRef},
		digest:      inclusionDigest(command),
		unversioned: unversionedInclusionDigest(command),
		inclusion:   inclusion,
		subject:     command.Inclusion,
		formed:      LateChargeIncluded,
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
		key:         ports.InclusionKey{TenantID: command.TenantID, Inclusion: inclusionRef},
		digest:      adjustmentInclusionDigest(command),
		unversioned: unversionedAdjustmentInclusionDigest(command),
		inclusion:   inclusion,
		subject:     command.Inclusion,
		formed:      AdjustmentIncluded,
	})
}

// inclusionCommit 是两种纳入（迟到费用、既有调整）越过提交边界前共有的那几件：幂等键、
// 内容指纹、纳入本体、续办主体与成功时的结果格。两条路在领域门之前各走各的，门之后
// 的幂等/冲突/并发/意图纪律只有一份。
type inclusionCommit struct {
	key         ports.InclusionKey
	digest      string
	unversioned string
	inclusion   domain.SubsequentInclusion
	subject     string
	formed      StatementOutcome
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
		switch domain.CompareStoredDigest(existing.ContentDigest, commit.digest, commit.unversioned) {
		case domain.DifferentPayload:
			return StatementResult{outcome: InclusionConflict}, nil
		case domain.UnknownPayloadShape:
			return statementUndecided(InclusionStoreUnavailable, commit.subject), nil
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
// 对账单。登记时在同一事务里取争议号（ADR-0355）：先排队同一争议标识，再按标识查重——重放读回原来那一项连同它的
// 争议号，不再取号；领域门拒了的（未受理、单已作废）不取号。
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
		statementRecord.Statement, chargeID, command.DisputedMinor, reason, disputeID, command.OpenedAt, handler.deps.Clock.Now())
	if errors.Is(err, domain.ErrDisputeOnVoidedStatement) {
		return StatementResult{outcome: DisputeOnVoidedStatement}, nil
	}
	if err != nil {
		return StatementResult{outcome: StatementNotAccepted}, nil
	}

	key := ports.DisputeKey{TenantID: command.TenantID, Dispute: disputeID}
	digest := disputeDigest(command)
	// 同一争议标识的两份并发登记在这里排队：输的那一份等赢的那一份提交之后才往下读，读到的就是赢家，按内容答已存在
	// 或冲突（AT-SA-228），不会两份都取了号、输家再撞主键。
	if err := handler.deps.Disputes.LockRegistration(ctx, key); err != nil {
		return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
	}
	existing, opened, err := handler.deps.Disputes.FindByKey(ctx, key)
	if err != nil {
		return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
	}
	if opened {
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversionedDisputeDigest(command)) {
		case domain.DifferentPayload:
			return StatementResult{outcome: DisputeConflict}, nil
		case domain.UnknownPayloadShape:
			return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
		}
		return StatementResult{outcome: DisputeExistingResult, dispute: existing, hasRecord: true}, nil
	}

	zone, zoneRead := handler.accountTimeZone(ctx, command.TenantID, statementRecord.Statement.Account())
	if !zoneRead {
		return statementUndecided(StatementArrangementViewUnavailable, command.Dispute), nil
	}
	numbering, collided, err := issueDocumentNumber(ctx, handler.deps.Numbers, ports.DocumentNumberRequest{
		Tenant:         command.TenantID,
		Type:           domain.StatementDisputeDocumentNumberType.Code,
		IssuerTimeZone: zone,
		IsUsed: func(ctx context.Context, number string) (bool, error) {
			return handler.deps.Disputes.DocumentNumberUsed(ctx, command.TenantID, number)
		},
	})
	if err != nil {
		return StatementResult{}, fmt.Errorf("settlement accounting: number dispute %s: %w", command.Dispute, err)
	}
	if collided {
		return StatementResult{outcome: StatementNumberingRuleCollision}, nil
	}

	record := ports.DisputeRecord{
		Key: key, ContentDigest: digest, Dispute: dispute, RecordedAt: handler.deps.Clock.Now(),
		OperationEvidence: command.OperationEvidence, Numbering: numbering,
	}
	saved, err := handler.deps.Disputes.Save(ctx, record)
	if err != nil {
		if _, numbered := numbering.Number(); numbered {
			return StatementResult{}, fmt.Errorf("%w: save dispute %s: %w", ErrNumberedIssueInterrupted, command.Dispute, err)
		}
		return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
	}
	switch saved {
	case ports.DisputeSaved:
		return StatementResult{outcome: DisputeOpened, dispute: record, hasRecord: true}, nil
	case ports.DisputeAlreadyOpened:
		// 同一标识已在 LockRegistration 上排过队，走到这里说明有人没排队就写了库。没取到号时读回赢家、按内容判已存在或
		// 冲突，与读时就已在册同一个判法；取到了号就交回错误整笔回滚——读回赢家作答会让这个号随事务提交成为空号。
		if _, numbered := numbering.Number(); numbered {
			return StatementResult{}, fmt.Errorf("%w: dispute %s was saved by another transaction", ErrNumberedIssueInterrupted, command.Dispute)
		}
		winner, found, err := handler.deps.Disputes.FindByKey(ctx, key)
		if err != nil || !found {
			return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
		}
		switch domain.CompareStoredDigest(winner.ContentDigest, digest, unversionedDisputeDigest(command)) {
		case domain.DifferentPayload:
			return StatementResult{outcome: DisputeConflict}, nil
		case domain.UnknownPayloadShape:
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
	existing.ResolutionEvidence = command.OperationEvidence
	ok, err := handler.deps.Disputes.Replace(ctx, existing)
	if err != nil {
		return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
	}
	if !ok {
		// 读与写之间另一人先落了终局：库上的条件挡住了这一笔，答那一份终局，与读时就已终局同一个答案——不答未受理，
		// 那会让后到的人以为自己的输入有错。
		winner, found, err := handler.deps.Disputes.FindByKey(ctx, key)
		if err != nil || !found {
			return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
		}
		if kind, _, _, resolved := winner.Dispute.Resolution(); resolved && kind != domain.DisputePendingReview {
			return StatementResult{outcome: DisputeAlreadyResolved, dispute: winner, hasRecord: true}, nil
		}
		return statementUndecided(DisputeStoreUnavailable, command.Dispute), nil
	}
	return StatementResult{outcome: DisputeResolvedOutcome, dispute: existing, hasRecord: true}, nil
}

func draftFrom(command PublishStatementCommand, charges []domain.CustomerCharge, last domain.AttributionDate) (domain.StatementDraft, error) {
	spec := domain.StatementDraftSpec{
		CutOffAt:            command.CutOffAt,
		LastAttributionDate: last,
		Charges:             charges,
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
	return settlementCanonicalDigest(domain.CanonicalizePublishStatementPayloadByAttributionDate(
		command.Account, command.Period, command.Version, command.Currency, command.DeclaredTotalMinor, command.CutOffAt,
		command.LastAttributionDate, command.ChargeIDs))
}

// sac1PublishDigest 按 SAC-1 那一版算本次命令：只用于与已存的 SAC-1 发布摘要比对（domain.CompareStoredPublishDigest）。
func sac1PublishDigest(command PublishStatementCommand) string {
	return settlementCanonicalDigest(domain.CanonicalizePublishStatementPayload(
		command.Account, command.Period, command.Version, command.Currency, command.DeclaredTotalMinor, command.CutOffAt, command.ChargeIDs))
}

func inclusionDigest(command IncludeLateChargeCommand) string {
	return settlementCanonicalDigest(domain.CanonicalizeLateChargeInclusionPayload(
		command.Number, command.ChargeID, command.SubsequentPeriod, command.IncludedAt))
}

// adjustmentInclusionDigest 与 inclusionDigest 分面：两种纳入指向的金额对象不同类
// （费用 / 调整），同一个纳入标识先纳费用再纳调整是冲突。
func adjustmentInclusionDigest(command IncludeAdjustmentCommand) string {
	return settlementCanonicalDigest(domain.CanonicalizeAdjustmentInclusionPayload(
		command.Number, command.AdjustmentID, command.SubsequentPeriod, command.IncludedAt))
}

func disputeDigest(command OpenDisputeCommand) string {
	return settlementCanonicalDigest(domain.CanonicalizeOpenDisputePayload(
		command.Number, command.ChargeID, command.DisputedMinor, command.Reason, command.OpenedAt))
}

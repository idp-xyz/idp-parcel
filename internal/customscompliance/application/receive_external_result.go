// Package application 编排 customs-compliance 的用例。监管语义判断在领域，这里只做
// 受理、幂等、归属、解释与提交的协调。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// ErrUnexpectedResultSave 说明结果库交回了封闭集合以外的写入结果。
var ErrUnexpectedResultSave = errors.New("customs compliance: unexpected external result save outcome")

// ExternalResultOutcome 是一次外部结果提交的应用处理结果。`未受理`与`未决`分格
// （ADR-0029）；`归属不上`与`同层冲突`都是保存性结果——留存不猜、留存双方。
type ExternalResultOutcome uint8

const (
	ExternalResultOutcomeInvalid ExternalResultOutcome = iota
	ResultRecorded
	ResultExistingResult
	ResultSourceConflict
	ResultUnattributable
	ResultLayerConflict
	ResultNotAccepted
	ResultUndecided
)

func (outcome ExternalResultOutcome) String() string {
	switch outcome {
	case ResultRecorded:
		return "RESULT_RECORDED"
	case ResultExistingResult:
		return "EXISTING_RESULT"
	case ResultSourceConflict:
		return "SOURCE_CONFLICT"
	case ResultUnattributable:
		return "UNATTRIBUTABLE"
	case ResultLayerConflict:
		return "LAYER_CONFLICT"
	case ResultNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case ResultUndecided:
		return "RESULT_UNDECIDED"
	default:
		return ""
	}
}

// ExternalResultUndecidedReason 指名提交停在哪一步等谁。解释规则未配置是实例半边的
// 一格——不用默认口径猜监管语义。评估时点不可信与辖区解析不出是规则选择侧的两格
// （ADR-0070）：选不出该用哪版规则时停在未决，绝不拿当前指针兜底。
type ExternalResultUndecidedReason uint8

const (
	ExternalResultUndecidedReasonNone ExternalResultUndecidedReason = iota
	ResultStoreUnavailable
	SubmissionIndexUnavailable
	InterpretationRuleUnconfigured
	LayerFactsUnavailable
	EvaluationInstantUntrusted
	CaseChainUnavailable
	JurisdictionUnresolved
)

func (reason ExternalResultUndecidedReason) String() string {
	switch reason {
	case ResultStoreUnavailable:
		return "RESULT_STORE_UNAVAILABLE"
	case SubmissionIndexUnavailable:
		return "SUBMISSION_INDEX_UNAVAILABLE"
	case InterpretationRuleUnconfigured:
		return "INTERPRETATION_RULE_UNCONFIGURED"
	case LayerFactsUnavailable:
		return "LAYER_FACTS_UNAVAILABLE"
	case EvaluationInstantUntrusted:
		return "EVALUATION_INSTANT_UNTRUSTED"
	case CaseChainUnavailable:
		return "CASE_CHAIN_UNAVAILABLE"
	case JurisdictionUnresolved:
		return "JURISDICTION_UNRESOLVED"
	default:
		return ""
	}
}

// ReceiveExternalResultCommand 携带一条外部监管响应的全部来源。
type ReceiveExternalResultCommand struct {
	TenantID       domain.TenantID
	SourceID       string
	Layer          domain.ResultLayer
	Role           string
	RawSemantics   string
	ClaimedVersion string
	Attempt        int
	Scope          string
	OccurredAt     time.Time
	ReceivedAt     time.Time
}

type ReceiveExternalResultResult struct {
	outcome      ExternalResultOutcome
	reason       ExternalResultUndecidedReason
	record       ports.ExternalResultRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result ReceiveExternalResultResult) Outcome() ExternalResultOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result ReceiveExternalResultResult) UndecidedReason() ExternalResultUndecidedReason {
	return result.reason
}

func (result ReceiveExternalResultResult) Record() (ports.ExternalResultRecord, bool) {
	return result.record, result.hasRecord
}

func (result ReceiveExternalResultResult) ContinuationReference() string {
	return result.continuation
}

// ResultHandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result ReceiveExternalResultResult) ResultHandoffReference() string {
	return result.handoff
}

// ReceiveExternalResultDeps 的 Units 与 Cases 只为辖区回指链服务（范围→单元→案件）：
// 规则选择要的适用辖区在案件本体上，不在外部结果这条链的任何自报字段里。
type ReceiveExternalResultDeps struct {
	Results     ports.ExternalResultStore
	Submissions ports.SubmissionIndex
	Rules       ports.InterpretationRuleView
	Units       ports.DeclarationUnitStore
	Cases       ports.CustomsCaseStore
	Downstream  ports.ExternalResultHandoff
	Clock       ports.Clock
}

type ReceiveExternalResultHandler struct {
	deps ReceiveExternalResultDeps
}

func NewReceiveExternalResultHandler(deps ReceiveExternalResultDeps) *ReceiveExternalResultHandler {
	return &ReceiveExternalResultHandler{deps: deps}
}

// Handle 把一条外部监管响应推进到分层事实：幂等/冲突按内容指纹分界 → 关联原提交
// （找不到→留存不猜）→ 评估时点与适用辖区（取不出→未决）→ 按时点与辖区解析解释
// 规则版本（未配置→未决）→ 领域解释与同层一致性（冲突留存双方）→ 原子提交 →
// 发布意图。意图投递失败不翻结果，重放重发同一份。
func (handler *ReceiveExternalResultHandler) Handle(
	ctx context.Context,
	command ReceiveExternalResultCommand,
) (ReceiveExternalResultResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" ||
		strings.TrimSpace(command.SourceID) == "" ||
		strings.TrimSpace(command.RawSemantics) == "" ||
		strings.TrimSpace(command.ClaimedVersion) == "" {
		return ReceiveExternalResultResult{outcome: ResultNotAccepted}, nil
	}

	key := ports.ExternalResultKey{TenantID: command.TenantID, SourceID: command.SourceID}
	digest := externalResultDigest(command)
	existing, found, err := handler.deps.Results.FindByKey(ctx, key)
	if err != nil {
		return resultStoreUndecided(command.SourceID), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一来源响应身份携带不同语义或范围：冲突保留原结果，不按最后到达覆盖。
			return ReceiveExternalResultResult{outcome: ResultSourceConflict}, nil
		}
		return handler.existingResult(ctx, existing), nil
	}

	version, err := domain.NewSubmissionVersionID(command.ClaimedVersion)
	if err != nil {
		return ReceiveExternalResultResult{outcome: ResultNotAccepted}, nil
	}
	attributed, err := handler.deps.Submissions.FindSubmission(ctx, command.TenantID, version)
	if err != nil {
		return ReceiveExternalResultResult{outcome: ResultUndecided, reason: SubmissionIndexUnavailable,
			continuation: resultContinuation("SUBMISSION_INDEX_UNAVAILABLE", command.SourceID)}, nil
	}
	if !attributed {
		// 归属不上原提交：留存原始响应与其声称的版本，不猜测提交、不补造层次
		// （CONTEXT 硬句 187）。留存的不是监管事实，不交意图。
		record := ports.ExternalResultRecord{
			Key:            key,
			ContentDigest:  digest,
			Unattributable: true,
			RawSemantics:   command.RawSemantics,
			ClaimedVersion: command.ClaimedVersion,
			RecordedAt:     handler.deps.Clock.Now(),
		}
		return handler.commit(ctx, record, ResultUnattributable)
	}

	// 评估时点 = 业务发生或适用时间（ADR-0070 问二甲）。来源未给出（零值）或给出因果
	// 上立不住的值（业务发生晚于接收）即显式未决——绝不改拿消息到达或系统当前时间
	// 顶替，那正是硬句 191 点名禁止的替代，当前指针册子的缺陷不能原样藏进版本化册子。
	if command.OccurredAt.IsZero() ||
		(!command.ReceivedAt.IsZero() && command.OccurredAt.After(command.ReceivedAt)) {
		return ReceiveExternalResultResult{outcome: ResultUndecided, reason: EvaluationInstantUntrusted,
			continuation: resultContinuation("EVALUATION_INSTANT_UNTRUSTED", command.SourceID)}, nil
	}

	// 适用辖区从外部结果回指案件取（ADR-0070 问三甲）：结果范围指名申报单元，单元持有
	// 其案件（ADR-0073），辖区在案件本体上。链上读不回是依赖故障，走不通（范围不指名
	// 在册单元、单元的案件不在册）是解析不出——两格续办动作不同，分开。单辖区租户下
	// 辖区也必须来自这条链，不留「当前唯一辖区」的兜底缝（问三丙的错法）。
	unitID, err := domain.NewDeclarationUnitID(command.Scope)
	if err != nil {
		return ReceiveExternalResultResult{outcome: ResultNotAccepted}, nil
	}
	unit, unitFound, err := handler.deps.Units.FindByID(ctx, command.TenantID, unitID)
	if err != nil {
		return ReceiveExternalResultResult{outcome: ResultUndecided, reason: CaseChainUnavailable,
			continuation: resultContinuation("CASE_CHAIN_UNAVAILABLE", command.SourceID)}, nil
	}
	if !unitFound {
		return ReceiveExternalResultResult{outcome: ResultUndecided, reason: JurisdictionUnresolved,
			continuation: resultContinuation("JURISDICTION_UNRESOLVED", command.SourceID)}, nil
	}
	customsCase, caseFound, err := handler.deps.Cases.FindByID(ctx, command.TenantID, unit.Case())
	if err != nil {
		return ReceiveExternalResultResult{outcome: ResultUndecided, reason: CaseChainUnavailable,
			continuation: resultContinuation("CASE_CHAIN_UNAVAILABLE", command.SourceID)}, nil
	}
	if !caseFound {
		return ReceiveExternalResultResult{outcome: ResultUndecided, reason: JurisdictionUnresolved,
			continuation: resultContinuation("JURISDICTION_UNRESOLVED", command.SourceID)}, nil
	}

	rule, configured, err := handler.deps.Rules.LoadInterpretationRule(
		ctx, command.TenantID, command.Layer, customsCase.Jurisdiction(), command.OccurredAt)
	if err != nil {
		return resultStoreUndecided(command.SourceID), nil
	}
	if !configured {
		// 解释规则是实例半边：该辖区该层在该评估时点无已登记版本即停在未决，不用默认
		// 口径猜监管语义。
		return ReceiveExternalResultResult{outcome: ResultUndecided, reason: InterpretationRuleUnconfigured,
			continuation: resultContinuation("INTERPRETATION_RULE_UNCONFIGURED", command.SourceID)}, nil
	}

	role, err := domain.NewSourceAuthorityRole(command.Role)
	if err != nil {
		return ReceiveExternalResultResult{outcome: ResultNotAccepted}, nil
	}
	scope, err := domain.NewDecisionScopeReference(command.Scope)
	if err != nil {
		return ReceiveExternalResultResult{outcome: ResultNotAccepted}, nil
	}
	interpreted, err := domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer:        command.Layer,
		SourceID:     command.SourceID,
		Role:         role,
		RawSemantics: command.RawSemantics,
		Rule:         rule,
		Version:      version,
		Attempt:      command.Attempt,
		Scope:        scope,
		OccurredAt:   command.OccurredAt,
		ReceivedAt:   command.ReceivedAt,
	})
	if err != nil {
		return ReceiveExternalResultResult{outcome: ResultNotAccepted}, nil
	}

	layerFacts, err := handler.deps.Results.LoadForSubmission(ctx, command.TenantID, version)
	if err != nil {
		return ReceiveExternalResultResult{outcome: ResultUndecided, reason: LayerFactsUnavailable,
			continuation: resultContinuation("LAYER_FACTS_UNAVAILABLE", command.SourceID)}, nil
	}
	record := ports.ExternalResultRecord{
		Key:           key,
		ContentDigest: digest,
		Result:        interpreted,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	outcome := ResultRecorded
	if err := domain.CheckLayerConsistency(layerFacts, interpreted); errors.Is(err, domain.ErrLayerConflict) {
		// 同层冲突：双方事实都留存，不选边、不改当前判断——冲突以标记与结果格显式可见。
		record.LayerConflict = true
		outcome = ResultLayerConflict
	} else if err != nil {
		return ReceiveExternalResultResult{outcome: ResultNotAccepted}, nil
	}

	return handler.commit(ctx, record, outcome)
}

func resultStoreUndecided(sourceID string) ReceiveExternalResultResult {
	return ReceiveExternalResultResult{
		outcome:      ResultUndecided,
		reason:       ResultStoreUnavailable,
		continuation: resultContinuation("RESULT_STORE_UNAVAILABLE", sourceID),
	}
}

// commit 提交记录并交发布意图；并发下另一方先提交时读回赢家。
func (handler *ReceiveExternalResultHandler) commit(
	ctx context.Context,
	record ports.ExternalResultRecord,
	outcome ExternalResultOutcome,
) (ReceiveExternalResultResult, error) {
	saved, err := handler.deps.Results.Save(ctx, record)
	if err != nil {
		return resultStoreUndecided(record.Key.SourceID), nil
	}
	switch saved {
	case ports.ExternalResultSaved:
		result := ReceiveExternalResultResult{outcome: outcome, record: record, hasRecord: true}
		result.handoff = handler.handOff(ctx, record)
		return result, nil
	case ports.ExternalResultAlreadyRecorded:
		winner, found, err := handler.deps.Results.FindByKey(ctx, record.Key)
		if err != nil || !found {
			return resultStoreUndecided(record.Key.SourceID), nil
		}
		return handler.existingResult(ctx, winner), nil
	default:
		return ReceiveExternalResultResult{}, fmt.Errorf("%w: %d", ErrUnexpectedResultSave, saved)
	}
}

// existingResult 按已有记录作答并重发同一份意图。
func (handler *ReceiveExternalResultHandler) existingResult(
	ctx context.Context,
	record ports.ExternalResultRecord,
) ReceiveExternalResultResult {
	return ReceiveExternalResultResult{
		outcome:   ResultExistingResult,
		record:    record,
		hasRecord: true,
		handoff:   handler.handOff(ctx, record),
	}
}

// handOff 交发布意图。归属不上的留存记录没有可供判断消费的监管事实，不交；投递失败
// 不翻结果，留续办引用重放时重发同一份。
func (handler *ReceiveExternalResultHandler) handOff(
	ctx context.Context,
	record ports.ExternalResultRecord,
) string {
	if record.Unattributable {
		return ""
	}
	if err := handler.deps.Downstream.HandOffExternalResult(ctx, ports.ExternalResultHandoffIntent{Record: record}); err == nil {
		return ""
	}
	return resultContinuation("EXTERNAL_RESULT_HANDOFF", record.Key.TenantID.String(), record.Key.SourceID)
}

func resultContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// externalResultDigest 是同一来源响应身份的内容比对锚：层、原始语义、声称版本、尝试
// 序号、范围与业务时间任一不同即是另一份内容。
func externalResultDigest(command ReceiveExternalResultCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		fmt.Sprintf("%d", command.Layer),
		command.RawSemantics,
		command.ClaimedVersion,
		fmt.Sprintf("%d", command.Attempt),
		command.Scope,
		command.OccurredAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

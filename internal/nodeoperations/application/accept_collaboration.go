// accept_collaboration.go 编排 UC-NO-001 的两个入口：承接决定与执行事实登记。判断
// 形状都在领域（决定三格、越权拒、不冒充查验结论），这里只做受理、幂等与发布意图。
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

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

var (
	// ErrUnexpectedAcceptanceSave 说明承接库交回了封闭集合以外的写入结果。
	ErrUnexpectedAcceptanceSave = errors.New("node operations: unexpected acceptance save outcome")
	// ErrUnexpectedFactSave 说明执行事实库交回了封闭集合以外的写入结果。
	ErrUnexpectedFactSave = errors.New("node operations: unexpected execution fact save outcome")
)

// CollaborationOutcome 是承接/执行提交的应用处理结果。`拒接事项无可执行`与`越权`是
// 业务负向格——恢复动作分别是重新承接与扩承接范围，不是改单也不是重试（ADR-0029）。
type CollaborationOutcome uint8

const (
	CollaborationOutcomeInvalid CollaborationOutcome = iota
	CollaborationDecided
	CollaborationExistingDecision
	CollaborationDecisionConflict
	ExecutionRecorded
	ExecutionExistingFact
	ExecutionFactConflict
	ExecutionOnDeclinedItem
	ExecutionOutsideScope
	CollaborationNotAccepted
	CollaborationUndecided
)

func (outcome CollaborationOutcome) String() string {
	switch outcome {
	case CollaborationDecided:
		return "COLLABORATION_DECIDED"
	case CollaborationExistingDecision:
		return "EXISTING_DECISION"
	case CollaborationDecisionConflict:
		return "DECISION_CONFLICT"
	case ExecutionRecorded:
		return "EXECUTION_RECORDED"
	case ExecutionExistingFact:
		return "EXISTING_FACT"
	case ExecutionFactConflict:
		return "FACT_CONFLICT"
	case ExecutionOnDeclinedItem:
		return "ITEM_DECLINED"
	case ExecutionOutsideScope:
		return "OUTSIDE_ACCEPTED_SCOPE"
	case CollaborationNotAccepted:
		return "SOURCE_NOT_ACCEPTED"
	case CollaborationUndecided:
		return "COLLABORATION_UNDECIDED"
	default:
		return ""
	}
}

// CollaborationUndecidedReason 指名提交停在哪一步等谁。
type CollaborationUndecidedReason uint8

const (
	CollaborationUndecidedReasonNone CollaborationUndecidedReason = iota
	AcceptanceStoreUnavailable
	FactStoreUnavailable
)

func (reason CollaborationUndecidedReason) String() string {
	switch reason {
	case AcceptanceStoreUnavailable:
		return "ACCEPTANCE_STORE_UNAVAILABLE"
	case FactStoreUnavailable:
		return "FACT_STORE_UNAVAILABLE"
	default:
		return ""
	}
}

// AcceptCollaborationCommand 携带一次承接决定的全部输入。
type AcceptCollaborationCommand struct {
	TenantID        domain.TenantID
	Node            string
	Item            string
	Decision        domain.AcceptanceDecisionKind
	AcceptedUnits   []string
	AcceptedActions []domain.CollaborationActionKind
	Authority       string
	Basis           string
	DecidedAt       time.Time
}

// RecordExecutionCommand 携带一次执行事实登记的全部输入。
type RecordExecutionCommand struct {
	TenantID    domain.TenantID
	Item        string
	Unit        string
	Action      domain.CollaborationActionKind
	Evidence    string
	PerformedAt time.Time
}

type CollaborationResult struct {
	outcome      CollaborationOutcome
	reason       CollaborationUndecidedReason
	acceptance   ports.CollaborationAcceptanceRecord
	fact         ports.ExecutionFactRecord
	hasRecord    bool
	continuation string
	handoff      string
}

func (result CollaborationResult) Outcome() CollaborationOutcome {
	return result.outcome
}

// UndecidedReason 只在`未决`时非零。
func (result CollaborationResult) UndecidedReason() CollaborationUndecidedReason {
	return result.reason
}

func (result CollaborationResult) Acceptance() (ports.CollaborationAcceptanceRecord, bool) {
	return result.acceptance, result.hasRecord && result.acceptance.Key.Item.String() != ""
}

func (result CollaborationResult) Fact() (ports.ExecutionFactRecord, bool) {
	return result.fact, result.hasRecord && result.fact.Key.Item.String() != ""
}

func (result CollaborationResult) ContinuationReference() string {
	return result.continuation
}

// HandoffReference 非空说明记录已提交但意图还没交出去，重放会重发同一份。
func (result CollaborationResult) HandoffReference() string {
	return result.handoff
}

type AcceptCollaborationDeps struct {
	Acceptances ports.CollaborationAcceptanceStore
	Facts       ports.ExecutionFactStore
	Acknowledge ports.CollaborationAcceptanceHandoff
	Executions  ports.ExecutionFactHandoff
	Clock       ports.Clock
}

type AcceptCollaborationHandler struct {
	deps AcceptCollaborationDeps
}

func NewAcceptCollaborationHandler(deps AcceptCollaborationDeps) *AcceptCollaborationHandler {
	return &AcceptCollaborationHandler{deps: deps}
}

// Accept 把一次承接决定推进到提交：受理（决定三格由领域把门）→ 幂等按（租户+事项）
// 分重放/冲突（同一事项只决定一次）→ 原子提交 → 意图交回 CC。
func (handler *AcceptCollaborationHandler) Accept(
	ctx context.Context,
	command AcceptCollaborationCommand,
) (CollaborationResult, error) {
	acceptance, err := decideFrom(command)
	if err != nil {
		return CollaborationResult{outcome: CollaborationNotAccepted}, nil
	}

	key := ports.CollaborationAcceptanceKey{TenantID: command.TenantID, Item: acceptance.Item()}
	digest := acceptanceDigest(command)
	existing, found, err := handler.deps.Acceptances.FindByKey(ctx, key)
	if err != nil {
		return acceptanceStoreUndecided(command.Item), nil
	}
	if found {
		if existing.ContentDigest != digest {
			// 同一事项携带不同决定或范围：冲突保留原决定——改主意走事项方的重派，
			// 不在这里顶替。
			return CollaborationResult{outcome: CollaborationDecisionConflict}, nil
		}
		return handler.existingAcceptance(ctx, existing), nil
	}

	record := ports.CollaborationAcceptanceRecord{
		Key:           key,
		ContentDigest: digest,
		Acceptance:    acceptance,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Acceptances.Save(ctx, record)
	if err != nil {
		return acceptanceStoreUndecided(command.Item), nil
	}
	switch saved {
	case ports.AcceptanceSaved:
		result := CollaborationResult{outcome: CollaborationDecided, acceptance: record, hasRecord: true}
		result.handoff = handler.handOffAcceptance(ctx, record)
		return result, nil
	case ports.AcceptanceAlreadyDecided:
		winner, found, err := handler.deps.Acceptances.FindByKey(ctx, key)
		if err != nil || !found {
			return acceptanceStoreUndecided(command.Item), nil
		}
		return handler.existingAcceptance(ctx, winner), nil
	default:
		return CollaborationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedAcceptanceSave, saved)
	}
}

// RecordExecution 把一次执行事实推进到登记：读回承接决定（守界依据）→ 领域记录
// （拒接/越权各归业务负向格）→ 幂等按（租户+事项+实物+动作）→ 意图交 CC 处置执行
// 核对。
func (handler *AcceptCollaborationHandler) RecordExecution(
	ctx context.Context,
	command RecordExecutionCommand,
) (CollaborationResult, error) {
	item, err := domain.NewCollaborationItemReference(command.Item)
	if err != nil {
		return CollaborationResult{outcome: CollaborationNotAccepted}, nil
	}
	unit, err := domain.NewHandlingUnitID(command.Unit)
	if err != nil {
		return CollaborationResult{outcome: CollaborationNotAccepted}, nil
	}
	evidence, err := domain.NewExecutionEvidenceReference(command.Evidence)
	if err != nil {
		return CollaborationResult{outcome: CollaborationNotAccepted}, nil
	}

	acceptanceRecord, found, err := handler.deps.Acceptances.FindByKey(ctx,
		ports.CollaborationAcceptanceKey{TenantID: command.TenantID, Item: item})
	if err != nil {
		return acceptanceStoreUndecided(command.Item), nil
	}
	if !found {
		// 没有承接决定就没有可执行的范围——先承接再执行。
		return CollaborationResult{outcome: CollaborationNotAccepted}, nil
	}

	fact, err := domain.RecordExecutionFact(acceptanceRecord.Acceptance, unit, command.Action, evidence, command.PerformedAt)
	if errors.Is(err, domain.ErrCollaborationRefused) {
		return CollaborationResult{outcome: ExecutionOnDeclinedItem}, nil
	}
	if errors.Is(err, domain.ErrOutsideAcceptedScope) {
		return CollaborationResult{outcome: ExecutionOutsideScope}, nil
	}
	if err != nil {
		return CollaborationResult{outcome: CollaborationNotAccepted}, nil
	}

	key := ports.ExecutionFactKey{TenantID: command.TenantID, Item: item, Unit: unit, Action: command.Action}
	digest := executionDigest(command)
	existing, factFound, err := handler.deps.Facts.FindByKey(ctx, key)
	if err != nil {
		return factStoreUndecided(command.Item), nil
	}
	if factFound {
		if existing.ContentDigest != digest {
			return CollaborationResult{outcome: ExecutionFactConflict}, nil
		}
		return handler.existingFact(ctx, existing), nil
	}

	record := ports.ExecutionFactRecord{
		Key:           key,
		ContentDigest: digest,
		Fact:          fact,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return factStoreUndecided(command.Item), nil
	}
	switch saved {
	case ports.ExecutionFactSaved:
		result := CollaborationResult{outcome: ExecutionRecorded, fact: record, hasRecord: true}
		result.handoff = handler.handOffFact(ctx, record)
		return result, nil
	case ports.ExecutionFactAlreadyRecorded:
		winner, found, err := handler.deps.Facts.FindByKey(ctx, key)
		if err != nil || !found {
			return factStoreUndecided(command.Item), nil
		}
		return handler.existingFact(ctx, winner), nil
	default:
		return CollaborationResult{}, fmt.Errorf("%w: %d", ErrUnexpectedFactSave, saved)
	}
}

func decideFrom(command AcceptCollaborationCommand) (domain.CollaborationAcceptance, error) {
	spec := domain.CollaborationAcceptanceSpec{
		TenantID:        command.TenantID,
		Decision:        command.Decision,
		AcceptedActions: command.AcceptedActions,
		DecidedAt:       command.DecidedAt,
	}
	var err error
	if spec.Node, err = domain.NewNodeReference(command.Node); err != nil {
		return domain.CollaborationAcceptance{}, err
	}
	if spec.Item, err = domain.NewCollaborationItemReference(command.Item); err != nil {
		return domain.CollaborationAcceptance{}, err
	}
	if spec.Authority, err = domain.NewAcceptanceAuthorityReference(command.Authority); err != nil {
		return domain.CollaborationAcceptance{}, err
	}
	if strings.TrimSpace(command.Basis) != "" {
		if spec.Basis, err = domain.NewAcceptanceBasisReference(command.Basis); err != nil {
			return domain.CollaborationAcceptance{}, err
		}
	}
	for _, raw := range command.AcceptedUnits {
		unit, err := domain.NewHandlingUnitID(raw)
		if err != nil {
			return domain.CollaborationAcceptance{}, err
		}
		spec.AcceptedUnits = append(spec.AcceptedUnits, unit)
	}
	return domain.DecideCollaborationAcceptance(spec)
}

func acceptanceStoreUndecided(item string) CollaborationResult {
	return CollaborationResult{
		outcome:      CollaborationUndecided,
		reason:       AcceptanceStoreUnavailable,
		continuation: collaborationContinuation("ACCEPTANCE_STORE_UNAVAILABLE", item),
	}
}

func factStoreUndecided(item string) CollaborationResult {
	return CollaborationResult{
		outcome:      CollaborationUndecided,
		reason:       FactStoreUnavailable,
		continuation: collaborationContinuation("FACT_STORE_UNAVAILABLE", item),
	}
}

// existingAcceptance 按已有决定作答并重发同一份意图。
func (handler *AcceptCollaborationHandler) existingAcceptance(
	ctx context.Context,
	record ports.CollaborationAcceptanceRecord,
) CollaborationResult {
	return CollaborationResult{
		outcome:    CollaborationExistingDecision,
		acceptance: record,
		hasRecord:  true,
		handoff:    handler.handOffAcceptance(ctx, record),
	}
}

// existingFact 按已有事实作答并重发同一份意图。
func (handler *AcceptCollaborationHandler) existingFact(
	ctx context.Context,
	record ports.ExecutionFactRecord,
) CollaborationResult {
	return CollaborationResult{
		outcome:   ExecutionExistingFact,
		fact:      record,
		hasRecord: true,
		handoff:   handler.handOffFact(ctx, record),
	}
}

func (handler *AcceptCollaborationHandler) handOffAcceptance(
	ctx context.Context,
	record ports.CollaborationAcceptanceRecord,
) string {
	if err := handler.deps.Acknowledge.HandOffCollaborationAcceptance(ctx, ports.CollaborationAcceptanceHandoffIntent{Record: record}); err == nil {
		return ""
	}
	return collaborationContinuation("COLLABORATION_ACCEPTANCE_HANDOFF", record.Key.TenantID.String(), record.Key.Item.String())
}

func (handler *AcceptCollaborationHandler) handOffFact(
	ctx context.Context,
	record ports.ExecutionFactRecord,
) string {
	if err := handler.deps.Executions.HandOffExecutionFact(ctx, ports.ExecutionFactHandoffIntent{Record: record}); err == nil {
		return ""
	}
	return collaborationContinuation("EXECUTION_FACT_HANDOFF", record.Key.TenantID.String(), record.Key.Item.String())
}

func collaborationContinuation(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "CONT-" + hex.EncodeToString(digest[:8])
}

// acceptanceDigest 是同一事项决定的内容比对锚：决定、范围、授权与依据任一不同即是
// 另一个决定。范围先排序——提交顺序不构成不同的决定。
func acceptanceDigest(command AcceptCollaborationCommand) string {
	units := append([]string(nil), command.AcceptedUnits...)
	sort.Strings(units)
	actions := make([]string, 0, len(command.AcceptedActions))
	for _, action := range command.AcceptedActions {
		actions = append(actions, fmt.Sprintf("%d", action))
	}
	sort.Strings(actions)
	digest := sha256.Sum256([]byte(strings.Join(append(append([]string{
		fmt.Sprintf("%d", command.Decision),
		command.Node,
		command.Authority,
		command.Basis,
	}, units...), actions...), "\x00")))
	return hex.EncodeToString(digest[:])
}

// executionDigest 是同一（事项+实物+动作）登记的内容比对锚：证据与业务时间任一不同
// 即是另一份内容。
func executionDigest(command RecordExecutionCommand) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		command.Evidence,
		command.PerformedAt.UTC().Format(time.RFC3339Nano),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}

// accept_collaboration.go 编排 UC-NO-001 的两个入口：承接决定与执行事实登记。判断
// 形状都在领域（决定三格、越权拒、不冒充查验结论），这里只做受理、幂等与发布意图。
package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
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
	outcome    CollaborationOutcome
	acceptance ports.CollaborationAcceptanceRecord
	fact       ports.ExecutionFactRecord
	hasRecord  bool
	handoff    string
}

func (result CollaborationResult) Outcome() CollaborationOutcome {
	return result.outcome
}

func (result CollaborationResult) Acceptance() (ports.CollaborationAcceptanceRecord, bool) {
	return result.acceptance, result.hasRecord && result.acceptance.Key.Item.String() != ""
}

func (result CollaborationResult) Fact() (ports.ExecutionFactRecord, bool) {
	return result.fact, result.hasRecord && result.fact.Key.Item.String() != ""
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
//
// 本方册读写失败、已存摘要形状认不出、并发读回失败，一律交回 error 而不是答一格`未决`：UC-NO-001 把它们归
// 「节点协作结果未形成」，ADR-0022 让「没形成」只有返回 error 一种说法——答 2xx 的话设备出队，这一次作业在服务端
// 就没有记录了（ADR-0186 决定三的同形）。执行事实那条路同此。
func (handler *AcceptCollaborationHandler) Accept(
	ctx context.Context,
	command AcceptCollaborationCommand,
) (CollaborationResult, error) {
	acceptance, err := decideFrom(command)
	if err != nil {
		return CollaborationResult{outcome: CollaborationNotAccepted}, nil
	}

	key := ports.CollaborationAcceptanceKey{TenantID: command.TenantID, Item: acceptance.Item()}
	_, digest, err := domain.CanonicalizeAcceptancePayload(
		command.Decision, command.Node, command.Authority, command.Basis, command.AcceptedUnits, command.AcceptedActions,
	)
	if err != nil {
		return CollaborationResult{}, err
	}
	existing, found, err := handler.deps.Acceptances.FindByKey(ctx, key)
	if err != nil {
		return CollaborationResult{}, fmt.Errorf("look up collaboration acceptance: %w", err)
	}
	if found {
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversionedAcceptanceDigest(command)) {
		case domain.SamePayload:
			return handler.existingAcceptance(ctx, existing), nil
		case domain.DifferentPayload:
			// 同一事项携带不同决定或范围：冲突保留原决定——改主意走事项方的重派，
			// 不在这里顶替。
			return CollaborationResult{outcome: CollaborationDecisionConflict}, nil
		default:
			return CollaborationResult{}, errors.New("node operations: stored acceptance digest is not comparable")
		}
	}

	record := ports.CollaborationAcceptanceRecord{
		Key:           key,
		ContentDigest: digest,
		Acceptance:    acceptance,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Acceptances.Save(ctx, record)
	if err != nil {
		return CollaborationResult{}, fmt.Errorf("record collaboration acceptance: %w", err)
	}
	switch saved {
	case ports.AcceptanceSaved:
		result := CollaborationResult{outcome: CollaborationDecided, acceptance: record, hasRecord: true}
		result.handoff = handler.handOffAcceptance(ctx, record)
		return result, nil
	case ports.AcceptanceAlreadyDecided:
		winner, found, err := handler.deps.Acceptances.FindByKey(ctx, key)
		if err != nil {
			return CollaborationResult{}, fmt.Errorf("read back collaboration acceptance winner: %w", err)
		}
		if !found {
			return CollaborationResult{}, errors.New("node operations: acceptance already decided but not readable")
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
		return CollaborationResult{}, fmt.Errorf("look up collaboration acceptance: %w", err)
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
	_, digest, err := domain.CanonicalizeExecutionPayload(command.Evidence, command.PerformedAt)
	if err != nil {
		return CollaborationResult{}, err
	}
	existing, factFound, err := handler.deps.Facts.FindByKey(ctx, key)
	if err != nil {
		return CollaborationResult{}, fmt.Errorf("look up execution fact: %w", err)
	}
	if factFound {
		switch domain.CompareStoredDigest(existing.ContentDigest, digest, unversionedExecutionDigest(command)) {
		case domain.SamePayload:
			return handler.existingFact(ctx, existing), nil
		case domain.DifferentPayload:
			return CollaborationResult{outcome: ExecutionFactConflict}, nil
		default:
			return CollaborationResult{}, errors.New("node operations: stored execution fact digest is not comparable")
		}
	}

	record := ports.ExecutionFactRecord{
		Key:           key,
		ContentDigest: digest,
		Fact:          fact,
		RecordedAt:    handler.deps.Clock.Now(),
	}
	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return CollaborationResult{}, fmt.Errorf("record execution fact: %w", err)
	}
	switch saved {
	case ports.ExecutionFactSaved:
		result := CollaborationResult{outcome: ExecutionRecorded, fact: record, hasRecord: true}
		result.handoff = handler.handOffFact(ctx, record)
		return result, nil
	case ports.ExecutionFactAlreadyRecorded:
		winner, found, err := handler.deps.Facts.FindByKey(ctx, key)
		if err != nil {
			return CollaborationResult{}, fmt.Errorf("read back execution fact winner: %w", err)
		}
		if !found {
			return CollaborationResult{}, errors.New("node operations: execution fact already recorded but not readable")
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

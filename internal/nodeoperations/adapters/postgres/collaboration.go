package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// CollaborationAcceptances 实现 ports.CollaborationAcceptanceStore（写入代数同 ADR-0031）。
type CollaborationAcceptances struct {
	db *bentopg.DB
}

func NewCollaborationAcceptances(db *bentopg.DB) (*CollaborationAcceptances, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &CollaborationAcceptances{db: db}, nil
}

// FindByKey 按（租户+协作事项）取回已保存的承接决定。否定结果只回 false，不区分
// 「不存在」与「属于另一个租户」。读回经 DecideCollaborationAcceptance 重建。
func (repository *CollaborationAcceptances) FindByKey(
	ctx context.Context,
	key ports.CollaborationAcceptanceKey,
) (ports.CollaborationAcceptanceRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.CollaborationAcceptanceRecord{}, false, fmt.Errorf("find collaboration acceptance: %w", err)
	}

	var (
		nodeRef, decisionName, authority, digest string
		basis                                    *string
		unitsJSON, actionsJSON, gapsJSON         []byte
		round                                    int
		decidedAt, recordedAt                    time.Time
	)
	// 取当前轮次：待补充之后的新决定是更高一轮，旧轮原样留在表里。
	err = querier.QueryRow(ctx,
		`SELECT node_ref, decision, authority, basis, accepted_units, accepted_actions, gaps,
		        round, decided_at, content_digest, recorded_at
		   FROM node_operations.collaboration_acceptance
		  WHERE tenant_id = $1
		    AND item_ref = $2
		  ORDER BY round DESC
		  LIMIT 1`,
		key.TenantID.String(),
		key.Item.String(),
	).Scan(&nodeRef, &decisionName, &authority, &basis, &unitsJSON, &actionsJSON, &gapsJSON,
		&round, &decidedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.CollaborationAcceptanceRecord{}, false, nil
	}
	if err != nil {
		return ports.CollaborationAcceptanceRecord{}, false, fmt.Errorf("find collaboration acceptance: %w", err)
	}

	acceptance, err := rebuildAcceptance(key.TenantID, key.Item, nodeRef, decisionName, authority, basis, unitsJSON, actionsJSON, gapsJSON, decidedAt)
	if err != nil {
		return ports.CollaborationAcceptanceRecord{}, false, fmt.Errorf("find collaboration acceptance: %w", err)
	}
	return ports.CollaborationAcceptanceRecord{
		Key:           key,
		Round:         round,
		ContentDigest: digest,
		Acceptance:    acceptance,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次承接决定。同（租户+事项+轮次）已有记录时答`已有决定`——业务答案不是错误
// （ADR-0031）；用 ON CONFLICT DO NOTHING 而不是捕 23505：撞键的 INSERT 会把整个事务
// 打进中止态，而编排拿到`已有决定`还要在同一个事务里读回原决定作答。
func (repository *CollaborationAcceptances) Save(
	ctx context.Context,
	record ports.CollaborationAcceptanceRecord,
) (ports.AcceptanceSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.AcceptanceSaveOutcomeInvalid, fmt.Errorf("save collaboration acceptance: %w", err)
	}

	units := make([]string, 0, len(record.Acceptance.AcceptedUnits()))
	for _, unit := range record.Acceptance.AcceptedUnits() {
		units = append(units, unit.String())
	}
	unitsJSON, err := json.Marshal(units)
	if err != nil {
		return ports.AcceptanceSaveOutcomeInvalid, fmt.Errorf("save collaboration acceptance: %w", err)
	}
	actions := make([]string, 0, len(record.Acceptance.AcceptedActions()))
	for _, action := range record.Acceptance.AcceptedActions() {
		actions = append(actions, action.String())
	}
	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		return ports.AcceptanceSaveOutcomeInvalid, fmt.Errorf("save collaboration acceptance: %w", err)
	}

	if record.Round < 1 {
		return ports.AcceptanceSaveOutcomeInvalid, fmt.Errorf("save collaboration acceptance: round starts at 1")
	}
	gaps := make([]string, 0, len(record.Acceptance.Gaps()))
	for _, gap := range record.Acceptance.Gaps() {
		gaps = append(gaps, gap.String())
	}
	gapsJSON, err := json.Marshal(gaps)
	if err != nil {
		return ports.AcceptanceSaveOutcomeInvalid, fmt.Errorf("save collaboration acceptance: %w", err)
	}

	var basis *string
	if value, present := record.Acceptance.Basis(); present {
		raw := value.String()
		basis = &raw
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.collaboration_acceptance
			(tenant_id, item_ref, round, node_ref, decision, authority, basis,
			 accepted_units, accepted_actions, gaps, decided_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Item.String(),
		record.Round,
		record.Acceptance.Node().String(),
		record.Acceptance.Decision().String(),
		record.Acceptance.Authority().String(),
		basis,
		unitsJSON,
		actionsJSON,
		gapsJSON,
		record.Acceptance.DecidedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.AcceptanceSaveOutcomeInvalid, fmt.Errorf("save collaboration acceptance: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.AcceptanceAlreadyDecided, nil
	}
	return ports.AcceptanceSaved, nil
}

func rebuildAcceptance(
	tenant domain.TenantID,
	item domain.CollaborationItemReference,
	nodeRef, decisionName, authority string,
	basis *string,
	unitsJSON, actionsJSON, gapsJSON []byte,
	decidedAt time.Time,
) (domain.CollaborationAcceptance, error) {
	node, err := domain.NewNodeReference(nodeRef)
	if err != nil {
		return domain.CollaborationAcceptance{}, err
	}
	decision, err := acceptanceDecisionFrom(decisionName)
	if err != nil {
		return domain.CollaborationAcceptance{}, err
	}
	authorityRef, err := domain.NewAcceptanceAuthorityReference(authority)
	if err != nil {
		return domain.CollaborationAcceptance{}, err
	}

	var unitIDs []string
	if err := json.Unmarshal(unitsJSON, &unitIDs); err != nil {
		return domain.CollaborationAcceptance{}, err
	}
	units := make([]domain.HandlingUnitID, 0, len(unitIDs))
	for _, raw := range unitIDs {
		unit, err := domain.NewHandlingUnitID(raw)
		if err != nil {
			return domain.CollaborationAcceptance{}, err
		}
		units = append(units, unit)
	}

	var actionNames []string
	if err := json.Unmarshal(actionsJSON, &actionNames); err != nil {
		return domain.CollaborationAcceptance{}, err
	}
	actions := make([]domain.CollaborationActionKind, 0, len(actionNames))
	for _, raw := range actionNames {
		action, err := collaborationActionFrom(raw)
		if err != nil {
			return domain.CollaborationAcceptance{}, err
		}
		actions = append(actions, action)
	}

	var gapNames []string
	if err := json.Unmarshal(gapsJSON, &gapNames); err != nil {
		return domain.CollaborationAcceptance{}, err
	}
	gaps := make([]domain.SupplementGapKind, 0, len(gapNames))
	for _, raw := range gapNames {
		gap, err := supplementGapFrom(raw)
		if err != nil {
			return domain.CollaborationAcceptance{}, err
		}
		gaps = append(gaps, gap)
	}

	spec := domain.CollaborationAcceptanceSpec{
		TenantID:        tenant,
		Node:            node,
		Item:            item,
		Decision:        decision,
		AcceptedUnits:   units,
		AcceptedActions: actions,
		Authority:       authorityRef,
		Gaps:            gaps,
		DecidedAt:       decidedAt,
	}
	if basis != nil {
		basisRef, err := domain.NewAcceptanceBasisReference(*basis)
		if err != nil {
			return domain.CollaborationAcceptance{}, err
		}
		spec.Basis = basisRef
	}
	return domain.DecideCollaborationAcceptance(spec)
}

func acceptanceDecisionFrom(raw string) (domain.AcceptanceDecisionKind, error) {
	switch raw {
	case domain.CollaborationAccepted.String():
		return domain.CollaborationAccepted, nil
	case domain.CollaborationDeclined.String():
		return domain.CollaborationDeclined, nil
	case domain.CollaborationPartiallyAccepted.String():
		return domain.CollaborationPartiallyAccepted, nil
	case domain.CollaborationPendingSupplement.String():
		return domain.CollaborationPendingSupplement, nil
	default:
		return 0, fmt.Errorf("unknown acceptance decision %q", raw)
	}
}

func supplementGapFrom(raw string) (domain.SupplementGapKind, error) {
	for _, gap := range []domain.SupplementGapKind{
		domain.MissingObjectGap, domain.MissingActionGap, domain.MissingNodeGap,
		domain.MissingAuthorityGap, domain.MissingWorkConditionGap,
	} {
		if raw == gap.String() {
			return gap, nil
		}
	}
	return 0, fmt.Errorf("unknown supplement gap %q", raw)
}

func collaborationActionFrom(raw string) (domain.CollaborationActionKind, error) {
	switch raw {
	case domain.UnsealAction.String():
		return domain.UnsealAction, nil
	case domain.IsolateAction.String():
		return domain.IsolateAction, nil
	case domain.PresentAction.String():
		return domain.PresentAction, nil
	case domain.TallyAction.String():
		return domain.TallyAction, nil
	case domain.ObserveAction.String():
		return domain.ObserveAction, nil
	default:
		return 0, fmt.Errorf("unknown collaboration action %q", raw)
	}
}

// ExecutionFacts 实现 ports.ExecutionFactStore（写入代数同 ADR-0031）。
type ExecutionFacts struct {
	db *bentopg.DB
}

func NewExecutionFacts(db *bentopg.DB) (*ExecutionFacts, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &ExecutionFacts{db: db}, nil
}

// FindByKey 按（租户+事项+实物+动作）取回已保存的执行事实。读回经重建门复验。
func (repository *ExecutionFacts) FindByKey(
	ctx context.Context,
	key ports.ExecutionFactKey,
) (ports.ExecutionFactRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ExecutionFactRecord{}, false, fmt.Errorf("find execution fact: %w", err)
	}

	var nodeRef, evidence, digest string
	var performedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT node_ref, evidence, performed_at, content_digest, recorded_at
		   FROM node_operations.execution_fact
		  WHERE tenant_id = $1
		    AND item_ref = $2
		    AND unit_id = $3
		    AND action = $4`,
		key.TenantID.String(),
		key.Item.String(),
		key.Unit.String(),
		key.Action.String(),
	).Scan(&nodeRef, &evidence, &performedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ExecutionFactRecord{}, false, nil
	}
	if err != nil {
		return ports.ExecutionFactRecord{}, false, fmt.Errorf("find execution fact: %w", err)
	}

	node, err := domain.NewNodeReference(nodeRef)
	if err != nil {
		return ports.ExecutionFactRecord{}, false, fmt.Errorf("find execution fact: %w", err)
	}
	evidenceRef, err := domain.NewExecutionEvidenceReference(evidence)
	if err != nil {
		return ports.ExecutionFactRecord{}, false, fmt.Errorf("find execution fact: %w", err)
	}
	fact, err := domain.RehydrateNodeExecutionFact(domain.RehydrateNodeExecutionFactSpec{
		TenantID:    key.TenantID,
		Node:        node,
		Item:        key.Item,
		Unit:        key.Unit,
		Action:      key.Action,
		Evidence:    evidenceRef,
		PerformedAt: performedAt,
	})
	if err != nil {
		return ports.ExecutionFactRecord{}, false, fmt.Errorf("find execution fact: %w", err)
	}
	return ports.ExecutionFactRecord{
		Key:           key,
		ContentDigest: digest,
		Fact:          fact,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一次执行事实。同键已有记录时答`已有记录`，不覆盖先到者。
func (repository *ExecutionFacts) Save(
	ctx context.Context,
	record ports.ExecutionFactRecord,
) (ports.ExecutionFactSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ExecutionFactSaveOutcomeInvalid, fmt.Errorf("save execution fact: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.execution_fact
			(tenant_id, item_ref, unit_id, action, node_ref, evidence,
			 performed_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Item.String(),
		record.Key.Unit.String(),
		record.Key.Action.String(),
		record.Fact.Node().String(),
		record.Fact.Evidence().String(),
		record.Fact.PerformedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ExecutionFactSaveOutcomeInvalid, fmt.Errorf("save execution fact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ExecutionFactAlreadyRecorded, nil
	}
	return ports.ExecutionFactSaved, nil
}

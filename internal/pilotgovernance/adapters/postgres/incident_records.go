package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

// inventoryEntryRow 是在途盘点 jsonb 数组的行模型，只在本包存在。条目六件齐全
// 由领域重建守——CHECK 不能对 jsonb 元素做子查询。
type inventoryEntryRow struct {
	ObjectIdentity   string    `json:"objectIdentity"`
	CurrentFacts     string    `json:"currentFacts"`
	CurrentAuthority string    `json:"currentAuthority"`
	ResponsibleParty string    `json:"responsibleParty"`
	NextAction       string    `json:"nextAction"`
	ReviewBy         time.Time `json:"reviewBy"`
}

func marshalInventory(inventory domain.InTransitInventory) ([]byte, error) {
	entries := inventory.Entries()
	rows := make([]inventoryEntryRow, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, inventoryEntryRow{
			ObjectIdentity:   entry.ObjectIdentity,
			CurrentFacts:     entry.CurrentFacts,
			CurrentAuthority: entry.CurrentAuthority,
			ResponsibleParty: entry.ResponsibleParty,
			NextAction:       entry.NextAction,
			ReviewBy:         entry.ReviewBy.UTC(),
		})
	}
	return json.Marshal(rows)
}

func rebuildInventory(raw []byte, takenAt time.Time) (domain.InTransitInventory, error) {
	var rows []inventoryEntryRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		return domain.InTransitInventory{}, err
	}
	entries := make([]domain.InventoryEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, domain.InventoryEntry{
			ObjectIdentity:   row.ObjectIdentity,
			CurrentFacts:     row.CurrentFacts,
			CurrentAuthority: row.CurrentAuthority,
			ResponsibleParty: row.ResponsibleParty,
			NextAction:       row.NextAction,
			ReviewBy:         row.ReviewBy,
		})
	}
	return domain.TakeInventory(entries, takenAt)
}

func intervalToAt(interval domain.AuthorityInterval) *time.Time {
	if interval.To.IsZero() {
		return nil
	}
	to := interval.To.UTC()
	return &to
}

func intervalFromRow(objectScope, capability, factKind, authority string, fromAt time.Time, toAt *time.Time) domain.AuthorityInterval {
	interval := domain.AuthorityInterval{
		ObjectScope: objectScope,
		Capability:  capability,
		FactKind:    factKind,
		Authority:   authority,
		From:        fromAt.UTC(),
	}
	if toAt != nil {
		interval.To = toAt.UTC()
	}
	return interval
}

// Suspensions 实现 ports.SuspensionStore。暂停标识即主键；同标识第二份由
// ON CONFLICT DO NOTHING 译成已有记录，没有 UPDATE。
type Suspensions struct {
	db *bentopg.DB
}

func NewSuspensions(db *bentopg.DB) (*Suspensions, error) {
	if db == nil {
		return nil, fmt.Errorf("pilot governance postgres: db is nil")
	}
	return &Suspensions{db: db}, nil
}

func (repository *Suspensions) FindByID(
	ctx context.Context,
	id domain.SuspensionID,
) (domain.SuspensionDecision, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SuspensionDecision{}, false, fmt.Errorf("find suspension: %w", err)
	}

	var trigger, basis, evidence, scope, executedBy, inTransit string
	var occurredAt, effectiveAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT trigger_source, basis, evidence, scope, executed_by,
		        occurred_at, effective_at, in_transit_note
		   FROM pilot_governance.suspension_decision
		  WHERE suspension_id = $1`,
		id.String(),
	).Scan(&trigger, &basis, &evidence, &scope, &executedBy, &occurredAt, &effectiveAt, &inTransit)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SuspensionDecision{}, false, nil
	}
	if err != nil {
		return domain.SuspensionDecision{}, false, fmt.Errorf("find suspension: %w", err)
	}

	scopeRef, err := domain.NewScopeVersionReference(scope)
	if err != nil {
		return domain.SuspensionDecision{}, false, fmt.Errorf("rebuild suspension: %w", err)
	}
	decision, err := domain.RecordSuspension(domain.SuspensionDecisionSpec{
		ID:            id,
		TriggerSource: trigger,
		Basis:         basis,
		Evidence:      evidence,
		Scope:         scopeRef,
		ExecutedBy:    executedBy,
		OccurredAt:    occurredAt,
		EffectiveAt:   effectiveAt,
		InTransitNote: inTransit,
	})
	if err != nil {
		return domain.SuspensionDecision{}, false, fmt.Errorf("rebuild suspension: %w", err)
	}
	return decision, true, nil
}

// FindUnresumedSuspension 回答该范围版本在时点 at 还拦不拦新准入，并交回作数的那条暂停。
// 「尚未恢复」是治理侧自己的说法：恢复必须由试点业务责任角色依据证据明确决定，指标回落
// 或规则不再命中都不解除暂停，所以没有恢复记录就仍然拦着。
//
// 边界各有出处，都不是本方法自选的：
//
//   - **答不出覆盖关系时保守答暂停**，见 domain.AdmissionSuspendedByUnreadableScopeRelation。
//     曾经这里按范围版本字面相等匹配，答不上的一律当成没暂停；那是把一条仍立着的判断
//     静默覆盖掉，而范围版本从一版升到下一版是一次限量范围扩大的 `Go/No-Go`，不是恢复
//     决定——让它顺带解除一条暂停，正是「规则不再命中即自动恢复」，明文禁止。
//   - **覆盖关系按登记读，不推不猜**（scope_version_relation，随 Go/No-Go 决定登记）：
//     登有「所问版本承继该暂停范围」的边即按承继拦（domain.AdmissionSuspendedByInheritedScope）；
//     登有互不相干的边（对称事实，任一方向）该暂停即不及于所问版本——那不是静默恢复，
//     暂停对它写明的范围照旧拦着，解除它仍只走恢复决定四件齐备。什么关系都没登，第三态
//     保守作答照旧。
//   - **生效时点算已生效**（`effective_at <= at`），与本仓权威区间 `[From, To)` 的半开
//     约定同向：生效时间那一刻起就已生效。恢复同此，故暂停与恢复同刻时以恢复为准。
//   - **同一范围多条暂停各自独立解除**。恢复记录逐条引用一个暂停标识，所以只要还有一条
//     已生效且未恢复，范围就仍在暂停中；交回哪一条按生效时间与标识定序，保证同一登记册
//     每次问都得到同一条（调用方会把它的标识写进自己的答复）。
//
// 命中那格优先于承继，承继优先于保守：三格都拦，但写明本版的那条才是调用方该引的最强
// 证据；保守那格的暂停引用只是指向「读不出关系的那一条」本身。
func (repository *Suspensions) FindUnresumedSuspension(
	ctx context.Context,
	scope domain.ScopeVersionReference,
	at time.Time,
) (domain.SuspensionDecision, domain.AdmissionSuspensionGround, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.SuspensionDecision{}, domain.AdmissionSuspensionGroundInvalid,
			fmt.Errorf("find unresumed suspension: %w", err)
	}

	var id, trigger, basis, evidence, scopeValue, executedBy, inTransit string
	var occurredAt, effectiveAt time.Time
	var namesAskedScope, inheritedByAskedScope bool
	err = querier.QueryRow(ctx,
		`SELECT suspension.suspension_id, suspension.trigger_source, suspension.basis,
		        suspension.evidence, suspension.scope, suspension.executed_by,
		        suspension.occurred_at, suspension.effective_at, suspension.in_transit_note,
		        suspension.scope = $1 AS names_asked_scope,
		        inherits.successor_scope IS NOT NULL AS inherited_by_asked_scope
		   FROM pilot_governance.suspension_decision AS suspension
		   LEFT JOIN pilot_governance.resumption_decision AS resumption
		          ON resumption.suspension_id = suspension.suspension_id
		         AND resumption.effective_at <= $2
		   LEFT JOIN pilot_governance.scope_version_relation AS inherits
		          ON inherits.relation_kind = 'INHERITS_SUSPENSIONS'
		         AND inherits.successor_scope = $1
		         AND inherits.predecessor_scope = suspension.scope
		   LEFT JOIN pilot_governance.scope_version_relation AS unrelated
		          ON unrelated.relation_kind = 'UNRELATED'
		         AND ((unrelated.successor_scope = $1 AND unrelated.predecessor_scope = suspension.scope)
		           OR (unrelated.successor_scope = suspension.scope AND unrelated.predecessor_scope = $1))
		  WHERE suspension.effective_at <= $2
		    AND resumption.suspension_id IS NULL
		    AND unrelated.successor_scope IS NULL
		  ORDER BY (suspension.scope = $1) DESC,
		           (inherits.successor_scope IS NOT NULL) DESC,
		           suspension.effective_at, suspension.suspension_id
		  LIMIT 1`,
		scope.String(),
		at.UTC(),
	).Scan(&id, &trigger, &basis, &evidence, &scopeValue, &executedBy,
		&occurredAt, &effectiveAt, &inTransit, &namesAskedScope, &inheritedByAskedScope)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.SuspensionDecision{}, domain.AdmissionNotSuspended, nil
	}
	if err != nil {
		return domain.SuspensionDecision{}, domain.AdmissionSuspensionGroundInvalid,
			fmt.Errorf("find unresumed suspension: %w", err)
	}

	ground := domain.AdmissionSuspendedByUnreadableScopeRelation
	switch {
	case namesAskedScope:
		ground = domain.AdmissionSuspendedByNamedScope
	case inheritedByAskedScope:
		ground = domain.AdmissionSuspendedByInheritedScope
	}

	suspensionID, err := domain.NewSuspensionID(id)
	if err != nil {
		return domain.SuspensionDecision{}, domain.AdmissionSuspensionGroundInvalid,
			fmt.Errorf("rebuild suspension: %w", err)
	}
	scopeRef, err := domain.NewScopeVersionReference(scopeValue)
	if err != nil {
		return domain.SuspensionDecision{}, domain.AdmissionSuspensionGroundInvalid,
			fmt.Errorf("rebuild suspension: %w", err)
	}
	decision, err := domain.RecordSuspension(domain.SuspensionDecisionSpec{
		ID:            suspensionID,
		TriggerSource: trigger,
		Basis:         basis,
		Evidence:      evidence,
		Scope:         scopeRef,
		ExecutedBy:    executedBy,
		OccurredAt:    occurredAt,
		EffectiveAt:   effectiveAt,
		InTransitNote: inTransit,
	})
	if err != nil {
		return domain.SuspensionDecision{}, domain.AdmissionSuspensionGroundInvalid,
			fmt.Errorf("rebuild suspension: %w", err)
	}
	return decision, ground, nil
}

func (repository *Suspensions) Save(
	ctx context.Context,
	decision domain.SuspensionDecision,
) (ports.GovernanceSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.GovernanceSaveOutcomeInvalid, fmt.Errorf("save suspension: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO pilot_governance.suspension_decision
			(suspension_id, trigger_source, basis, evidence, scope, executed_by,
			 occurred_at, effective_at, in_transit_note)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (suspension_id) DO NOTHING`,
		decision.ID().String(),
		decision.TriggerSource(),
		decision.Basis(),
		decision.Evidence(),
		decision.Scope().String(),
		decision.ExecutedBy(),
		decision.OccurredAt().UTC(),
		decision.EffectiveAt().UTC(),
		decision.InTransitNote(),
	)
	if err != nil {
		return ports.GovernanceSaveOutcomeInvalid, fmt.Errorf("save suspension: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.GovernanceAlreadyRecorded, nil
	}
	return ports.GovernanceSaved, nil
}

// Resumptions 实现 ports.ResumptionStore。一暂停至多一次恢复；外键指回暂停表，
// 恢复不可能先于暂停存在。
type Resumptions struct {
	db *bentopg.DB
}

func NewResumptions(db *bentopg.DB) (*Resumptions, error) {
	if db == nil {
		return nil, fmt.Errorf("pilot governance postgres: db is nil")
	}
	return &Resumptions{db: db}, nil
}

func (repository *Resumptions) FindBySuspension(
	ctx context.Context,
	id domain.SuspensionID,
) (domain.ResumptionDecision, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ResumptionDecision{}, false, fmt.Errorf("find resumption: %w", err)
	}

	var release, consistency, decidedBy string
	var inventoryRaw []byte
	var takenAt, decidedAt, effectiveAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT release_evidence, consistency_check, inventory, inventory_taken_at,
		        decided_by, decided_at, effective_at
		   FROM pilot_governance.resumption_decision
		  WHERE suspension_id = $1`,
		id.String(),
	).Scan(&release, &consistency, &inventoryRaw, &takenAt, &decidedBy, &decidedAt, &effectiveAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ResumptionDecision{}, false, nil
	}
	if err != nil {
		return domain.ResumptionDecision{}, false, fmt.Errorf("find resumption: %w", err)
	}

	inventory, err := rebuildInventory(inventoryRaw, takenAt)
	if err != nil {
		return domain.ResumptionDecision{}, false, fmt.Errorf("rebuild resumption: %w", err)
	}
	decision, err := domain.RecordResumption(domain.ResumptionDecisionSpec{
		Suspension:       id,
		ReleaseEvidence:  release,
		ConsistencyCheck: consistency,
		Inventory:        inventory,
		DecidedBy:        decidedBy,
		DecidedAt:        decidedAt,
		EffectiveAt:      effectiveAt,
	})
	if err != nil {
		return domain.ResumptionDecision{}, false, fmt.Errorf("rebuild resumption: %w", err)
	}
	return decision, true, nil
}

func (repository *Resumptions) Save(
	ctx context.Context,
	decision domain.ResumptionDecision,
) (ports.GovernanceSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.GovernanceSaveOutcomeInvalid, fmt.Errorf("save resumption: %w", err)
	}

	inventoryRaw, err := marshalInventory(decision.Inventory())
	if err != nil {
		return ports.GovernanceSaveOutcomeInvalid, fmt.Errorf("save resumption: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO pilot_governance.resumption_decision
			(suspension_id, release_evidence, consistency_check, inventory,
			 inventory_taken_at, decided_by, decided_at, effective_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (suspension_id) DO NOTHING`,
		decision.Suspension().String(),
		decision.ReleaseEvidence(),
		decision.ConsistencyCheck(),
		inventoryRaw,
		decision.Inventory().TakenAt().UTC(),
		decision.DecidedBy(),
		decision.DecidedAt().UTC(),
		decision.EffectiveAt().UTC(),
	)
	if err != nil {
		return ports.GovernanceSaveOutcomeInvalid, fmt.Errorf("save resumption: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.GovernanceAlreadyRecorded, nil
	}
	return ports.GovernanceSaved, nil
}

// Takeovers 实现 ports.TakeoverStore。键是区间四维身份加生效区间；开放区间的
// to_at 为 NULL，UNIQUE NULLS NOT DISTINCT 让同一开放区间只此一行。
type Takeovers struct {
	db *bentopg.DB
}

func NewTakeovers(db *bentopg.DB) (*Takeovers, error) {
	if db == nil {
		return nil, fmt.Errorf("pilot governance postgres: db is nil")
	}
	return &Takeovers{db: db}, nil
}

func (repository *Takeovers) FindByInterval(
	ctx context.Context,
	interval domain.AuthorityInterval,
) (domain.TakeoverRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.TakeoverRecord{}, false, fmt.Errorf("find takeover: %w", err)
	}

	var (
		objectScope, capability, factKind, authority   string
		fromAt                                         time.Time
		toAt                                           *time.Time
		stop, accepted, pending, control, duties, next string
		inventoryRaw                                   []byte
		takenAt, effectiveAt                           time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT object_scope, capability, fact_kind, authority, from_at, to_at,
		        stop_evidence, accepted_facts, pending_externals, actual_control,
		        responsibilities, next_action, inventory, inventory_taken_at, effective_at
		   FROM pilot_governance.takeover_record
		  WHERE object_scope = $1 AND capability = $2 AND fact_kind = $3
		    AND authority = $4 AND from_at = $5 AND to_at IS NOT DISTINCT FROM $6`,
		interval.ObjectScope,
		interval.Capability,
		interval.FactKind,
		interval.Authority,
		interval.From.UTC(),
		intervalToAt(interval),
	).Scan(&objectScope, &capability, &factKind, &authority, &fromAt, &toAt,
		&stop, &accepted, &pending, &control, &duties, &next,
		&inventoryRaw, &takenAt, &effectiveAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.TakeoverRecord{}, false, nil
	}
	if err != nil {
		return domain.TakeoverRecord{}, false, fmt.Errorf("find takeover: %w", err)
	}

	inventory, err := rebuildInventory(inventoryRaw, takenAt)
	if err != nil {
		return domain.TakeoverRecord{}, false, fmt.Errorf("rebuild takeover: %w", err)
	}
	record, err := domain.RecordTakeover(domain.TakeoverRecordSpec{
		StopEvidence:     stop,
		Interval:         intervalFromRow(objectScope, capability, factKind, authority, fromAt, toAt),
		AcceptedFacts:    accepted,
		PendingExternals: pending,
		ActualControl:    control,
		Responsibilities: duties,
		NextAction:       next,
		Inventory:        inventory,
		EffectiveAt:      effectiveAt,
	})
	if err != nil {
		return domain.TakeoverRecord{}, false, fmt.Errorf("rebuild takeover: %w", err)
	}
	return record, true, nil
}

func (repository *Takeovers) Save(
	ctx context.Context,
	record domain.TakeoverRecord,
) (ports.GovernanceSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.GovernanceSaveOutcomeInvalid, fmt.Errorf("save takeover: %w", err)
	}

	inventoryRaw, err := marshalInventory(record.Inventory())
	if err != nil {
		return ports.GovernanceSaveOutcomeInvalid, fmt.Errorf("save takeover: %w", err)
	}

	interval := record.Interval()
	tag, err := executor.Exec(ctx,
		`INSERT INTO pilot_governance.takeover_record
			(object_scope, capability, fact_kind, authority, from_at, to_at,
			 stop_evidence, accepted_facts, pending_externals, actual_control,
			 responsibilities, next_action, inventory, inventory_taken_at, effective_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 ON CONFLICT ON CONSTRAINT takeover_record_interval_identity DO NOTHING`,
		interval.ObjectScope,
		interval.Capability,
		interval.FactKind,
		interval.Authority,
		interval.From.UTC(),
		intervalToAt(interval),
		record.StopEvidence(),
		record.AcceptedFacts(),
		record.PendingExternals(),
		record.ActualControl(),
		record.Responsibilities(),
		record.NextAction(),
		inventoryRaw,
		record.Inventory().TakenAt().UTC(),
		record.EffectiveAt().UTC(),
	)
	if err != nil {
		return ports.GovernanceSaveOutcomeInvalid, fmt.Errorf("save takeover: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.GovernanceAlreadyRecorded, nil
	}
	return ports.GovernanceSaved, nil
}

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// ConsolidationFacts 实现 ports.ConsolidationFactStore（写入代数同 ADR-0031）：集运
// 作业的来源事实层，按（租户+来源标识）幂等。它与 ConsolidationUnits 分立，因为两者
// 存的不是一类东西——单元行是派生状态，本表是来源事实（ADR-0005）。
type ConsolidationFacts struct {
	db *bentopg.DB
}

func NewConsolidationFacts(db *bentopg.DB) (*ConsolidationFacts, error) {
	if db == nil {
		return nil, fmt.Errorf("node operations postgres: db is nil")
	}
	return &ConsolidationFacts{db: db}, nil
}

// FindByKey 按（租户+来源标识）取回已登记的作业事实。
//
// 走 ReadExecutor：事务内读得到本事务刚写的行。否定结果只回 false，不区分「不存在」
// 与「属于另一个租户」。读回的一切经领域构造函数重建——一次坏写入在这里暴露，而不是
// 变成一份看起来合法的来源事实。
func (repository *ConsolidationFacts) FindByKey(
	ctx context.Context,
	key ports.ConsolidationFactKey,
) (ports.ConsolidationFactRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ConsolidationFactRecord{}, false, fmt.Errorf("find consolidation fact: %w", err)
	}

	var (
		digest, actionName, unitID string
		memberID, sealRef          *string
		performedBy, evidence      string
		occurredAt, recordedAt     time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT content_digest, action, unit_id, member_id, seal_ref,
		        performed_by, evidence, occurred_at, recorded_at
		   FROM node_operations.consolidation_fact
		  WHERE tenant_id = $1
		    AND source_id = $2`,
		key.TenantID.String(),
		key.SourceID,
	).Scan(&digest, &actionName, &unitID, &memberID, &sealRef,
		&performedBy, &evidence, &occurredAt, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ConsolidationFactRecord{}, false, nil
	}
	if err != nil {
		return ports.ConsolidationFactRecord{}, false, fmt.Errorf("find consolidation fact: %w", err)
	}

	action, err := consolidationActionFrom(actionName)
	if err != nil {
		return ports.ConsolidationFactRecord{}, false, fmt.Errorf("find consolidation fact: %w", err)
	}
	unit, err := domain.NewConsolidationUnitID(unitID)
	if err != nil {
		return ports.ConsolidationFactRecord{}, false, fmt.Errorf("find consolidation fact: %w", err)
	}
	party, err := domain.NewPerformingPartyReference(performedBy)
	if err != nil {
		return ports.ConsolidationFactRecord{}, false, fmt.Errorf("find consolidation fact: %w", err)
	}
	proof, err := domain.NewExecutionEvidenceReference(evidence)
	if err != nil {
		return ports.ConsolidationFactRecord{}, false, fmt.Errorf("find consolidation fact: %w", err)
	}

	record := ports.ConsolidationFactRecord{
		Key:           key,
		ContentDigest: digest,
		Action:        action,
		Unit:          unit,
		PerformedBy:   party,
		Evidence:      proof,
		OccurredAt:    occurredAt.UTC(),
		RecordedAt:    recordedAt.UTC(),
	}
	if memberID != nil {
		member, err := domain.NewHandlingUnitID(*memberID)
		if err != nil {
			return ports.ConsolidationFactRecord{}, false, fmt.Errorf("find consolidation fact: %w", err)
		}
		record.Member = member
	}
	if sealRef != nil {
		seal, err := domain.NewSealReference(*sealRef)
		if err != nil {
			return ports.ConsolidationFactRecord{}, false, fmt.Errorf("find consolidation fact: %w", err)
		}
		record.Seal = seal
	}
	return record, true, nil
}

// Save 登记一次作业事实。同（租户+来源标识）已有记录时答`已有记录`——业务答案不是
// 错误（ADR-0031）；用 ON CONFLICT DO NOTHING 而不是捕 23505 译码：撞键的 INSERT 会
// 把整个事务打进中止态，而编排拿到`已有记录`还要在同一个事务里读回原结果作答。
//
// 走 RequireExecutor：来源事实与它推动的那一步单元状态必须同生共死——只落一半时，
// 要么单元变了却说不出是谁变的，要么登记声称做过一件没做的事。
func (repository *ConsolidationFacts) Save(
	ctx context.Context,
	record ports.ConsolidationFactRecord,
) (ports.ConsolidationFactSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ConsolidationFactSaveOutcomeInvalid, fmt.Errorf("save consolidation fact: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO node_operations.consolidation_fact
			(tenant_id, source_id, content_digest, action, unit_id, member_id, seal_ref,
			 performed_by, evidence, occurred_at, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.SourceID,
		record.ContentDigest,
		record.Action.String(),
		record.Unit.String(),
		optionalReference(record.Member.String()),
		optionalReference(record.Seal.String()),
		record.PerformedBy.String(),
		record.Evidence.String(),
		record.OccurredAt.UTC(),
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ConsolidationFactSaveOutcomeInvalid, fmt.Errorf("save consolidation fact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ConsolidationFactAlreadyRecorded, nil
	}
	return ports.ConsolidationFactSaved, nil
}

// optionalReference 把「这一格不适用」写成 NULL 而不是空串：迁移的在场 CHECK 按
// IS NULL 判格，空串会被当成「在场却空白」而拒。
func optionalReference(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func consolidationActionFrom(raw string) (domain.ConsolidationActionKind, error) {
	switch raw {
	case domain.OpenUnitAction.String():
		return domain.OpenUnitAction, nil
	case domain.AddMemberAction.String():
		return domain.AddMemberAction, nil
	case domain.RemoveMemberAction.String():
		return domain.RemoveMemberAction, nil
	case domain.SealUnitAction.String():
		return domain.SealUnitAction, nil
	case domain.UnsealUnitAction.String():
		return domain.UnsealUnitAction, nil
	case domain.CloseUnitAction.String():
		return domain.CloseUnitAction, nil
	default:
		return 0, fmt.Errorf("unknown consolidation action kind %q", raw)
	}
}

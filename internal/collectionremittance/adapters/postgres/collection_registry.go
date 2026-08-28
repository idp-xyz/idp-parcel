package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// CollectionRegistrations 实现 ports.CollectionRegistry：代收指令、代收事实与差异事项
// 三本册子的写口（0001 建表）。
//
// 三个方法都只 INSERT ... ON CONFLICT DO NOTHING，没有一处 UPDATE：代收事实的更正走
// 追加新事实，指令内容变化走登记新指令。给这三张表任何改写入口，已依它们落账的记账
// 就会凭空指向一份不存在过的依据，而账面上看不出这件事发生过。
type CollectionRegistrations struct {
	db *bentopg.DB
}

func NewCollectionRegistrations(db *bentopg.DB) (*CollectionRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("collection remittance postgres: db is nil")
	}
	return &CollectionRegistrations{db: db}, nil
}

var _ ports.CollectionRegistry = (*CollectionRegistrations)(nil)

func (registry *CollectionRegistrations) RegisterInstruction(
	ctx context.Context,
	tenant domain.TenantID,
	instruction domain.CollectionInstruction,
) (ports.SaveOutcome, error) {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("register collection instruction: %w", err)
	}

	ledger := instruction.Ledger()
	tag, err := executor.Exec(ctx,
		`INSERT INTO collection_remittance.collection_instruction
			(tenant_id, instruction_ref, parcel_ref, requirement_ref,
			 customer_ref, legal_entity_ref, currency, channel_ref,
			 amount_minor, instructed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), instruction.ID().String(),
		instruction.Parcel().String(), instruction.Requirement().String(),
		ledger.Customer().String(), ledger.LegalEntity().String(),
		ledger.Currency().String(), ledger.Channel().String(),
		instruction.Amount().AmountMinor(), instruction.InstructedAt().UTC(),
	)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("register collection instruction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SaveAlreadyRegistered, nil
	}
	return ports.SaveRegistered, nil
}

// AcceptCollectionFact 接受一条代收事实。来源层级的落库词由领域词表给出——空串说明
// 传进来的是未知格，那是调用方编程错误；让它撞库上的 CHECK 会把它折成「依赖故障」，
// 而两者的处置完全不同。
func (registry *CollectionRegistrations) AcceptCollectionFact(
	ctx context.Context,
	tenant domain.TenantID,
	fact domain.CollectionFact,
) (ports.SaveOutcome, error) {
	layer := fact.Layer().String()
	if layer == "" {
		return ports.SaveOutcomeInvalid,
			fmt.Errorf("accept collection fact: unknown source layer %d", fact.Layer())
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("accept collection fact: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO collection_remittance.collection_fact
			(tenant_id, fact_ref, instruction_ref, source_layer,
			 evidence_ref, currency, amount_minor, occurred_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), fact.ID().String(), fact.Instruction().String(), layer,
		fact.Evidence().String(), fact.Amount().Currency().String(),
		fact.Amount().AmountMinor(), fact.OccurredAt().UTC(),
	)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("accept collection fact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SaveAlreadyRegistered, nil
	}
	return ports.SaveRegistered, nil
}

func (registry *CollectionRegistrations) RegisterDiscrepancyItem(
	ctx context.Context,
	tenant domain.TenantID,
	item domain.DiscrepancyItem,
) (ports.SaveOutcome, error) {
	kind := item.Kind().String()
	if kind == "" {
		return ports.SaveOutcomeInvalid,
			fmt.Errorf("register discrepancy item: unknown discrepancy kind %d", item.Kind())
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("register discrepancy item: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO collection_remittance.discrepancy_item
			(tenant_id, discrepancy_ref, instruction_ref, kind,
			 currency, amount_minor, basis_ref, observed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), item.ID().String(), item.Instruction().String(), kind,
		item.Amount().Currency().String(), item.Amount().AmountMinor(),
		item.Basis().String(), item.ObservedAt().UTC(),
	)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("register discrepancy item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SaveAlreadyRegistered, nil
	}
	return ports.SaveRegistered, nil
}

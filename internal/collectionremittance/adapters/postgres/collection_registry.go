package postgres

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// CollectionRegistrations 实现 ports.CollectionRegistry：代收指令、代收事实、差异事项
// 与认可的协议抵扣四本册子的写口（0001、0003 建表）。
//
// 各方法都只 INSERT ... ON CONFLICT DO NOTHING，没有一处 UPDATE：代收事实的更正走
// 追加新事实，指令内容变化走登记新指令，认可的抵扣被更正走新认可新行。给这些表任何
// 改写入口，已依它们落账的记账就会凭空指向一份不存在过的依据，而账面上看不出这件事
// 发生过。
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
	// 运营企业真实到账必须已钉上登记时读到的采用事实版本：编排读回结算之后才落册。没钉就来写是调用方编程错误，
	// 让它撞库上的 CHECK 会被折成「依赖故障」，而两者的处置完全不同。
	var fundsVersion *string
	if fact.Layer().SettlesToOperator() {
		version, pinned := fact.AdoptedFundsVersion()
		if !pinned {
			return ports.SaveOutcomeInvalid,
				fmt.Errorf("accept collection fact: an operator bank credit must pin its adopted funds version")
		}
		raw := version.String()
		fundsVersion = &raw
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("accept collection fact: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO collection_remittance.collection_fact
			(tenant_id, fact_ref, instruction_ref, source_layer,
			 evidence_ref, currency, amount_minor, occurred_at, funds_fact_version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), fact.ID().String(), fact.Instruction().String(), layer,
		fact.Evidence().String(), fact.Amount().Currency().String(),
		fact.Amount().AmountMinor(), fact.OccurredAt().UTC(), fundsVersion,
	)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("accept collection fact: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SaveAlreadyRegistered, nil
	}
	return ports.SaveRegistered, nil
}

// LockAdoptedFundsReferences 取（租户，已采用事实引用）的事务级咨询锁，双 int 形式同本仓其余事务级锁；第二键撞上只让
// 两条不相干的事实多等一下。走 RequireExecutor：锁必须与随后的读回合计和落册在同一笔事务里，无环境事务即拒。
func (registry *CollectionRegistrations) LockAdoptedFundsReferences(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.AdoptedFundsReference,
) error {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("lock adopted funds references: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('collection_remittance.collection_fact.adopted_funds'),
		                              hashtext($1 || chr(31) || $2))`,
		tenant.String(), reference.String(),
	); err != nil {
		return fmt.Errorf("lock adopted funds references: %w", err)
	}
	return nil
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

func (registry *CollectionRegistrations) RegisterAgreedOffset(
	ctx context.Context,
	tenant domain.TenantID,
	offset domain.AgreedOffset,
) (ports.SaveOutcome, error) {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("register agreed offset: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO collection_remittance.agreed_offset
			(tenant_id, offset_ref, instruction_ref, currency, amount_minor,
			 approval_ref, approved_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), offset.ID().String(), offset.Instruction().String(),
		offset.Amount().Currency().String(), offset.Amount().AmountMinor(),
		offset.Approval().String(), offset.ApprovedAt().UTC(),
	)
	if err != nil {
		return ports.SaveOutcomeInvalid, fmt.Errorf("register agreed offset: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.SaveAlreadyRegistered, nil
	}
	return ports.SaveRegistered, nil
}

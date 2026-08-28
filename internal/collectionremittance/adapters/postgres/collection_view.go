package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// CollectionPointView 实现 ports.CollectionView：按键读回代收面三本册子的一行。
//
// found=false 只表示该键未登记。空键是调用方编程错误，与「实例还没登记」是两回事
// ——悄悄查成「未配置」会把编排的坏输入折成一个像样的业务答案（判据同关务点读口）。
type CollectionPointView struct {
	db *bentopg.DB
}

func NewCollectionPointView(db *bentopg.DB) (*CollectionPointView, error) {
	if db == nil {
		return nil, fmt.Errorf("collection remittance postgres: db is nil")
	}
	return &CollectionPointView{db: db}, nil
}

var _ ports.CollectionView = (*CollectionPointView)(nil)

func (view *CollectionPointView) LoadInstruction(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CollectionInstructionID,
) (domain.CollectionInstruction, bool, error) {
	none := domain.CollectionInstruction{}
	if strings.TrimSpace(id.String()) == "" {
		return none, false, fmt.Errorf("load collection instruction: the instruction ID is blank")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load collection instruction: %w", err)
	}

	var (
		parcelRaw, requirementRaw                       string
		customerRaw, entityRaw, currencyRaw, channelRaw string
		amountMinor                                     int64
		instructedAt                                    time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT parcel_ref, requirement_ref, customer_ref, legal_entity_ref,
		        currency, channel_ref, amount_minor, instructed_at
		   FROM collection_remittance.collection_instruction
		  WHERE tenant_id = $1 AND instruction_ref = $2`,
		tenant.String(), id.String(),
	).Scan(&parcelRaw, &requirementRaw, &customerRaw, &entityRaw,
		&currencyRaw, &channelRaw, &amountMinor, &instructedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load collection instruction: %w", err)
	}

	parcel, err := domain.NewParcelReference(parcelRaw)
	if err != nil {
		return none, false, fmt.Errorf("rebuild collection instruction: %w", err)
	}
	requirement, err := domain.NewServiceRequirementReference(requirementRaw)
	if err != nil {
		return none, false, fmt.Errorf("rebuild collection instruction: %w", err)
	}
	ledger, err := rebuildLedgerKey(customerRaw, entityRaw, currencyRaw, channelRaw)
	if err != nil {
		return none, false, err
	}
	amount, err := rebuildMoney(currencyRaw, amountMinor)
	if err != nil {
		return none, false, err
	}
	instruction, err := domain.IssueCollectionInstruction(domain.CollectionInstructionSpec{
		ID:           id,
		Parcel:       parcel,
		Requirement:  requirement,
		Ledger:       ledger,
		Amount:       amount,
		InstructedAt: instructedAt,
	})
	if err != nil {
		return none, false, fmt.Errorf("rebuild collection instruction: %w", err)
	}
	return instruction, true, nil
}

func (view *CollectionPointView) LoadCollectionFact(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CollectionFactID,
) (domain.CollectionFact, bool, error) {
	none := domain.CollectionFact{}
	if strings.TrimSpace(id.String()) == "" {
		return none, false, fmt.Errorf("load collection fact: the fact ID is blank")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load collection fact: %w", err)
	}

	var (
		instructionRaw, layerRaw, evidenceRaw, currencyRaw string
		amountMinor                                        int64
		occurredAt                                         time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT instruction_ref, source_layer, evidence_ref, currency, amount_minor, occurred_at
		   FROM collection_remittance.collection_fact
		  WHERE tenant_id = $1 AND fact_ref = $2`,
		tenant.String(), id.String(),
	).Scan(&instructionRaw, &layerRaw, &evidenceRaw, &currencyRaw, &amountMinor, &occurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load collection fact: %w", err)
	}

	instruction, err := domain.NewCollectionInstructionID(instructionRaw)
	if err != nil {
		return none, false, fmt.Errorf("rebuild collection fact: %w", err)
	}
	// 库里的陌生来源层级不折成默认成员：四层不得互相推导这条，靠的正是「不认识就
	// 说不认识」——把它默成收件人付款会让一笔真实到账在读回后变成渠道在途。
	layer, known := domain.ParseCollectionSourceLayer(layerRaw)
	if !known {
		return none, false, fmt.Errorf("rebuild collection fact: unknown source layer %q", layerRaw)
	}
	evidence, err := domain.NewEvidenceReference(evidenceRaw)
	if err != nil {
		return none, false, fmt.Errorf("rebuild collection fact: %w", err)
	}
	amount, err := rebuildMoney(currencyRaw, amountMinor)
	if err != nil {
		return none, false, err
	}
	fact, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID:          id,
		Instruction: instruction,
		Layer:       layer,
		Evidence:    evidence,
		Amount:      amount,
		OccurredAt:  occurredAt,
	})
	if err != nil {
		return none, false, fmt.Errorf("rebuild collection fact: %w", err)
	}
	return fact, true, nil
}

func (view *CollectionPointView) LoadDiscrepancyItem(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.DiscrepancyItemID,
) (domain.DiscrepancyItem, bool, error) {
	none := domain.DiscrepancyItem{}
	if strings.TrimSpace(id.String()) == "" {
		return none, false, fmt.Errorf("load discrepancy item: the discrepancy ID is blank")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load discrepancy item: %w", err)
	}

	var (
		instructionRaw, kindRaw, currencyRaw, basisRaw string
		amountMinor                                    int64
		observedAt                                     time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT instruction_ref, kind, currency, amount_minor, basis_ref, observed_at
		   FROM collection_remittance.discrepancy_item
		  WHERE tenant_id = $1 AND discrepancy_ref = $2`,
		tenant.String(), id.String(),
	).Scan(&instructionRaw, &kindRaw, &currencyRaw, &amountMinor, &basisRaw, &observedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load discrepancy item: %w", err)
	}

	instruction, err := domain.NewCollectionInstructionID(instructionRaw)
	if err != nil {
		return none, false, fmt.Errorf("rebuild discrepancy item: %w", err)
	}
	kind, known := domain.ParseDiscrepancyKind(kindRaw)
	if !known {
		return none, false, fmt.Errorf("rebuild discrepancy item: unknown kind %q", kindRaw)
	}
	amount, err := rebuildMoney(currencyRaw, amountMinor)
	if err != nil {
		return none, false, err
	}
	basis, err := domain.NewBasisReference(basisRaw)
	if err != nil {
		return none, false, fmt.Errorf("rebuild discrepancy item: %w", err)
	}
	item, err := domain.RegisterDiscrepancyItem(domain.DiscrepancyItemSpec{
		ID:          id,
		Instruction: instruction,
		Kind:        kind,
		Amount:      amount,
		Basis:       basis,
		ObservedAt:  observedAt,
	})
	if err != nil {
		return none, false, fmt.Errorf("rebuild discrepancy item: %w", err)
	}
	return item, true, nil
}

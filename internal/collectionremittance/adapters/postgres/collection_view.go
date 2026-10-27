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
		fundsVersion                                       *string
		amountMinor                                        int64
		occurredAt                                         time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT instruction_ref, source_layer, evidence_ref, currency, amount_minor, occurred_at,
		        funds_fact_version
		   FROM collection_remittance.collection_fact
		  WHERE tenant_id = $1 AND fact_ref = $2`,
		tenant.String(), id.String(),
	).Scan(&instructionRaw, &layerRaw, &evidenceRaw, &currencyRaw, &amountMinor, &occurredAt, &fundsVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load collection fact: %w", err)
	}

	fact, err := rebuildFact(id.String(), instructionRaw, layerRaw, evidenceRaw,
		currencyRaw, amountMinor, occurredAt, fundsVersion)
	if err != nil {
		return none, false, err
	}
	return fact, true, nil
}

// LoadInstructionFacts 交回该指令名下的全部代收事实，按事实标识排序。坏行同点读口一样
// 上抛：漏掉一条真实到账，按指令判短款时就看不见那笔还没移位的钱。
func (view *CollectionPointView) LoadInstructionFacts(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CollectionInstructionID,
) ([]domain.CollectionFact, error) {
	if strings.TrimSpace(id.String()) == "" {
		return nil, fmt.Errorf("load instruction facts: the instruction ID is blank")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load instruction facts: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT fact_ref, source_layer, evidence_ref, currency, amount_minor, occurred_at,
		        funds_fact_version
		   FROM collection_remittance.collection_fact
		  WHERE tenant_id = $1 AND instruction_ref = $2
		  ORDER BY fact_ref`,
		tenant.String(), id.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("load instruction facts: %w", err)
	}
	defer rows.Close()

	facts := make([]domain.CollectionFact, 0)
	for rows.Next() {
		var (
			factRaw, layerRaw, evidenceRaw, currencyRaw string
			fundsVersion                                *string
			amountMinor                                 int64
			occurredAt                                  time.Time
		)
		if err := rows.Scan(&factRaw, &layerRaw, &evidenceRaw, &currencyRaw,
			&amountMinor, &occurredAt, &fundsVersion); err != nil {
			return nil, fmt.Errorf("load instruction facts: %w", err)
		}
		fact, err := rebuildFact(factRaw, id.String(), layerRaw, evidenceRaw,
			currencyRaw, amountMinor, occurredAt, fundsVersion)
		if err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load instruction facts: %w", err)
	}
	return facts, nil
}

// LoadAdoptedFundsReferences 交回全部引用这条已采用事实的运营企业真实到账（跨指令），按事实标识排序。坏行同点读口一样
// 上抛：漏掉一条到账，合计就少算一笔。库上证据引用是自由字符串、别的三层也可能撞上同一串，所以按来源层级收窄。
func (view *CollectionPointView) LoadAdoptedFundsReferences(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.AdoptedFundsReference,
) ([]domain.CollectionFact, error) {
	if strings.TrimSpace(reference.String()) == "" {
		return nil, fmt.Errorf("load adopted funds references: the reference is blank")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load adopted funds references: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT fact_ref, instruction_ref, source_layer, evidence_ref, currency, amount_minor, occurred_at,
		        funds_fact_version
		   FROM collection_remittance.collection_fact
		  WHERE tenant_id = $1 AND evidence_ref = $2 AND source_layer = $3
		  ORDER BY fact_ref`,
		tenant.String(), reference.String(), domain.OperatorBankCredit.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("load adopted funds references: %w", err)
	}
	defer rows.Close()

	facts := make([]domain.CollectionFact, 0)
	for rows.Next() {
		var (
			factRaw, instructionRaw, layerRaw, evidenceRaw, currencyRaw string
			fundsVersion                                                *string
			amountMinor                                                 int64
			occurredAt                                                  time.Time
		)
		if err := rows.Scan(&factRaw, &instructionRaw, &layerRaw, &evidenceRaw, &currencyRaw,
			&amountMinor, &occurredAt, &fundsVersion); err != nil {
			return nil, fmt.Errorf("load adopted funds references: %w", err)
		}
		fact, err := rebuildFact(factRaw, instructionRaw, layerRaw, evidenceRaw,
			currencyRaw, amountMinor, occurredAt, fundsVersion)
		if err != nil {
			return nil, err
		}
		facts = append(facts, fact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load adopted funds references: %w", err)
	}
	return facts, nil
}

func rebuildFact(
	factRaw, instructionRaw, layerRaw, evidenceRaw, currencyRaw string,
	amountMinor int64,
	occurredAt time.Time,
	fundsVersionRaw *string,
) (domain.CollectionFact, error) {
	none := domain.CollectionFact{}
	id, err := domain.NewCollectionFactID(factRaw)
	if err != nil {
		return none, fmt.Errorf("rebuild collection fact: %w", err)
	}
	instruction, err := domain.NewCollectionInstructionID(instructionRaw)
	if err != nil {
		return none, fmt.Errorf("rebuild collection fact: %w", err)
	}
	// 库里的陌生来源层级不折成默认成员：四层不得互相推导这条，靠的正是「不认识就
	// 说不认识」——把它默成收件人付款会让一笔真实到账在读回后变成渠道在途。
	layer, known := domain.ParseCollectionSourceLayer(layerRaw)
	if !known {
		return none, fmt.Errorf("rebuild collection fact: unknown source layer %q", layerRaw)
	}
	evidence, err := domain.NewEvidenceReference(evidenceRaw)
	if err != nil {
		return none, fmt.Errorf("rebuild collection fact: %w", err)
	}
	amount, err := rebuildMoney(currencyRaw, amountMinor)
	if err != nil {
		return none, err
	}
	spec := domain.CollectionFactSpec{
		ID:          id,
		Instruction: instruction,
		Layer:       layer,
		Evidence:    evidence,
		Amount:      amount,
		OccurredAt:  occurredAt,
	}
	// NULL 就是「没按引用读过结算」：按旧口径登记的存量真实到账。不造一个空版本，事实上显式缺席。
	if fundsVersionRaw != nil {
		spec.AdoptedFundsVersion, err = domain.NewAdoptedFundsVersion(*fundsVersionRaw)
		if err != nil {
			return none, fmt.Errorf("rebuild collection fact: %w", err)
		}
	}
	fact, err := domain.AcceptCollectionFact(spec)
	if err != nil {
		return none, fmt.Errorf("rebuild collection fact: %w", err)
	}
	return fact, nil
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

func (view *CollectionPointView) LoadAgreedOffset(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.AgreedOffsetID,
) (domain.AgreedOffset, bool, error) {
	none := domain.AgreedOffset{}
	if strings.TrimSpace(id.String()) == "" {
		return none, false, fmt.Errorf("load agreed offset: the agreed offset ID is blank")
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load agreed offset: %w", err)
	}

	var (
		instructionRaw, currencyRaw, approvalRaw string
		amountMinor                              int64
		approvedAt                               time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT instruction_ref, currency, amount_minor, approval_ref, approved_at
		   FROM collection_remittance.agreed_offset
		  WHERE tenant_id = $1 AND offset_ref = $2`,
		tenant.String(), id.String(),
	).Scan(&instructionRaw, &currencyRaw, &amountMinor, &approvalRaw, &approvedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load agreed offset: %w", err)
	}

	instruction, err := domain.NewCollectionInstructionID(instructionRaw)
	if err != nil {
		return none, false, fmt.Errorf("rebuild agreed offset: %w", err)
	}
	amount, err := rebuildMoney(currencyRaw, amountMinor)
	if err != nil {
		return none, false, err
	}
	approval, err := domain.NewApprovalReference(approvalRaw)
	if err != nil {
		return none, false, fmt.Errorf("rebuild agreed offset: %w", err)
	}
	offset, err := domain.RegisterAgreedOffset(domain.AgreedOffsetSpec{
		ID:          id,
		Instruction: instruction,
		Amount:      amount,
		Approval:    approval,
		ApprovedAt:  approvedAt,
	})
	if err != nil {
		return none, false, fmt.Errorf("rebuild agreed offset: %w", err)
	}
	return offset, true, nil
}

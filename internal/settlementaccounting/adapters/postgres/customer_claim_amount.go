package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// CustomerClaimAmounts 实现 ports.CustomerClaimAmountStore（写入代数同 ADR-0031）。
// 同一金额标识只形成一次。
type CustomerClaimAmounts struct {
	db *bentopg.DB
}

func NewCustomerClaimAmounts(db *bentopg.DB) (*CustomerClaimAmounts, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &CustomerClaimAmounts{db: db}, nil
}

// FindByKey 按（租户+金额）取回。否定结果只回 false。读回经重建门复验两族形状，
// 不重审规则目录是否配置。
func (repository *CustomerClaimAmounts) FindByKey(
	ctx context.Context,
	key ports.ClaimAmountKey,
) (ports.ClaimAmountRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ClaimAmountRecord{}, false, fmt.Errorf("find customer claim amount: %w", err)
	}

	var kindName, claimItem, responsibility, rule, legal, currency, period, digest string
	var amount int64
	var original *string
	var formedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT kind, claim_item, responsibility_ref, rule_version, legal_entity,
		        original_charge, currency, amount_minor, period_ref, formed_at,
		        content_digest, recorded_at
		   FROM settlement_accounting.customer_claim_amount
		  WHERE tenant_id = $1
		    AND amount_id = $2`,
		key.TenantID.String(),
		key.Amount.String(),
	).Scan(&kindName, &claimItem, &responsibility, &rule, &legal,
		&original, &currency, &amount, &period, &formedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ClaimAmountRecord{}, false, nil
	}
	if err != nil {
		return ports.ClaimAmountRecord{}, false, fmt.Errorf("find customer claim amount: %w", err)
	}

	claimAmount, err := rebuildCustomerClaimAmount(key.Amount, kindName, claimItem, responsibility, rule, legal, original, currency, amount, period, formedAt)
	if err != nil {
		return ports.ClaimAmountRecord{}, false, fmt.Errorf("find customer claim amount: %w", err)
	}
	return ports.ClaimAmountRecord{
		Key:           key,
		ContentDigest: digest,
		Amount:        claimAmount,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一笔索赔金额。同标识已有记录时答`已形成`，不覆盖先到者。
func (repository *CustomerClaimAmounts) Save(
	ctx context.Context,
	record ports.ClaimAmountRecord,
) (ports.ClaimAmountSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ClaimAmountSaveOutcomeInvalid, fmt.Errorf("save customer claim amount: %w", err)
	}

	currency, amount := record.Amount.Amount()
	original, hasOriginal := record.Amount.OriginalCharge()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.customer_claim_amount
			(tenant_id, amount_id, kind, claim_item, responsibility_ref, rule_version,
			 legal_entity, original_charge, currency, amount_minor, period_ref,
			 formed_at, content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Amount.String(),
		record.Amount.Kind().String(),
		record.Amount.ClaimItem().String(),
		record.Amount.Responsibility().String(),
		record.Amount.RuleVersion().String(),
		record.Amount.LegalEntity().String(),
		optionalRef(original, hasOriginal),
		currency.String(),
		amount,
		record.Amount.Period().String(),
		record.Amount.FormedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ClaimAmountSaveOutcomeInvalid, fmt.Errorf("save customer claim amount: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ClaimAmountAlreadyFormed, nil
	}
	return ports.ClaimAmountSaved, nil
}

func rebuildCustomerClaimAmount(
	id domain.CustomerClaimAmountID,
	kindName, claimItem, responsibility, rule, legal string,
	original *string,
	currency string,
	amount int64,
	period string,
	formedAt time.Time,
) (domain.CustomerClaimAmount, error) {
	kind, err := customerClaimAmountKindFrom(kindName)
	if err != nil {
		return domain.CustomerClaimAmount{}, err
	}
	claimRef, err := domain.NewClaimItemReference(claimItem)
	if err != nil {
		return domain.CustomerClaimAmount{}, err
	}
	responsibilityRef, err := domain.NewResponsibilityConclusionReference(responsibility)
	if err != nil {
		return domain.CustomerClaimAmount{}, err
	}
	ruleRef, err := domain.NewAmountRuleVersionReference(rule)
	if err != nil {
		return domain.CustomerClaimAmount{}, err
	}
	legalRef, err := domain.NewLegalEntityReference(legal)
	if err != nil {
		return domain.CustomerClaimAmount{}, err
	}
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return domain.CustomerClaimAmount{}, err
	}
	periodRef, err := domain.NewBillingPeriodReference(period)
	if err != nil {
		return domain.CustomerClaimAmount{}, err
	}
	spec := domain.RehydrateCustomerClaimAmountSpec{
		ID:             id,
		Kind:           kind,
		ClaimItem:      claimRef,
		Responsibility: responsibilityRef,
		RuleVersion:    ruleRef,
		LegalEntity:    legalRef,
		Currency:       currencyCode,
		AmountMinor:    amount,
		Period:         periodRef,
		FormedAt:       formedAt,
	}
	if original != nil {
		spec.OriginalCharge, err = domain.NewCustomerChargeID(*original)
		if err != nil {
			return domain.CustomerClaimAmount{}, err
		}
	}
	return domain.RehydrateCustomerClaimAmount(spec)
}

func customerClaimAmountKindFrom(raw string) (domain.CustomerClaimAmountKind, error) {
	switch raw {
	case domain.CustomerCompensationPayable.String():
		return domain.CustomerCompensationPayable, nil
	case domain.ClaimChargeRefund.String():
		return domain.ClaimChargeRefund, nil
	default:
		return 0, fmt.Errorf("unknown customer claim amount kind %q", raw)
	}
}

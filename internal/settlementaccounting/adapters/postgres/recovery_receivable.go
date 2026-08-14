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

// RecoveryReceivables 实现 ports.RecoveryReceivableStore（写入代数同 ADR-0031）。
// 同一应追偿标识只形成一次。
type RecoveryReceivables struct {
	db *bentopg.DB
}

func NewRecoveryReceivables(db *bentopg.DB) (*RecoveryReceivables, error) {
	if db == nil {
		return nil, fmt.Errorf("settlement accounting postgres: db is nil")
	}
	return &RecoveryReceivables{db: db}, nil
}

// FindByKey 按（租户+应追偿）取回。否定结果只回 false。读回经重建门复验自身形状，
// 不重审责任结论是否仍成立。
func (repository *RecoveryReceivables) FindByKey(
	ctx context.Context,
	key ports.ReceivableKey,
) (ports.ReceivableRecord, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ReceivableRecord{}, false, fmt.Errorf("find recovery receivable: %w", err)
	}

	var matter, responsibility, counterparty, rule, legal, currency, digest string
	var amount int64
	var formedAt, recordedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT matter_ref, responsibility_ref, counterparty_ref, rule_version,
		        legal_entity, currency, amount_minor, formed_at, content_digest, recorded_at
		   FROM settlement_accounting.recovery_receivable
		  WHERE tenant_id = $1
		    AND receivable_id = $2`,
		key.TenantID.String(),
		key.Receivable.String(),
	).Scan(&matter, &responsibility, &counterparty, &rule, &legal, &currency,
		&amount, &formedAt, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.ReceivableRecord{}, false, nil
	}
	if err != nil {
		return ports.ReceivableRecord{}, false, fmt.Errorf("find recovery receivable: %w", err)
	}

	receivable, err := rebuildRecoveryReceivable(key.Receivable, matter, responsibility, counterparty, rule, legal, currency, amount, formedAt)
	if err != nil {
		return ports.ReceivableRecord{}, false, fmt.Errorf("find recovery receivable: %w", err)
	}
	return ports.ReceivableRecord{
		Key:           key,
		ContentDigest: digest,
		Receivable:    receivable,
		RecordedAt:    recordedAt.UTC(),
	}, true, nil
}

// Save 写下一笔应追偿。同标识已有记录时答`已形成`，不覆盖先到者。
func (repository *RecoveryReceivables) Save(
	ctx context.Context,
	record ports.ReceivableRecord,
) (ports.ReceivableSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ReceivableSaveOutcomeInvalid, fmt.Errorf("save recovery receivable: %w", err)
	}

	currency, amount := record.Receivable.Amount()
	tag, err := executor.Exec(ctx,
		`INSERT INTO settlement_accounting.recovery_receivable
			(tenant_id, receivable_id, matter_ref, responsibility_ref, counterparty_ref,
			 rule_version, legal_entity, currency, amount_minor, formed_at,
			 content_digest, recorded_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		 ON CONFLICT DO NOTHING`,
		record.Key.TenantID.String(),
		record.Key.Receivable.String(),
		record.Receivable.Matter().String(),
		record.Receivable.Responsibility().String(),
		record.Receivable.Counterparty().String(),
		record.Receivable.RuleVersion().String(),
		record.Receivable.LegalEntity().String(),
		currency.String(),
		amount,
		record.Receivable.FormedAt().UTC(),
		record.ContentDigest,
		record.RecordedAt.UTC(),
	)
	if err != nil {
		return ports.ReceivableSaveOutcomeInvalid, fmt.Errorf("save recovery receivable: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ReceivableAlreadyFormed, nil
	}
	return ports.ReceivableSaved, nil
}

func rebuildRecoveryReceivable(
	id domain.RecoveryReceivableID,
	matter, responsibility, counterparty, rule, legal, currency string,
	amount int64,
	formedAt time.Time,
) (domain.RecoveryReceivable, error) {
	matterRef, err := domain.NewRecoveryMatterReference(matter)
	if err != nil {
		return domain.RecoveryReceivable{}, err
	}
	responsibilityRef, err := domain.NewResponsibilityConclusionReference(responsibility)
	if err != nil {
		return domain.RecoveryReceivable{}, err
	}
	counterpartyRef, err := domain.NewRecoveryCounterpartyReference(counterparty)
	if err != nil {
		return domain.RecoveryReceivable{}, err
	}
	ruleRef, err := domain.NewAmountRuleVersionReference(rule)
	if err != nil {
		return domain.RecoveryReceivable{}, err
	}
	legalRef, err := domain.NewLegalEntityReference(legal)
	if err != nil {
		return domain.RecoveryReceivable{}, err
	}
	currencyCode, err := domain.NewCurrencyCode(currency)
	if err != nil {
		return domain.RecoveryReceivable{}, err
	}
	return domain.RehydrateRecoveryReceivable(domain.RehydrateRecoveryReceivableSpec{
		ID:             id,
		Matter:         matterRef,
		Responsibility: responsibilityRef,
		Counterparty:   counterpartyRef,
		RuleVersion:    ruleRef,
		LegalEntity:    legalRef,
		Currency:       currencyCode,
		AmountMinor:    amount,
		FormedAt:       formedAt,
	})
}
